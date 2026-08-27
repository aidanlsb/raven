package templatesvc

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/template"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const WarningIndexUpdateFailed = codes.WarnIndexUpdateFailed

type Warning struct {
	Code    codes.WarningCode
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

type ReadRequest struct {
	VaultPath   string
	TemplateDir string
	Path        string
}

type ReadResult struct {
	Path        string
	TemplateDir string
	Content     string
	Exists      bool
}

type WriteResult struct {
	Path        string
	Status      string
	TemplateDir string
	Changed     bool
	ChangedPath string
	Warnings    []Warning
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

func List(rt *vaultruntime.Runtime, req ListRequest) (*ListResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	req.VaultPath = rt.VaultPath

	root := filepath.Join(req.VaultPath, filepath.FromSlash(req.TemplateDir))
	if err := paths.ValidateWithinVault(req.VaultPath, root); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "template directory must be within the vault", err)
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return &ListResult{
			TemplateDir: req.TemplateDir,
			Templates:   []TemplateFileInfo{},
		}, nil
	} else if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read template directory", err)
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
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to list template files", err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return &ListResult{
		TemplateDir: req.TemplateDir,
		Templates:   files,
	}, nil
}

func Read(req ReadRequest) (*ReadResult, error) {
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}

	fileRef, fullPath, err := resolveTemplatePath(req.VaultPath, req.TemplateDir, req.Path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(fullPath)
	if os.IsNotExist(err) {
		return &ReadResult{
			Path:        fileRef,
			TemplateDir: req.TemplateDir,
			Content:     "",
			Exists:      false,
		}, nil
	}
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed reading template file", err)
	}

	return &ReadResult{
		Path:        fileRef,
		TemplateDir: req.TemplateDir,
		Content:     string(content),
		Exists:      true,
	}, nil
}

func Write(rt *vaultruntime.Runtime, req WriteRequest) (*WriteResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	req.VaultPath = rt.VaultPath
	projectionLock, err := reindexsvc.LockProjection(rt, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = projectionLock.Close() }()

	fileRef, fullPath, err := resolveTemplatePath(req.VaultPath, req.TemplateDir, req.Path)
	if err != nil {
		return nil, err
	}

	if err := template.ValidateContent(req.Content); err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, err.Error(), err).WithSuggestion("Template files should contain only body Markdown; Raven writes object frontmatter separately")
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "unable to create template directory", err)
	}

	status := "created"
	if existing, readErr := os.ReadFile(fullPath); readErr == nil {
		if string(existing) == req.Content {
			status = "unchanged"
		} else {
			status = "updated"
		}
	} else if !os.IsNotExist(readErr) {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed reading existing template file", readErr)
	}

	changed := status != "unchanged"
	if changed {
		if err := atomicfile.WriteFile(fullPath, []byte(req.Content), 0o644); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed writing template file", err)
		}
	}

	result := &WriteResult{
		Path:        fileRef,
		Status:      status,
		TemplateDir: req.TemplateDir,
		Changed:     changed,
		ChangedPath: fullPath,
	}
	if changed {
		for _, warning := range reindexsvc.ProjectFileLocked(rt, fullPath) {
			result.Warnings = append(result.Warnings, Warning{
				Code: warning.Code, Message: warning.Message, Ref: warning.Ref,
			})
		}
	}
	return result, nil
}

func Delete(rt *vaultruntime.Runtime, req DeleteRequest) (*DeleteResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	req.VaultPath = rt.VaultPath
	projectionLock, err := reindexsvc.LockProjection(rt, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = projectionLock.Close() }()

	fileRef, fullPath, err := resolveTemplatePath(req.VaultPath, req.TemplateDir, req.Path)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, svcerr.Wrap(codes.ErrFileNotFound, fmt.Sprintf("template file not found: %s", fileRef), err)
	} else if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read template file metadata", err)
	}

	templateIDs, err := schemaTemplateRefsForFile(rt, fileRef, req.TemplateDir)
	if err != nil {
		return nil, err
	}
	if len(templateIDs) > 0 && !req.Force {
		return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("template file %q is referenced by schema templates: %s", fileRef, strings.Join(templateIDs, ", "))).WithSuggestion("Remove those template definitions first with `rvn schema template remove <template_id>` or use --force")
	}

	trashRef, err := moveTemplateToTrash(req.VaultPath, fileRef)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "unable to move template file to .trash", err)
	}

	warnings := make([]Warning, 0, 1)
	if err := rt.OpenDB(); err != nil {
		warnings = append(warnings, Warning{
			Code:    WarningIndexUpdateFailed,
			Message: fmt.Sprintf("failed to open index for cleanup: %v", err),
			Ref:     "Run 'rvn reindex' to rebuild the index",
		})
	} else {
		if err := rt.DB.RemoveFile(fileRef); err != nil {
			warnings = append(warnings, Warning{
				Code:    WarningIndexUpdateFailed,
				Message: fmt.Sprintf("failed to remove file from index: %v", err),
				Ref:     "Run 'rvn reindex' to rebuild the index",
			})
		}
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
		return "", "", svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion(fmt.Sprintf("Use a file path under %s", templateDir))
	}

	fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return "", "", svcerr.Wrap(codes.ErrFileOutsideVault, "template files must be within the vault", err)
	}

	return fileRef, fullPath, nil
}

func schemaTemplateRefsForFile(rt *vaultruntime.Runtime, fileRef, templateDir string) ([]string, error) {
	if rt.SchemaLoadErr != nil {
		return nil, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to load schema", rt.SchemaLoadErr).WithSuggestion("Fix schema.yaml and try again")
	}
	if rt.Schema == nil {
		return nil, svcerr.New(codes.ErrSchemaInvalid, "schema runtime is required").WithSuggestion("Fix schema.yaml and try again")
	}
	sch := rt.Schema

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
