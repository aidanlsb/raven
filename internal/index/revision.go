package index

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	revisionGenerationMetaKey = "revision_generation"
	revisionCounterMetaKey    = "revision_counter"
	revisionGenerationBytes   = 16
)

// Revision is a disposable index identity. Generation is random and created
// whenever the index is built from scratch. Counter is monotonic and bumped
// in the same transaction as every index projection. Deleting the index
// file discards both; the next open mints a new generation.
type Revision struct {
	Generation string `json:"generation"`
	Counter    int64  `json:"counter"`
}

// Map returns the JSON-object form used in CLI/MCP/API envelopes.
func (r Revision) Map() map[string]interface{} {
	return map[string]interface{}{
		"generation": r.Generation,
		"counter":    r.Counter,
	}
}

// Revision returns the durable generation and projection counter.
func (d *Database) Revision() (Revision, error) {
	if d == nil || d.db == nil {
		return Revision{}, fmt.Errorf("database is nil")
	}
	return readRevision(d.db)
}

// ReadRevision opens the vault index, reads its revision, and closes the handle.
func ReadRevision(vaultPath string) (Revision, error) {
	db, err := Open(vaultPath)
	if err != nil {
		return Revision{}, err
	}
	defer db.Close()
	return db.Revision()
}

type metaStore interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func readRevision(store metaStore) (Revision, error) {
	var generation string
	err := store.QueryRow(`SELECT value FROM meta WHERE key = ?`, revisionGenerationMetaKey).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, nil
	}
	if err != nil {
		return Revision{}, fmt.Errorf("read index revision generation: %w", err)
	}

	var raw string
	err = store.QueryRow(`SELECT value FROM meta WHERE key = ?`, revisionCounterMetaKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{Generation: generation}, nil
	}
	if err != nil {
		return Revision{}, fmt.Errorf("read index revision counter: %w", err)
	}
	counter, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return Revision{}, fmt.Errorf("parse index revision counter %q: %w", raw, err)
	}
	return Revision{Generation: generation, Counter: counter}, nil
}

func ensureRevision(store metaStore, forceNew bool) error {
	if forceNew {
		return resetRevision(store)
	}
	var generation string
	err := store.QueryRow(`SELECT value FROM meta WHERE key = ?`, revisionGenerationMetaKey).Scan(&generation)
	if err == nil && strings.TrimSpace(generation) != "" {
		var raw string
		counterErr := store.QueryRow(`SELECT value FROM meta WHERE key = ?`, revisionCounterMetaKey).Scan(&raw)
		if counterErr == nil {
			return nil
		}
		if !errors.Is(counterErr, sql.ErrNoRows) {
			return fmt.Errorf("read index revision counter: %w", counterErr)
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read index revision generation: %w", err)
	}
	return resetRevision(store)
}

func resetRevision(store metaStore) error {
	generation, err := newRevisionGeneration()
	if err != nil {
		return err
	}
	if _, err := store.Exec(
		`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`,
		revisionGenerationMetaKey,
		generation,
	); err != nil {
		return fmt.Errorf("set index revision generation: %w", err)
	}
	if _, err := store.Exec(
		`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`,
		revisionCounterMetaKey,
		"0",
	); err != nil {
		return fmt.Errorf("set index revision counter: %w", err)
	}
	return nil
}

func bumpRevision(store metaStore) error {
	if _, err := store.Exec(`
		INSERT INTO meta (key, value) VALUES (?, '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1
	`, revisionCounterMetaKey); err != nil {
		return fmt.Errorf("bump index revision: %w", err)
	}
	return nil
}

func newRevisionGeneration() (string, error) {
	var value [revisionGenerationBytes]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create index revision generation: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
