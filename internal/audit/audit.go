// Package audit provides an append-only audit log for tracking operations.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a single audit log entry.
type Entry struct {
	Timestamp time.Time              `json:"ts"`
	Operation string                 `json:"op"`     // create, update, delete, reindex
	Entity    string                 `json:"entity"` // object, trait, file
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type,omitempty"`  // For objects: type name
	Parent    string                 `json:"parent,omitempty"` // For traits: parent object
	Content   string                 `json:"content,omitempty"`
	Changes   map[string]interface{} `json:"changes,omitempty"` // For updates: {field: {old: x, new: y}}
	Extra     map[string]interface{} `json:"extra,omitempty"`   // Any additional context
}

// Logger handles writing to the audit log.
type Logger struct {
	path    string
	enabled bool
	mu      sync.Mutex
}

// New creates a new audit logger for the given vault.
// If enabled is false, the logger will be a no-op.
func New(vaultPath string, enabled bool) *Logger {
	if !enabled {
		return &Logger{enabled: false}
	}

	logPath := filepath.Join(vaultPath, ".raven", "audit.log")
	return &Logger{
		path:    logPath,
		enabled: true,
	}
}

// Log writes an entry to the audit log.
func (l *Logger) Log(entry Entry) error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure timestamp is set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(l.path), 0755); err != nil {
		return fmt.Errorf("failed to create audit directory: %w", err)
	}

	// Append to log file
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}

	return nil
}

// LogCreate logs an object or trait creation.
func (l *Logger) LogCreate(entity, id, entityType string, extra map[string]interface{}) error {
	return l.Log(Entry{
		Operation: "create",
		Entity:    entity,
		ID:        id,
		Type:      entityType,
		Extra:     extra,
	})
}

// LogUpdate logs an object or trait update.
func (l *Logger) LogUpdate(entity, id string, changes map[string]interface{}) error {
	return l.Log(Entry{
		Operation: "update",
		Entity:    entity,
		ID:        id,
		Changes:   changes,
	})
}

// LogDelete logs an object or trait deletion.
func (l *Logger) LogDelete(entity, id string) error {
	return l.Log(Entry{
		Operation: "delete",
		Entity:    entity,
		ID:        id,
	})
}

// LogReindex logs a reindex operation with discovered/removed entities.
func (l *Logger) LogReindex(discovered, removed []string) error {
	extra := make(map[string]interface{})
	if len(discovered) > 0 {
		extra["discovered"] = discovered
	}
	if len(removed) > 0 {
		extra["removed"] = removed
	}
	return l.Log(Entry{
		Operation: "reindex",
		Entity:    "vault",
		Extra:     extra,
	})
}

// LogCapture logs a quick capture operation.
func (l *Logger) LogCapture(file string, content string) error {
	return l.Log(Entry{
		Operation: "capture",
		Entity:    "file",
		ID:        file,
		Content:   content,
	})
}

// Read reads all entries from the audit log.
func (l *Logger) Read() ([]Entry, error) {
	if !l.enabled {
		return nil, nil
	}

	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read audit log: %w", err)
	}

	var entries []Entry
	lines := splitLines(string(data))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed entries
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ReadSince reads entries from the audit log since the given time.
func (l *Logger) ReadSince(since time.Time) ([]Entry, error) {
	all, err := l.Read()
	if err != nil {
		return nil, err
	}

	var filtered []Entry
	for _, entry := range all {
		if entry.Timestamp.After(since) || entry.Timestamp.Equal(since) {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// ReadForEntity reads entries for a specific entity ID.
func (l *Logger) ReadForEntity(entityID string) ([]Entry, error) {
	all, err := l.Read()
	if err != nil {
		return nil, err
	}

	var filtered []Entry
	for _, entry := range all {
		if entry.ID == entityID {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// Enabled returns true if the audit logger is enabled.
func (l *Logger) Enabled() bool {
	return l.enabled
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
