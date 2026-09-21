package sectionsvc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type resolveKind int

const (
	resolveFile resolveKind = iota
	resolveSectionRef
)

func resolveFileReference(rt *vaultruntime.Runtime, reference string) (*refresolve.ResolveResult, error) {
	return resolveTarget(rt, reference, resolveFile)
}

func resolveSection(rt *vaultruntime.Runtime, reference string) (*refresolve.ResolveResult, error) {
	return resolveTarget(rt, reference, resolveSectionRef)
}

func resolveTarget(rt *vaultruntime.Runtime, reference string, kind resolveKind) (*refresolve.ResolveResult, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		if kind == resolveFile {
			return nil, svcerr.New(codes.ErrInvalidInput, "file reference is required").WithSuggestion("Pass an existing Markdown file")
		}
		return nil, svcerr.New(codes.ErrInvalidInput, "section reference is required").WithSuggestion("Use a section ID like project/website#tasks")
	}

	resolved, err := refresolve.Resolve(reference, rt, false)
	if err != nil {
		return nil, mapResolveError(err, kind)
	}

	switch kind {
	case resolveFile:
		if resolved.IsSection {
			return nil, svcerr.New(codes.ErrInvalidInput, "section create target must be a file, not a section").WithSuggestion("Pass the containing file before the title")
		}
	case resolveSectionRef:
		if !resolved.IsSection {
			return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("section reference required: %s", reference)).WithSuggestion("Use a section ID like project/website#tasks")
		}
	}
	return resolved, nil
}

func mapResolveError(err error, kind resolveKind) error {
	var ambiguousErr *refresolve.AmbiguousRefError
	if errors.As(err, &ambiguousErr) {
		suggestion := "Use a full object or section ID to disambiguate"
		if kind == resolveSectionRef {
			suggestion = "Use a full section ID/path to disambiguate"
		}
		return svcerr.Wrap(codes.ErrRefAmbiguous, ambiguousErr.Error(), err).WithSuggestion(suggestion).WithDetails(map[string]any{"matches": ambiguousErr.Matches})
	}
	var notFoundErr *refresolve.RefNotFoundError
	if errors.As(err, &notFoundErr) {
		suggestion := "Check the reference and run 'rvn reindex' if needed"
		if kind == resolveSectionRef {
			suggestion = "Check the section reference and run 'rvn reindex' if needed"
		}
		return svcerr.Wrap(codes.ErrRefNotFound, notFoundErr.Error(), err).WithSuggestion(suggestion)
	}
	label := "reference"
	suggestion := "Run 'rvn reindex' and try again"
	if kind == resolveSectionRef {
		label = "section reference"
		suggestion = "Check the section reference and run 'rvn reindex' if needed"
	}
	return svcerr.Wrap(codes.ErrInternal, fmt.Sprintf("failed to resolve %s: %v", label, err), err).WithSuggestion(suggestion)
}

func loadResolvedFile(rt *vaultruntime.Runtime, reference string) (*documentState, error) {
	resolved, err := resolveFileReference(rt, reference)
	if err != nil {
		return nil, err
	}
	return loadDocument(rt, resolved.FilePath, resolved.FileObjectID)
}

func loadResolvedSection(rt *vaultruntime.Runtime, reference string) (*documentState, *model.Section, error) {
	resolved, err := resolveSection(rt, reference)
	if err != nil {
		return nil, nil, err
	}
	state, err := loadDocument(rt, resolved.FilePath, resolved.FileObjectID)
	if err != nil {
		return nil, nil, err
	}
	section := state.sectionsByID[resolved.ObjectID]
	if section == nil {
		return nil, nil, svcerr.New(codes.ErrRefNotFound, fmt.Sprintf("section not found: %s", resolved.ObjectID)).WithSuggestion("Run 'rvn reindex' if the index is stale")
	}
	return state, section, nil
}

func resolveAnchor(rt *vaultruntime.Runtime, state *documentState, reference string) (*model.Section, error) {
	resolved, err := resolveSection(rt, reference)
	if err != nil {
		return nil, err
	}
	if resolved.FileObjectID != state.fileID {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("anchor %s is not in %s", resolved.ObjectID, state.fileID)).WithSuggestion("Choose a section in the same file")
	}
	anchor := state.sectionsByID[resolved.ObjectID]
	if anchor == nil {
		return nil, svcerr.New(codes.ErrRefNotFound, fmt.Sprintf("anchor section not found: %s", resolved.ObjectID)).WithSuggestion("Run 'rvn reindex' if the index is stale")
	}
	return anchor, nil
}
