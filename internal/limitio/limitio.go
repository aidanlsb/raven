// Package limitio reads bounded inputs so YAML/JSON parsers cannot be fed
// arbitrarily large vault or import files.
package limitio

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	// MaxFrontmatterBytes is the largest YAML frontmatter block Raven will parse.
	MaxFrontmatterBytes = 1 << 20
	// MaxSchemaBytes is the largest schema.yaml Raven will load.
	MaxSchemaBytes = 8 << 20
	// MaxConfigBytes is the largest raven.yaml Raven will load.
	MaxConfigBytes = 2 << 20
	// MaxImportBytes is the largest import JSON payload (file or stdin).
	MaxImportBytes = 32 << 20
	// MaxMappingBytes is the largest import mapping YAML file.
	MaxMappingBytes = 2 << 20
)

// ErrTooLarge is returned when an input exceeds the configured byte limit.
var ErrTooLarge = errors.New("input exceeds size limit")

// ReadAll reads from r up to limit bytes. A single extra byte past limit yields
// ErrTooLarge; the caller should not treat a truncated body as valid input.
func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("invalid size limit")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w (%d bytes)", ErrTooLarge, limit)
	}
	return data, nil
}

// ReadFile reads path with the same limit as ReadAll.
func ReadFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadAll(f, limit)
}
