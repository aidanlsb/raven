package readsvc

import (
	"fmt"
	"path/filepath"
	"strings"
)

type OpenTarget struct {
	Reference    string `json:"reference"`
	ObjectID     string `json:"object_id"`
	FilePath     string `json:"file_path"`
	RelativePath string `json:"relative_path"`
	IsSection    bool   `json:"is_section,omitempty"`
	FileObjectID string `json:"file_object_id,omitempty"`
	LineStart    int    `json:"line_start,omitempty"`
}

type OpenFailure struct {
	Reference string `json:"reference"`
	Message   string `json:"message"`
}

func ResolveOpenTarget(rt *Runtime, reference string) (*OpenTarget, error) {
	resolveOp, err := newResolveOperation(rt)
	if err != nil {
		return nil, err
	}
	defer resolveOp.Close()
	return resolveOpenTargetWithOperation(rt, reference, resolveOp)
}

func resolveOpenTargetWithOperation(rt *Runtime, reference string, resolveOp *resolveOperation) (*OpenTarget, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return nil, fmt.Errorf("reference is required")
	}

	var (
		resolved *ResolveResult
		err      error
	)
	if resolveOp != nil {
		resolved, err = resolveOp.resolveReferenceWithDynamicDates(ref, false)
	} else {
		resolved, err = ResolveReferenceWithDynamicDates(ref, rt, false)
	}
	if err != nil {
		return nil, err
	}
	relPath, err := filepath.Rel(rt.VaultPath, resolved.FilePath)
	if err != nil {
		return nil, err
	}

	fileObjectID := ""
	if resolved.IsSection {
		fileObjectID = resolved.FileObjectID
	}
	return &OpenTarget{
		Reference:    ref,
		ObjectID:     resolved.ObjectID,
		FilePath:     resolved.FilePath,
		RelativePath: filepath.ToSlash(relPath),
		IsSection:    resolved.IsSection,
		FileObjectID: fileObjectID,
		LineStart:    resolved.LineStart,
	}, nil
}

func ResolveOpenTargets(rt *Runtime, references []string) ([]OpenTarget, []OpenFailure) {
	targets := make([]OpenTarget, 0, len(references))
	failures := make([]OpenFailure, 0)
	resolveOp, err := newResolveOperation(rt)
	if err != nil {
		return nil, []OpenFailure{{Reference: "", Message: err.Error()}}
	}
	defer resolveOp.Close()

	for _, reference := range references {
		ref := strings.TrimSpace(reference)
		if ref == "" {
			continue
		}
		target, err := resolveOpenTargetWithOperation(rt, ref, resolveOp)
		if err != nil {
			failures = append(failures, OpenFailure{Reference: ref, Message: err.Error()})
			continue
		}
		targets = append(targets, *target)
	}

	return targets, failures
}
