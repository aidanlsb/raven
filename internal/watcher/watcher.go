// Package watcher provides file watching and automatic reindexing for Raven vaults.
//
// It can be used standalone via `rvn watch` or embedded in the LSP server.
package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// Watcher monitors a vault directory for changes and automatically reindexes files.
type Watcher struct {
	vaultPath string
	db        *index.Database
	schema    *schema.Schema

	// Configuration
	debounceDelay time.Duration
	debug         bool

	// Internal state
	fsWatcher *fsnotify.Watcher
	pending   map[string]time.Time
	mu        sync.Mutex

	// Callbacks
	onReindex func(path string, err error)
}

// Config holds configuration options for the Watcher.
type Config struct {
	VaultPath     string
	Database      *index.Database
	Schema        *schema.Schema
	DebounceDelay time.Duration // Default: 100ms
	Debug         bool
	OnReindex     func(path string, err error) // Optional callback
}

// New creates a new Watcher with the given configuration.
func New(cfg Config) (*Watcher, error) {
	if cfg.VaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}
	if cfg.Database == nil {
		return nil, fmt.Errorf("database is required")
	}
	if cfg.Schema == nil {
		return nil, fmt.Errorf("schema is required")
	}

	debounce := cfg.DebounceDelay
	if debounce == 0 {
		debounce = 100 * time.Millisecond
	}

	return &Watcher{
		vaultPath:     cfg.VaultPath,
		db:            cfg.Database,
		schema:        cfg.Schema,
		debounceDelay: debounce,
		debug:         cfg.Debug,
		pending:       make(map[string]time.Time),
		onReindex:     cfg.OnReindex,
	}, nil
}

// Start begins watching the vault for file changes.
// It blocks until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	var err error
	w.fsWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	defer w.fsWatcher.Close()

	// Add vault directory and subdirectories
	if err := w.addWatchRecursive(w.vaultPath); err != nil {
		return fmt.Errorf("failed to watch vault: %w", err)
	}

	w.logDebug("Watching vault: %s", w.vaultPath)

	// Start debounce processor
	go w.processDebounced(ctx)

	// Event loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return nil
			}
			w.handleEvent(event)

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return nil
			}
			w.logDebug("Watcher error: %v", err)
		}
	}
}

// ReindexFile parses and indexes a single file.
// This can be called directly without starting the watcher.
func (w *Watcher) ReindexFile(path string) error {
	// Ensure path is absolute
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.vaultPath, path)
	}

	// Skip non-markdown files
	if !strings.HasSuffix(path, ".md") {
		return nil
	}

	// Skip ignored directories
	if w.shouldIgnore(path) {
		return nil
	}

	// Get file mtime before reading
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	fileMtime := stat.ModTime().Unix()

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	parsed, err := parser.ParseDocument(string(content), path, w.vaultPath)
	if err != nil {
		return fmt.Errorf("failed to parse document: %w", err)
	}

	if err := w.db.IndexDocumentWithMtime(parsed, w.schema, fileMtime); err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}

	return nil
}

// RemoveFromIndex removes a file from the index.
func (w *Watcher) RemoveFromIndex(path string) error {
	// Convert to relative path / object ID
	relPath, err := filepath.Rel(w.vaultPath, path)
	if err != nil {
		return err
	}

	// Remove .md extension to get object ID
	objectID := strings.TrimSuffix(relPath, ".md")

	return w.db.RemoveDocument(objectID)
}

// handleEvent processes a single filesystem event.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// Skip non-markdown files
	if !strings.HasSuffix(path, ".md") {
		// But watch new directories
		if event.Op&fsnotify.Create != 0 {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				w.addWatchRecursive(path)
			}
		}
		return
	}

	// Skip ignored paths
	if w.shouldIgnore(path) {
		return
	}

	w.logDebug("Event: %s %s", event.Op, path)

	switch {
	case event.Op&fsnotify.Write != 0, event.Op&fsnotify.Create != 0:
		w.scheduleReindex(path)
	case event.Op&fsnotify.Remove != 0, event.Op&fsnotify.Rename != 0:
		// File removed - remove from index
		if err := w.RemoveFromIndex(path); err != nil {
			w.logDebug("Failed to remove from index: %v", err)
		}
	}
}

// scheduleReindex adds a file to the pending reindex queue with debouncing.
func (w *Watcher) scheduleReindex(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending[path] = time.Now()
}

// processDebounced processes pending reindex requests after debounce delay.
func (w *Watcher) processDebounced(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processPending()
		}
	}
}

// processPending checks for files ready to reindex (past debounce delay).
func (w *Watcher) processPending() {
	w.mu.Lock()
	now := time.Now()
	ready := make([]string, 0)

	for path, scheduledAt := range w.pending {
		if now.Sub(scheduledAt) >= w.debounceDelay {
			ready = append(ready, path)
			delete(w.pending, path)
		}
	}
	w.mu.Unlock()

	// Reindex ready files
	for _, path := range ready {
		err := w.ReindexFile(path)
		if w.onReindex != nil {
			w.onReindex(path, err)
		}
		if err != nil {
			w.logDebug("Failed to reindex %s: %v", path, err)
		} else {
			w.logDebug("Reindexed: %s", path)
		}
	}
}

// addWatchRecursive adds a directory and all subdirectories to the watcher.
func (w *Watcher) addWatchRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip ignored directories
			if w.shouldIgnoreDir(path) {
				return filepath.SkipDir
			}
			if err := w.fsWatcher.Add(path); err != nil {
				w.logDebug("Failed to watch %s: %v", path, err)
			}
		}
		return nil
	})
}

// shouldIgnore returns true if the path should be ignored.
func (w *Watcher) shouldIgnore(path string) bool {
	rel, err := filepath.Rel(w.vaultPath, path)
	if err != nil {
		return false
	}

	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == ".raven" || part == ".git" || part == ".trash" || part == "node_modules" {
			return true
		}
	}
	return false
}

// shouldIgnoreDir returns true if the directory should not be watched.
func (w *Watcher) shouldIgnoreDir(path string) bool {
	base := filepath.Base(path)
	return base == ".raven" || base == ".git" || base == ".trash" || base == "node_modules"
}

// logDebug logs a debug message if debug mode is enabled.
func (w *Watcher) logDebug(format string, args ...interface{}) {
	if w.debug {
		fmt.Fprintf(os.Stderr, "[raven-watcher] "+format+"\n", args...)
	}
}
