package schemachange

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// RecordInvalidation analyzes a schema mutation and records the appropriate
// journal invalidation before the mutation commits. Returns the operation ID
// for tracking and the classification.
//
// This must be called before the schema write commits so recovery state is
// durable before the mutation takes effect. If recording fails, the mutation
// should abort.
//
// If auto-reindex is disabled, the journal entry persists until a manual
// reindex clears it.
func RecordInvalidation(vaultPath string, beforeSchema, afterSchema *schema.Schema) (string, Classification, error) {
	classification := Classify(Diff{Before: beforeSchema, After: afterSchema})

	switch classification.Policy {
	case PolicyNone:
		// No index work required
		return "", classification, nil

	case PolicyResolverRefresh:
		// Reference resolution changed, but we don't need to reparse markdown.
		// The resolver-refresh path currently requires a full scan to update
		// references properly, so we record it as full-scan.
		// Future optimization: differentiate resolver-only refresh from full reparse.
		operationID, err := indexjournal.RequireFullScan(vaultPath, "")
		if err != nil {
			return "", classification, fmt.Errorf("record resolver refresh invalidation: %w", err)
		}
		return operationID, classification, nil

	case PolicyFullScan:
		// Existing markdown must be reparsed into the current database.
		operationID, err := indexjournal.RequireFullScan(vaultPath, "")
		if err != nil {
			return "", classification, fmt.Errorf("record full scan invalidation: %w", err)
		}
		return operationID, classification, nil

	case PolicyFullRebuild:
		// Reserved for internal index version changes, not user schema mutations.
		// Treat as full scan for safety.
		operationID, err := indexjournal.RequireFullScan(vaultPath, "")
		if err != nil {
			return "", classification, fmt.Errorf("record full rebuild invalidation: %w", err)
		}
		return operationID, classification, nil

	default:
		// Unknown policy: fail safe with full scan
		operationID, err := indexjournal.RequireFullScan(vaultPath, "")
		if err != nil {
			return "", classification, fmt.Errorf("record unknown policy invalidation: %w", err)
		}
		return operationID, classification, nil
	}
}

// ReindexFunc is the function signature for running a smart reindex operation.
// The service layer should provide this to avoid schemachange depending on readsvc.
type ReindexFunc func(rt *vaultruntime.Runtime) error

// ApplyInvalidation performs the index refresh required by a schema mutation.
// This runs after the schema write commits when auto-reindex is enabled.
//
// If auto-reindex is disabled, this is a no-op and the journal entry persists
// until a manual reindex.
//
// The reindex parameter is optional but must be provided when auto-reindex is
// enabled. The caller (typically a service) should pass a function that calls
// readsvc.SmartReindex.
func ApplyInvalidation(rt *vaultruntime.Runtime, operationID string, classification Classification, reindex ReindexFunc) error {
	if rt == nil || rt.VaultCfg == nil {
		return nil
	}

	autoReindexEnabled := rt.VaultCfg.IsAutoReindexEnabled()
	if !autoReindexEnabled {
		// Journal entry stays; manual reindex will recover
		return nil
	}

	if operationID == "" {
		// No invalidation recorded (PolicyNone)
		return nil
	}

	if reindex == nil {
		// Auto-reindex is enabled but no reindex function provided.
		// This is a programming error in the service layer.
		return fmt.Errorf("auto-reindex enabled but no reindex function provided")
	}

	// Auto-reindex is enabled: run incremental refresh
	switch classification.Policy {
	case PolicyNone:
		// Nothing to do
		return nil

	case PolicyResolverRefresh, PolicyFullScan, PolicyFullRebuild:
		// Run smart reindex to recover the invalidated state
		if err := reindex(rt); err != nil {
			return fmt.Errorf("apply schema invalidation refresh: %w", err)
		}
		return nil

	default:
		// Unknown policy: attempt refresh
		if err := reindex(rt); err != nil {
			return fmt.Errorf("apply unknown policy refresh: %w", err)
		}
		return nil
	}
}
