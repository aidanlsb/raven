package initsvc

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/docsync"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
)

const WarnDocsFetchFailed = codes.WarnDocsFetchFailed

type DocsResult struct {
	Fetched   bool   `json:"fetched"`
	FileCount int    `json:"file_count,omitempty"`
	StorePath string `json:"store_path,omitempty"`
}

type Warning struct {
	Code    codes.WarningCode `json:"code"`
	Message string            `json:"message"`
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
	ConfigPath string
	CLIVersion string
}

func Initialize(req InitializeRequest) (*Result, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "path is required").WithSuggestion("Usage: rvn init <path>")
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create vault directory", err).WithSuggestion("Check that the destination path is writable")
	}

	ravenDir := filepath.Join(path, ".raven")
	if err := os.MkdirAll(ravenDir, 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create .raven directory", err).WithSuggestion("Check that the destination path is writable")
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
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write .gitignore", err).WithSuggestion("Check write permissions for .gitignore")
		}
	} else if existingContent != "" {
		gitignoreState = "unchanged"
	}

	createdConfig, err := config.CreateDefaultVaultConfig(path)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create raven.yaml", err)
	}
	createdSchema, err := schema.CreateDefault(path)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create schema.yaml", err)
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
		ConfigPath: strings.TrimSpace(req.ConfigPath),
		CLIVersion: strings.TrimSpace(req.CLIVersion),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	})
	if fetchErr != nil {
		result.Warnings = append(result.Warnings, Warning{
			Code:    WarnDocsFetchFailed,
			Message: fmt.Sprintf("Docs fetch failed: %v. Run 'rvn docs fetch' to retry.", fetchErr),
		})
	} else {
		result.Docs = DocsResult{
			Fetched:   true,
			FileCount: fetchResult.FileCount,
			StorePath: fetchResult.DocsPath,
		}
	}

	return result, nil
}
