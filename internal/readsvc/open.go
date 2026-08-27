package readsvc

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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

type OpenResult struct {
	Targets  []OpenTarget
	Failures []OpenFailure
	Opened   bool
	Editor   string
}

func OpenReferences(rt *vaultruntime.Runtime, cfg *config.Config, references []string) OpenResult {
	targets, failures := ResolveOpenTargets(rt, references)
	filePaths := make([]string, 0, len(targets))
	for _, target := range targets {
		filePaths = append(filePaths, target.FilePath)
	}
	editor := ""
	if cfg != nil {
		editor = cfg.GetEditor()
	}
	return OpenResult{
		Targets: targets, Failures: failures,
		Opened: vault.OpenFilesInEditor(cfg, filePaths), Editor: editor,
	}
}

func OpenReference(rt *vaultruntime.Runtime, cfg *config.Config, reference string) (*OpenTarget, bool, string, error) {
	target, err := ResolveOpenTarget(rt, reference)
	if err != nil {
		return nil, false, "", err
	}
	editor := ""
	if cfg != nil {
		editor = cfg.GetEditor()
	}
	return target, vault.OpenInEditorAtLine(cfg, target.FilePath, target.LineStart), editor, nil
}

func ResolveOpenTarget(rt *vaultruntime.Runtime, reference string) (*OpenTarget, error) {
	resolveOp, err := refresolve.New(rt)
	if err != nil {
		return nil, err
	}
	return resolveOpenTargetWithOperation(rt, reference, resolveOp)
}

func resolveOpenTargetWithOperation(rt *vaultruntime.Runtime, reference string, resolveOp *refresolve.Operation) (*OpenTarget, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return nil, fmt.Errorf("reference is required")
	}

	var (
		resolved *refresolve.ResolveResult
		err      error
	)
	if resolveOp != nil {
		resolved, err = resolveOp.ResolveDynamic(ref, false)
	} else {
		resolved, err = refresolve.ResolveDynamic(ref, rt, false)
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

func ResolveOpenTargets(rt *vaultruntime.Runtime, references []string) ([]OpenTarget, []OpenFailure) {
	targets := make([]OpenTarget, 0, len(references))
	failures := make([]OpenFailure, 0)
	resolveOp, err := refresolve.New(rt)
	if err != nil {
		return nil, []OpenFailure{{Reference: "", Message: err.Error()}}
	}

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
