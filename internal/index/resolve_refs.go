package index

import (
	"database/sql"
	"fmt"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// ReferenceResolutionResult contains statistics about reference resolution.
type ReferenceResolutionResult struct {
	Resolved   int // Number of references successfully resolved
	Unresolved int // Number of references that couldn't be resolved
	Ambiguous  int // Number of ambiguous references (multiple matches)
	Total      int // Total number of references processed

	FieldResolved   int // Number of field refs successfully resolved
	FieldUnresolved int // Number of field refs that couldn't be resolved
	FieldAmbiguous  int // Number of ambiguous field refs (multiple matches)
	FieldTotal      int // Total number of field refs processed
}

// ResolveReferences resolves all unresolved references in the refs table.
// This should be called after all files have been indexed.
// dailyDirectory is used to resolve date shorthand references like [[2025-02-01]].
func (d *Database) ResolveReferences(dailyDirectory string) (*ReferenceResolutionResult, error) {
	return d.ResolveReferencesWithSchema(dailyDirectory, nil)
}

// ResolveReferencesWithSchema resolves all unresolved references in the refs table
// using schema-aware name_field matching when schema is provided.
func (d *Database) ResolveReferencesWithSchema(dailyDirectory string, sch *schema.Schema) (*ReferenceResolutionResult, error) {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	return d.resolveReferencesWithSchemaLocked(nil, dailyDirectory, sch)
}

// ResolveReferencesForFileWithSchema resolves unresolved references for a single file
// using schema-aware name_field matching when schema is provided.
func (d *Database) ResolveReferencesForFileWithSchema(filePath, dailyDirectory string, sch *schema.Schema) (*ReferenceResolutionResult, error) {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	return d.resolveReferencesForFileWithSchemaLocked(filePath, dailyDirectory, sch)
}

func (d *Database) resolveReferencesForFileWithSchemaLocked(filePath, dailyDirectory string, sch *schema.Schema) (*ReferenceResolutionResult, error) {
	return d.resolveReferencesWithSchemaLocked(&filePath, dailyDirectory, sch)
}

func (d *Database) resolveReferencesWithSchemaLocked(filePath *string, dailyDirectory string, sch *schema.Schema) (*ReferenceResolutionResult, error) {
	result := &ReferenceResolutionResult{}

	hasUnresolved, err := d.hasUnresolvedReferencesLocked(filePath)
	if err != nil {
		return nil, err
	}
	if !hasUnresolved {
		return result, nil
	}

	res, err := d.getReferenceResolverLocked(dailyDirectory, sch)
	if err != nil {
		return nil, err
	}

	if err := d.resolveRefs(res, filePath, result); err != nil {
		return nil, err
	}
	if err := d.resolveFieldRefs(res, filePath, result); err != nil {
		return nil, err
	}

	return result, nil
}

const resolveRefsBatchSize = 750

const (
	resolutionStatusResolved  = "resolved"
	resolutionStatusAmbiguous = "ambiguous"
	resolutionStatusMissing   = "missing"
)

type unresolvedReference struct {
	id        int64
	targetRaw string
}

type referenceResolution struct {
	targetID string
	status   string
}

type referenceResolutionCounters struct {
	total      *int
	resolved   *int
	unresolved *int
	ambiguous  *int
}

func (c referenceResolutionCounters) record(resolution referenceResolution) {
	switch resolution.status {
	case resolutionStatusResolved:
		(*c.resolved)++
	case resolutionStatusAmbiguous:
		(*c.ambiguous)++
		(*c.unresolved)++
	case resolutionStatusMissing:
		(*c.unresolved)++
	}
}

type referenceBatchPlan struct {
	name              string
	fetchAllSQL       string
	fetchForFileSQL   string
	updateSQL         string
	counters          referenceResolutionCounters
	persistResolution func(*sql.Stmt, unresolvedReference, referenceResolution) error
}

func (d *Database) resolveRefs(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	return d.resolveReferenceBatches(res, filePath, referenceBatchPlan{
		name:            "refs",
		fetchAllSQL:     `SELECT id, target_raw FROM refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`,
		fetchForFileSQL: `SELECT id, target_raw FROM refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`,
		updateSQL:       `UPDATE refs SET target_id = ? WHERE id = ?`,
		counters: referenceResolutionCounters{
			total:      &result.Total,
			resolved:   &result.Resolved,
			unresolved: &result.Unresolved,
			ambiguous:  &result.Ambiguous,
		},
		persistResolution: func(stmt *sql.Stmt, ref unresolvedReference, resolution referenceResolution) error {
			if resolution.status != resolutionStatusResolved {
				return nil
			}
			_, err := stmt.Exec(resolution.targetID, ref.id)
			return err
		},
	})
}

func (d *Database) resolveFieldRefs(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	return d.resolveReferenceBatches(res, filePath, referenceBatchPlan{
		name:            "field refs",
		fetchAllSQL:     `SELECT id, target_raw FROM field_refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`,
		fetchForFileSQL: `SELECT id, target_raw FROM field_refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`,
		updateSQL:       `UPDATE field_refs SET target_id = ?, resolution_status = ? WHERE id = ?`,
		counters: referenceResolutionCounters{
			total:      &result.FieldTotal,
			resolved:   &result.FieldResolved,
			unresolved: &result.FieldUnresolved,
			ambiguous:  &result.FieldAmbiguous,
		},
		persistResolution: func(stmt *sql.Stmt, ref unresolvedReference, resolution referenceResolution) error {
			var targetID any
			if resolution.status == resolutionStatusResolved {
				targetID = resolution.targetID
			}
			_, err := stmt.Exec(targetID, resolution.status, ref.id)
			return err
		},
	})
}

func (d *Database) resolveReferenceBatches(res *resolver.Resolver, filePath *string, plan referenceBatchPlan) error {
	var lastID int64
	for {
		refs, err := d.fetchUnresolvedReferenceBatch(filePath, lastID, resolveRefsBatchSize, plan)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}

		if err := d.resolveReferenceBatch(res, refs, plan); err != nil {
			return err
		}

		lastID = refs[len(refs)-1].id
	}
}

func (d *Database) fetchUnresolvedReferenceBatch(
	filePath *string,
	afterID int64,
	limit int,
	plan referenceBatchPlan,
) ([]unresolvedReference, error) {
	query := plan.fetchAllSQL
	args := []any{afterID, limit}
	if filePath != nil {
		query = plan.fetchForFileSQL
		args = []any{*filePath, afterID, limit}
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", plan.name, err)
	}
	defer rows.Close()

	refs := make([]unresolvedReference, 0, limit)
	for rows.Next() {
		var r unresolvedReference
		if err := rows.Scan(&r.id, &r.targetRaw); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return refs, nil
}

func (d *Database) resolveReferenceBatch(
	res *resolver.Resolver,
	refs []unresolvedReference,
	plan referenceBatchPlan,
) error {
	*plan.counters.total += len(refs)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(plan.updateSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range refs {
		resolved := res.Resolve(ref.targetRaw)
		resolution := referenceResolution{
			targetID: resolved.TargetID,
			status:   resolutionStatusMissing,
		}
		if resolved.TargetID != "" {
			resolution.status = resolutionStatusResolved
		}
		if resolved.Ambiguous {
			resolution.targetID = ""
			resolution.status = resolutionStatusAmbiguous
		}

		plan.counters.record(resolution)
		if err := plan.persistResolution(stmt, ref, resolution); err != nil {
			return err
		}
	}

	return tx.Commit()
}
