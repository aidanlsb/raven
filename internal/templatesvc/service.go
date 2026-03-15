package templatesvc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/template"
)

type Code string

const (
	CodeInvalidInput     Code = "INVALID_INPUT"
	CodeFileNotFound     Code = "FILE_NOT_FOUND"
	CodeFileReadError    Code = "FILE_READ_ERROR"
	CodeFileWriteError   Code = "FILE_WRITE_ERROR"
	CodeFileOutsideVault Code = "FILE_OUTSIDE_VAULT"
	CodeSchemaInvalid    Code = "SCHEMA_INVALID"
	CodeValidationFailed Code = "VALIDATION_FAILED"
	CodeInternal         Code = "INTERNAL_ERROR"
)

const WarningIndexUpdateFailed = "INDEX_UPDATE_FAILED"

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type Warning struct {
	Code    string
	Message string
	Ref     string
}

type TemplateFileInfo struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type ListRequest struct {
	VaultPath   string
	TemplateDir string
}

type ListResult struct {
	TemplateDir string
	Templates   []TemplateFileInfo
}

type WriteRequest struct {
	VaultPath   string
	TemplateDir string
	Path        string
	Content     string
}

type WriteResult struct {
	Path        string
	Status      string
	TemplateDir string
	Changed     bool
	ChangedPath string
}

type DeleteRequest struct {
	VaultPath   string
	TemplateDir string
	Path        string
	Force       bool
}

type DeleteResult struct {
	DeletedPath string
	TrashPath   string
	Forced      bool
	TemplateIDs []string
	Warnings    []Warning
}

func List(req ListRequest) (*ListResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}

	root := filepath.Join(req.VaultPath, filepath.FromSlash(req.TemplateDir))
	if err := paths.ValidateWithinVault(req.VaultPath, root); err != nil {
		return nil, newError(CodeFileOutsideVault, "template directory must be within the vault", "", err)
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return &ListResult{
			TemplateDir: req.TemplateDir,
			Templates:   []TemplateFileInfo{},
		}, nil
	} else if err != nil {
		return nil, newError(CodeFileReadError, "failed to read template directory", "", err)
	}

	files := make([]TemplateFileInfo, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(req.VaultPath, path)
		if err != nil {
			return err
		}
		files = append(files, TemplateFileInfo{
			Path:      filepath.ToSlash(rel),
			SizeBytes: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, newError(CodeFileReadError, "failed to list template files", "", err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return &ListResult{
		TemplateDir: req.TemplateDir,
		Templates:   files,
	}, nil
}

func Write(req WriteRequest) (*WriteResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}

	fileRef, fullPath, err := resolveTemplatePath(req.VaultPath, req.TemplateDir, req.Path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, newError(CodeFileWriteError, "unable to create template directory", "", err)
	}

	status := "created"
	if existing, readErr := os.ReadFile(fullPath); readErr == nil {
		if string(existing) == req.Content {
			status = "unchanged"
		} else {
			status = "updated"
		}
	} else if !os.IsNotExist(readErr) {
		return nil, newError(CodeFileReadError, "failed reading existing template file", "", readErr)
	}

	changed := status != "unchanged"
	if changed {
		if err := atomicfile.WriteFile(fullPath, []byte(req.Content), 0o644); err != nil {
			return nil, newError(CodeFileWriteError, "failed writing template file", "", err)
		}
	}

	return &WriteResult{
		Path:        fileRef,
		Status:      status,
		TemplateDir: req.TemplateDir,
		Changed:     changed,
		ChangedPath: fullPath,
	}, nil
}

func Delete(req DeleteRequest) (*DeleteResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}

	fileRef, fullPath, err := resolveTemplatePath(req.VaultPath, req.TemplateDir, req.Path)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, newError(CodeFileNotFound, fmt.Sprintf("template file not found: %s", fileRef), "", err)
	} else if err != nil {
		return nil, newError(CodeFileReadError, "failed to read template file metadata", "", err)
	}

	templateIDs, err := schemaTemplateRefsForFile(req.VaultPath, fileRef, req.TemplateDir)
	if err != nil {
		return nil, err
	}
	if len(templateIDs) > 0 && !req.Force {
		return nil, newError(
			CodeValidationFailed,
			fmt.Sprintf("template file %q is referenced by schema templates: %s", fileRef, strings.Join(templateIDs, ", ")),
			"Remove those template definitions first with `rvn schema template remove <template_id>` or use --force",
			nil,
		)
	}

	trashRef, err := moveTemplateToTrash(req.VaultPath, fileRef)
	if err != nil {
		return nil, newError(CodeFileWriteError, "unable to move template file to .trash", "", err)
	}

	warnings := make([]Warning, 0, 1)
	db, err := index.Open(req.VaultPath)
	if err != nil {
		warnings = append(warnings, Warning{
			Code:    WarningIndexUpdateFailed,
			Message: fmt.Sprintf("failed to open index for cleanup: %v", err),
			Ref:     "Run 'rvn reindex' to rebuild the index",
		})
	} else {
		if err := db.RemoveFile(fileRef); err != nil {
			warnings = append(warnings, Warning{
				Code:    WarningIndexUpdateFailed,
				Message: fmt.Sprintf("failed to remove file from index: %v", err),
				Ref:     "Run 'rvn reindex' to rebuild the index",
			})
		}
		_ = db.Close()
	}

	return &DeleteResult{
		DeletedPath: fileRef,
		TrashPath:   trashRef,
		Forced:      req.Force,
		TemplateIDs: templateIDs,
		Warnings:    warnings,
	}, nil
}

