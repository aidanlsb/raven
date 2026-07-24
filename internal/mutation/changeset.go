// Package mutation defines transport-neutral descriptions of durable vault
// file changes.
package mutation

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
)

// Move describes one vault-relative file move.
type Move struct {
	From string
	To   string
}

// ChangeSet describes files affected by an applied mutation. All paths are
// vault-relative and use forward slashes.
//
// Preview plans are deliberately separate from ChangeSet: a ChangeSet only
// describes durable writes that already happened.
type ChangeSet struct {
	Changed []string
	Deleted []string
	Moved   []Move
}

// NewChangeSet returns an empty applied-mutation change set.
func NewChangeSet() ChangeSet {
	return ChangeSet{}
}

// AddChanged records files that were created or updated.
func (c *ChangeSet) AddChanged(filePaths ...string) {
	if c == nil {
		return
	}
	for _, filePath := range filePaths {
		if normalized := normalizedPath(filePath); normalized != "" {
			c.Changed = appendUnique(c.Changed, normalized)
		}
	}
}

// AddDeleted records files that were removed from their managed locations.
func (c *ChangeSet) AddDeleted(filePaths ...string) {
	if c == nil {
		return
	}
	for _, filePath := range filePaths {
		if normalized := normalizedPath(filePath); normalized != "" {
			c.Deleted = appendUnique(c.Deleted, normalized)
		}
	}
}

// AddMoved records a file move from one managed location to another.
func (c *ChangeSet) AddMoved(from, to string) {
	if c == nil {
		return
	}
	from = normalizedPath(from)
	to = normalizedPath(to)
	if from == "" || to == "" {
		return
	}
	for _, move := range c.Moved {
		if move.From == from && move.To == to {
			return
		}
	}
	c.Moved = append(c.Moved, Move{From: from, To: to})
}

// Merge adds all changes from the provided change sets.
func (c *ChangeSet) Merge(changeSets ...ChangeSet) {
	if c == nil {
		return
	}
	for _, other := range changeSets {
		c.AddChanged(other.Changed...)
		c.AddDeleted(other.Deleted...)
		for _, move := range other.Moved {
			c.AddMoved(move.From, move.To)
		}
	}
}

// Empty reports whether the change set contains no durable file changes.
func (c ChangeSet) Empty() bool {
	return len(c.Changed) == 0 && len(c.Deleted) == 0 && len(c.Moved) == 0
}

// IndexPaths returns candidate files whose current contents should be
// projected into the index. Move destinations are included. The coordinator
// filters moved-away candidates against the filesystem so path reuse remains
// representable without coupling this transport-neutral type to I/O.
func (c ChangeSet) IndexPaths() []string {
	result := append([]string(nil), c.Changed...)
	for _, move := range c.Moved {
		result = appendUnique(result, move.To)
	}
	return result
}

// RemovedPaths returns old file locations that should be removed from the
// index. Move sources are included.
func (c ChangeSet) RemovedPaths() []string {
	result := append([]string(nil), c.Deleted...)
	for _, move := range c.Moved {
		result = appendUnique(result, move.From)
	}
	return result
}

func normalizedPath(filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filepath.IsAbs(filePath) {
		return ""
	}
	normalized := paths.NormalizeVaultRelPath(filePath)
	if !paths.IsValidVaultRelPath(normalized) {
		return ""
	}
	return normalized
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
