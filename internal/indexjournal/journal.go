// Package indexjournal tracks derived index work that has not completed yet.
//
// The journal is disposable cache metadata. Markdown and managed asset files
// remain the source of truth, and a successful reindex can always discard
// journal entries after projecting their contents.
package indexjournal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/filelock"
	"github.com/aidanlsb/raven/internal/paths"
)

const (
	// Filename is the journal's location relative to .raven.
	Filename = "index-dirty.json"

	journalVersion = 1
	lockFilename   = "index-dirty.lock"
	operationDir   = "index-dirty-operations"
)

var activeOperations = struct {
	sync.Mutex
	files map[string]*os.File
}{files: make(map[string]*os.File)}

// Operation is one in-flight or interrupted mutation's pending index work.
// Unknown is true between the write-ahead guard and the point where the
// mutation's concrete changed paths are known.
type Operation struct {
	ID       string   `json:"id"`
	Revision uint64   `json:"revision"`
	Unknown  bool     `json:"unknown,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	// Active is snapshot-only state: the originating process held the
	// operation lock when Load observed this entry.
	Active bool `json:"-"`
}

// Snapshot is an immutable view of the pending journal.
type Snapshot struct {
	Operations []Operation
}

// Dirty reports whether any projection work is pending.
func (s Snapshot) Dirty() bool {
	return len(s.Operations) > 0
}

// RequiresFullScan reports whether a mutation was interrupted before its
// concrete changed paths were recorded.
func (s Snapshot) RequiresFullScan() bool {
	for _, operation := range s.Operations {
		if operation.Unknown {
			return true
		}
	}
	return false
}

// Paths returns the sorted union of known pending paths.
func (s Snapshot) Paths() []string {
	seen := make(map[string]struct{})
	for _, operation := range s.Operations {
		for _, relPath := range operation.Paths {
			seen[relPath] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for relPath := range seen {
		result = append(result, relPath)
	}
	sort.Strings(result)
	return result
}

// Begin records a write-ahead guard before a mutation starts.
func Begin(vaultPath string) (string, error) {
	operationID, err := newOperationID()
	if err != nil {
		return "", err
	}
	guardFile, err := acquireOperationGuard(vaultPath, operationID)
	if err != nil {
		return "", err
	}
	err = update(vaultPath, func(journal *journalFile) error {
		journal.Operations[operationID] = Operation{ID: operationID, Revision: 1, Unknown: true}
		return nil
	})
	if err != nil {
		_ = releaseOperationGuard(vaultPath, operationID, guardFile)
		return "", err
	}
	activeOperations.Lock()
	activeOperations.files[operationID] = guardFile
	activeOperations.Unlock()
	return operationID, nil
}

// SetPaths replaces an operation's write-ahead guard with its concrete pending
// paths. If operationID is empty, a new operation is created. Empty paths
// complete the operation, which is used for successful no-op mutations.
func SetPaths(vaultPath, operationID string, relPaths []string) (string, error) {
	defer func() { _ = releaseActiveOperation(vaultPath, operationID) }()
	if operationID != "" && !validOperationID(operationID) {
		return "", fmt.Errorf("invalid index dirty operation %q", operationID)
	}
	normalized, err := normalizePaths(relPaths)
	if err != nil {
		return "", err
	}
	if operationID == "" && len(normalized) > 0 {
		operationID, err = newOperationID()
		if err != nil {
			return "", err
		}
	}
	err = update(vaultPath, func(journal *journalFile) error {
		if len(normalized) == 0 {
			delete(journal.Operations, operationID)
			return nil
		}
		revision := uint64(1)
		if current, ok := journal.Operations[operationID]; ok {
			revision = current.Revision + 1
		}
		journal.Operations[operationID] = Operation{
			ID:       operationID,
			Revision: revision,
			Paths:    normalized,
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return operationID, nil
}

// ClearPaths removes successfully projected paths from one operation. The
// operation is removed when no work remains.
func ClearPaths(vaultPath, operationID string, relPaths ...string) error {
	if operationID == "" || len(relPaths) == 0 {
		return nil
	}
	if !validOperationID(operationID) {
		return fmt.Errorf("invalid index dirty operation %q", operationID)
	}
	normalized, err := normalizePaths(relPaths)
	if err != nil {
		return err
	}
	return update(vaultPath, func(journal *journalFile) error {
		operation, ok := journal.Operations[operationID]
		if !ok || operation.Unknown {
			return nil
		}
		cleared := make(map[string]struct{}, len(normalized))
		for _, relPath := range normalized {
			cleared[relPath] = struct{}{}
		}
		remaining := operation.Paths[:0]
		for _, relPath := range operation.Paths {
			if _, ok := cleared[relPath]; !ok {
				remaining = append(remaining, relPath)
			}
		}
		if len(remaining) == 0 {
			delete(journal.Operations, operationID)
			return nil
		}
		operation.Paths = append([]string(nil), remaining...)
		journal.Operations[operationID] = operation
		return nil
	})
}

// CancelUnknown removes a write-ahead guard when a handler returns before any
// durable mutation. If the operation has already advanced to concrete paths it
// is preserved.
func CancelUnknown(vaultPath, operationID string) error {
	if operationID == "" {
		return nil
	}
	defer func() { _ = releaseActiveOperation(vaultPath, operationID) }()
	if !validOperationID(operationID) {
		return fmt.Errorf("invalid index dirty operation %q", operationID)
	}
	return update(vaultPath, func(journal *journalFile) error {
		operation, ok := journal.Operations[operationID]
		if ok && operation.Unknown {
			delete(journal.Operations, operationID)
		}
		return nil
	})
}

// Abandon releases this process's active-operation lock while preserving the
// unknown journal entry. It is used when a handler fails because the failure
// may have followed a partial multi-file write.
func Abandon(vaultPath, operationID string) error {
	if operationID == "" {
		return nil
	}
	updateErr := update(vaultPath, func(journal *journalFile) error {
		operation, ok := journal.Operations[operationID]
		if ok {
			operation.Revision++
			journal.Operations[operationID] = operation
		}
		return nil
	})
	if updateErr != nil {
		// Keep the operation lock held. Releasing an unchanged unknown revision
		// could let an overlapping older recovery scan clear partial writes.
		return updateErr
	}
	return releaseActiveOperation(vaultPath, operationID)
}

// CompleteIfUnchanged removes an operation only if it still matches a recovery
// snapshot. A concurrent mutation that advances the same operation from
// unknown to concrete paths therefore cannot be cleared by an older scan.
func CompleteIfUnchanged(vaultPath string, recovered Operation) error {
	if recovered.ID == "" {
		return nil
	}
	return update(vaultPath, func(journal *journalFile) error {
		current, ok := journal.Operations[recovered.ID]
		if ok && operationsEqual(current, recovered) {
			delete(journal.Operations, recovered.ID)
		}
		return nil
	})
}

// ClearRecoveredPath clears path from every concrete operation represented in
// snapshot. Operations created after the snapshot are not affected.
func ClearRecoveredPath(vaultPath string, snapshot Snapshot, relPath string) error {
	for _, operation := range snapshot.Operations {
		if operation.Unknown || !operationContainsPath(operation, relPath) {
			continue
		}
		if err := ClearPaths(vaultPath, operation.ID, relPath); err != nil {
			return err
		}
	}
	return nil
}

// CompleteRecoveredUnknown completes unknown operations that still match the
// supplied snapshot after a successful full scan.
func CompleteRecoveredUnknown(vaultPath string, snapshot Snapshot) error {
	for _, operation := range snapshot.Operations {
		if !operation.Unknown || operation.Active {
			continue
		}
		if err := completeUnknownIfInactive(vaultPath, operation); err != nil {
			return err
		}
	}
	return nil
}

// CompleteRecoveredSnapshot completes every operation that still matches a
// snapshot after a successful full rebuild.
func CompleteRecoveredSnapshot(vaultPath string, snapshot Snapshot) error {
	for _, operation := range snapshot.Operations {
		var err error
		if operation.Unknown && !operation.Active {
			err = completeUnknownIfInactive(vaultPath, operation)
		} else if !operation.Unknown {
			err = CompleteIfUnchanged(vaultPath, operation)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Load returns the current journal snapshot. A missing journal is clean.
func Load(vaultPath string) (Snapshot, error) {
	var snapshot Snapshot
	err := withLock(vaultPath, func(journalPath string) error {
		journal, err := readJournal(journalPath)
		if err != nil {
			return err
		}
		snapshot.Operations = make([]Operation, 0, len(journal.Operations))
		for _, operation := range journal.Operations {
			operation.Paths = append([]string(nil), operation.Paths...)
			if operation.Unknown {
				active, err := operationIsActive(vaultPath, operation.ID)
				if err != nil {
					return err
				}
				operation.Active = active
			}
			snapshot.Operations = append(snapshot.Operations, operation)
		}
		sort.Slice(snapshot.Operations, func(i, j int) bool {
			return snapshot.Operations[i].ID < snapshot.Operations[j].ID
		})
		return nil
	})
	return snapshot, err
}

type journalFile struct {
	Version    int                  `json:"version"`
	Operations map[string]Operation `json:"operations"`
}

func update(vaultPath string, mutate func(*journalFile) error) error {
	return withLock(vaultPath, func(journalPath string) error {
		journal, err := readJournal(journalPath)
		if err != nil {
			return err
		}
		if err := mutate(&journal); err != nil {
			return err
		}
		if len(journal.Operations) == 0 {
			if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove index dirty journal: %w", err)
			}
			return nil
		}
		data, err := json.MarshalIndent(journal, "", "  ")
		if err != nil {
			return fmt.Errorf("encode index dirty journal: %w", err)
		}
		data = append(data, '\n')
		if err := atomicfile.WriteFile(journalPath, data, 0o644); err != nil {
			return fmt.Errorf("write index dirty journal: %w", err)
		}
		return nil
	})
}

func withLock(vaultPath string, fn func(journalPath string) error) error {
	if vaultPath == "" {
		return fmt.Errorf("vault path is required")
	}
	ravenDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(ravenDir, 0o755); err != nil {
		return fmt.Errorf("create .raven directory: %w", err)
	}
	lockPath := filepath.Join(ravenDir, lockFilename)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open index dirty journal lock: %w", err)
	}
	defer lockFile.Close()
	if err := filelock.LockExclusive(lockFile); err != nil {
		return fmt.Errorf("lock index dirty journal: %w", err)
	}
	defer func() { _ = filelock.Unlock(lockFile) }()
	return fn(filepath.Join(ravenDir, Filename))
}

func readJournal(journalPath string) (journalFile, error) {
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return journalFile{
			Version:    journalVersion,
			Operations: make(map[string]Operation),
		}, nil
	}
	if err != nil {
		return journalFile{}, fmt.Errorf("read index dirty journal: %w", err)
	}
	var journal journalFile
	if err := json.Unmarshal(data, &journal); err != nil {
		return journalFile{}, fmt.Errorf("decode index dirty journal: %w", err)
	}
	if journal.Version != journalVersion {
		return journalFile{}, fmt.Errorf("unsupported index dirty journal version %d", journal.Version)
	}
	if journal.Operations == nil {
		journal.Operations = make(map[string]Operation)
	}
	for operationID, operation := range journal.Operations {
		if !validOperationID(operationID) || operation.ID != operationID || operation.Revision == 0 {
			return journalFile{}, fmt.Errorf("invalid index dirty journal operation %q", operationID)
		}
		if operation.Unknown && len(operation.Paths) > 0 {
			return journalFile{}, fmt.Errorf("unknown index dirty operation %q has concrete paths", operationID)
		}
		normalized, err := normalizePaths(operation.Paths)
		if err != nil {
			return journalFile{}, fmt.Errorf("invalid index dirty operation %q: %w", operationID, err)
		}
		operation.Paths = normalized
		journal.Operations[operationID] = operation
	}
	return journal, nil
}

func normalizePaths(relPaths []string) ([]string, error) {
	seen := make(map[string]struct{}, len(relPaths))
	result := make([]string, 0, len(relPaths))
	for _, relPath := range relPaths {
		normalized := paths.NormalizeVaultRelPath(relPath)
		if normalized == "" || normalized != relPath || !paths.IsValidVaultRelPath(normalized) {
			return nil, fmt.Errorf("invalid vault-relative path %q", relPath)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result, nil
}

func newOperationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create index dirty operation ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func validOperationID(operationID string) bool {
	if len(operationID) != 32 {
		return false
	}
	_, err := hex.DecodeString(operationID)
	return err == nil
}

func acquireOperationGuard(vaultPath, operationID string) (*os.File, error) {
	lockDir := filepath.Join(vaultPath, ".raven", operationDir)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create index dirty operation lock directory: %w", err)
	}
	lockPath := filepath.Join(lockDir, operationID+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open index dirty operation lock: %w", err)
	}
	if err := filelock.LockExclusive(lockFile); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("lock index dirty operation: %w", err)
	}
	return lockFile, nil
}

func completeUnknownIfInactive(vaultPath string, operation Operation) error {
	lockDir := filepath.Join(vaultPath, ".raven", operationDir)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return fmt.Errorf("create index dirty operation lock directory: %w", err)
	}
	lockPath := filepath.Join(lockDir, operation.ID+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open index dirty operation lock: %w", err)
	}
	if err := filelock.TryLockExclusive(lockFile); err != nil {
		_ = lockFile.Close()
		if filelock.IsWouldBlock(err) {
			return nil
		}
		return fmt.Errorf("lock index dirty operation for recovery: %w", err)
	}
	completeErr := CompleteIfUnchanged(vaultPath, operation)
	releaseErr := releaseOperationGuard(vaultPath, operation.ID, lockFile)
	return errors.Join(completeErr, releaseErr)
}

func operationIsActive(vaultPath, operationID string) (bool, error) {
	lockDir := filepath.Join(vaultPath, ".raven", operationDir)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return false, fmt.Errorf("create index dirty operation lock directory: %w", err)
	}
	lockPath := filepath.Join(lockDir, operationID+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false, fmt.Errorf("open index dirty operation lock: %w", err)
	}
	if err := filelock.TryLockExclusive(lockFile); err != nil {
		_ = lockFile.Close()
		if filelock.IsWouldBlock(err) {
			return true, nil
		}
		return false, fmt.Errorf("inspect index dirty operation lock: %w", err)
	}
	if err := releaseOperationGuard(vaultPath, operationID, lockFile); err != nil {
		return false, err
	}
	return false, nil
}

func releaseActiveOperation(vaultPath, operationID string) error {
	if operationID == "" {
		return nil
	}
	activeOperations.Lock()
	lockFile := activeOperations.files[operationID]
	delete(activeOperations.files, operationID)
	activeOperations.Unlock()
	if lockFile != nil {
		return releaseOperationGuard(vaultPath, operationID, lockFile)
	}
	return nil
}

func releaseOperationGuard(vaultPath, operationID string, lockFile *os.File) error {
	if lockFile == nil {
		return nil
	}
	unlockErr := filelock.Unlock(lockFile)
	closeErr := lockFile.Close()
	removeErr := os.Remove(filepath.Join(vaultPath, ".raven", operationDir, operationID+".lock"))
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(unlockErr, closeErr, removeErr)
}

func operationsEqual(left, right Operation) bool {
	if left.ID != right.ID || left.Revision != right.Revision || left.Unknown != right.Unknown || len(left.Paths) != len(right.Paths) {
		return false
	}
	for i := range left.Paths {
		if left.Paths[i] != right.Paths[i] {
			return false
		}
	}
	return true
}

func operationContainsPath(operation Operation, relPath string) bool {
	for _, pendingPath := range operation.Paths {
		if pendingPath == relPath {
			return true
		}
	}
	return false
}