func resolveTemplatePath(vaultPath, templateDir, pathArg string) (string, string, error) {
	fileRef, err := template.ResolveFileRef(pathArg, templateDir)
	if err != nil {
		return "", "", newError(CodeInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir), err)
	}

	fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return "", "", newError(CodeFileOutsideVault, "template files must be within the vault", "", err)
	}

	return fileRef, fullPath, nil
}

func schemaTemplateRefsForFile(vaultPath, fileRef, templateDir string) ([]string, error) {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return nil, newError(CodeSchemaInvalid, "failed to load schema", "Fix schema.yaml and try again", err)
	}

	var refs []string
	target := filepath.ToSlash(fileRef)
	for templateID, def := range sch.Templates {
		if def == nil {
			continue
		}
		candidate := filepath.ToSlash(strings.TrimSpace(def.File))
		resolved, err := template.ResolveFileRef(candidate, templateDir)
		if err == nil {
			candidate = filepath.ToSlash(resolved)
		}
		if candidate == target {
			refs = append(refs, templateID)
		}
	}

	sort.Strings(refs)
	return refs, nil
}

func moveTemplateToTrash(vaultPath, fileRef string) (string, error) {
	sourceAbs := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	trashRef, err := uniqueTrashRef(vaultPath, filepath.ToSlash(filepath.Join(".trash", filepath.FromSlash(fileRef))))
	if err != nil {
		return "", err
	}
	destAbs := filepath.Join(vaultPath, filepath.FromSlash(trashRef))

	if err := paths.ValidateWithinVault(vaultPath, destAbs); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(sourceAbs, destAbs); err != nil {
		return "", err
	}
	return trashRef, nil
}

func uniqueTrashRef(vaultPath, initial string) (string, error) {
	candidate := initial
	ext := filepath.Ext(initial)
	base := strings.TrimSuffix(initial, ext)

	for i := 0; i < 1000; i++ {
		candidateAbs := filepath.Join(vaultPath, filepath.FromSlash(candidate))
		if _, err := os.Stat(candidateAbs); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d%s", base, time.Now().UTC().UnixNano(), ext)
	}

	return "", fmt.Errorf("failed to generate unique trash path for %s", initial)
}
