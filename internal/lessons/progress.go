package lessons

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
)

// ProgressRelPath is the vault-relative path used for lesson completion state.
const ProgressRelPath = ".raven/learn/progress.yaml"

// Progress tracks completed lessons by lesson ID and date.
type Progress struct {
	Completed map[string]string
}

type progressFile struct {
	Completed map[string]string `yaml:"completed"`
}

// NewProgress creates an empty progress state.
func NewProgress() *Progress {
	return &Progress{
		Completed: map[string]string{},
	}
}

// LoadProgress loads lesson completion state from the vault.
// Missing progress files are treated as empty progress.
func LoadProgress(vaultPath string) (*Progress, error) {
	progressPath, err := progressFilePath(vaultPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(progressPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewProgress(), nil
		}
		return nil, fmt.Errorf("read lesson progress: %w", err)
	}

	var raw progressFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse lesson progress: %w", err)
	}
	if raw.Completed == nil {
		raw.Completed = map[string]string{}
	}

	return &Progress{
		Completed: raw.Completed,
	}, nil
}

// SaveProgress saves lesson completion state to the vault.
func SaveProgress(vaultPath string, progress *Progress) error {
	if progress == nil {
		progress = NewProgress()
	}
	if progress.Completed == nil {
		progress.Completed = map[string]string{}
	}

	progressPath, err := progressFilePath(vaultPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(progressPath), 0o755); err != nil {
		return fmt.Errorf("create progress directory: %w", err)
	}

	content, err := yaml.Marshal(progressFile{
		Completed: progress.Completed,
	})
	if err != nil {
		return fmt.Errorf("marshal lesson progress: %w", err)
	}

	if err := atomicfile.WriteFile(progressPath, content, 0o644); err != nil {
		return fmt.Errorf("write lesson progress: %w", err)
	}
	return nil
}

// IsCompleted returns true when a lesson ID has been marked complete.
func (p *Progress) IsCompleted(lessonID string) bool {
	if p == nil {
		return false
	}
	if p.Completed == nil {
		return false
	}
	_, ok := p.Completed[lessonID]
	return ok
}

// CompletedDate returns the stored completion date for a lesson ID.
func (p *Progress) CompletedDate(lessonID string) (string, bool) {
	if p == nil || p.Completed == nil {
		return "", false
	}
	date, ok := p.Completed[lessonID]
	return date, ok
}

// MarkCompleted marks a lesson complete and returns true if it was already complete.
func (p *Progress) MarkCompleted(lessonID, date string) bool {
	if p.Completed == nil {
		p.Completed = map[string]string{}
	}
	if _, exists := p.Completed[lessonID]; exists {
		return true
	}
	p.Completed[lessonID] = date
	return false
}

func progressFilePath(vaultPath string) (string, error) {
	path := filepath.Join(vaultPath, filepath.FromSlash(ProgressRelPath))
	if err := paths.ValidateWithinVault(vaultPath, path); err != nil {
		return "", fmt.Errorf("invalid progress path: %w", err)
	}
	return path, nil
}
