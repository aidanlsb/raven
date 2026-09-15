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

	// Field* counts are derived from rows with field_name set.
	FieldResolved   int
	FieldUnresolved int
	FieldAmbiguous  int
	FieldTotal      int
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
	fieldName sql.NullString
}

type referenceResolution struct {
	targetID string
	status   string
}

func (r *ReferenceResolutionResult) record(resolution referenceResolution, fieldRef bool) {
	r.Total++
	recordResolutionCounters(&r.Resolved, &r.Unresolved, &r.Ambiguous, resolution)
	if !fieldRef {
		return
	}
	r.FieldTotal++
	recordResolutionCounters(&r.FieldResolved, &r.FieldUnresolved, &r.FieldAmbiguous, resolution)
}

func recordResolutionCounters(resolved, unresolved, ambiguous *int, resolution referenceResolution) {
	switch resolution.status {
	case resolutionStatusResolved:
		(*resolved)++
	case resolutionStatusAmbiguous:
		(*ambiguous)++
		(*unresolved)++
	case resolutionStatusMissing:
		(*unresolved)++
	}
}

func (d *Database) resolveRefs(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	var lastID int64
	for {
		refs, err := d.fetchUnresolvedReferenceBatch(filePath, lastID, resolveRefsBatchSize)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}

		if err := d.resolveReferenceBatch(res, refs, result); err != nil {
			return err
		}

		lastID = refs[len(refs)-1].id
	}
}

func (d *Database) fetchUnresolvedReferenceBatch(
	filePath *string,
	afterID int64,
	limit int,
) ([]unresolvedReference, error) {
	query := `SELECT id, target_raw, field_name FROM refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`
	args := []any{afterID, limit}
	if filePath != nil {
		query = `SELECT id, target_raw, field_name FROM refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`
		args = []any{*filePath, afterID, limit}
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query refs: %w", err)
	}
	defer rows.Close()

	refs := make([]unresolvedReference, 0, limit)
	for rows.Next() {
		var r unresolvedReference
		if err := rows.Scan(&r.id, &r.targetRaw, &r.fieldName); err != nil {
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
	result *ReferenceResolutionResult,
) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE refs SET target_id = ?, resolution_status = ? WHERE id = ?`)
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

		result.record(resolution, ref.fieldName.Valid && ref.fieldName.String != "")

		var targetID any
		if resolution.status == resolutionStatusResolved {
			targetID = resolution.targetID
		}
		if _, err := stmt.Exec(targetID, resolution.status, ref.id); err != nil {
			return err
		}
	}

	return tx.Commit()
}
