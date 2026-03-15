package initsvc

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/docsync"
	"github.com/aidanlsb/raven/internal/schema"
)

type Code string

const (
	CodeInvalidInput   Code = "INVALID_INPUT"
	CodeFileWriteError Code = "FILE_WRITE_ERROR"
)

const WarnDocsFetchFailed = "DOCS_FETCH_FAILED"

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

type DocsResult struct {
	Fetched   bool   `json:"fetched"`
	FileCount int    `json:"file_count,omitempty"`
	StorePath string `json:"store_path,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Result struct {
	Path           string     `json:"path"`
	Status         string     `json:"status"`
	CreatedConfig  bool       `json:"created_config"`
	CreatedSchema  bool       `json:"created_schema"`
	GitignoreState string     `json:"gitignore_state"`
	Docs           DocsResult `json:"docs"`
	Warnings       []Warning  `json:"-"`
}

type InitializeRequest struct {
	Path       string
	CLIVersion string
}

func Initialize(req InitializeRequest) (*Result, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, newError(CodeInvalidInput, "path is required", "Usage: rvn init <path>", nil)
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, newError(CodeFileWriteError, "failed to create vault directory", "Check that the destination path is writable", err)
	}

	ravenDir := filepath.Join(path, ".raven")
	if err := os.MkdirAll(ravenDir, 0o755); err != nil {
		return nil, newError(CodeFileWriteError, "failed to create .raven directory", "Check that the destination path is writable", err)
	}

	gitignorePath := filepath.Join(path, ".gitignore")
	gitignoreState := "created"
	ravenGitignoreEntries := []string{".raven/", ".trash/"}

	existingContent := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existingContent = string(data)
	}

	missingEntries := make([]string, 0)
	for _, entry := range ravenGitignoreEntries {
		if !strings.Contains(existingContent, entry) {
			missingEntries = append(missingEntries, entry)
		}
	}

	if len(missingEntries) > 0 {
		var newContent string
		if existingContent == "" {
			newContent = `# Raven (auto-generated)
# These are derived files - your markdown is the source of truth

# Index database (rebuilt with 'rvn reindex')
.raven/

# Trashed files
.trash/
`
		} else {
			gitignoreState = "updated"
			addition := "\n# Raven\n"
			for _, entry := range missingEntries {
				addition += entry + "\n"
			}
			newContent = strings.TrimRight(existingContent, "\n") + "\n" + addition
		}
		if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
			return nil, newError(CodeFileWriteError, "failed to write .gitignore", "Check write permissions for .gitignore", err)
		}
	} else if existingContent != "" {
		gitignoreState = "unchanged"
	}

	createdConfig, err := config.CreateDefaultVaultConfig(path)
	if err != nil {
		return nil, newError(CodeFileWriteError, "failed to create raven.yaml", "", err)
	}
	createdSchema, err := schema.CreateDefault(path)
	if err != nil {
		return nil, newError(CodeFileWriteError, "failed to create schema.yaml", "", err)
	}

	result := &Result{
		Path:           path,
		CreatedConfig:  createdConfig,
		CreatedSchema:  createdSchema,
		GitignoreState: gitignoreState,
		Docs:           DocsResult{},
		Warnings:       []Warning{},
	}

	if createdConfig || createdSchema {
		result.Status = "initialized"
	} else {
		result.Status = "existing"
	}

	fetchResult, fetchErr := docsync.Fetch(docsync.FetchOptions{
		VaultPath:  path,
		CLIVersion: strings.TrimSpace(req.CLIVersion),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	})
	if fetchErr != nil {
		result.Warnings = append(result.Warnings, Warning{
			Code:    WarnDocsFetchFailed,
			Message: fmt.Sprintf("Docs fetch failed: %v. Run 'rvn --vault-path %s docs fetch' to retry.", fetchErr, path),
		})
	} else {
		result.Docs = DocsResult{
			Fetched:   true,
			FileCount: fetchResult.FileCount,
			StorePath: docsync.StoreRelPath,
		}
	}

	return result, nil
}
