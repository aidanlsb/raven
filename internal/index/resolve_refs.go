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

	if err := d.resolveReferencesInBatches(res, filePath, result); err != nil {
		return nil, err
	}
	if err := d.resolveFieldRefsInBatches(res, filePath, result); err != nil {
		return nil, err
	}

	return result, nil
}

const resolveRefsBatchSize = 750

type refToResolve struct {
	id        int64
	targetRaw string
}

type fieldRefToResolve struct {
	id        int64
	targetRaw string
}

func (d *Database) resolveReferencesInBatches(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	var lastID int64
	for {
		refs, err := d.fetchUnresolvedRefsBatch(filePath, lastID, resolveRefsBatchSize)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}

		if err := d.resolveRefBatch(res, refs, result); err != nil {
			return err
		}

		lastID = refs[len(refs)-1].id
	}
}

func (d *Database) resolveFieldRefsInBatches(res *resolver.Resolver, filePath *string, result *ReferenceResolutionResult) error {
	var lastID int64
	for {
		refs, err := d.fetchUnresolvedFieldRefsBatch(filePath, lastID, resolveRefsBatchSize)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}

		if err := d.resolveFieldRefBatch(res, refs, result); err != nil {
			return err
		}

		lastID = refs[len(refs)-1].id
	}
}

func (d *Database) fetchUnresolvedRefsBatch(filePath *string, afterID int64, limit int) ([]refToResolve, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if filePath == nil {
		rows, err = d.db.Query(`SELECT id, target_raw FROM refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`, afterID, limit)
	} else {
		rows, err = d.db.Query(`SELECT id, target_raw FROM refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`, *filePath, afterID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query refs: %w", err)
	}
	defer rows.Close()

	refs := make([]refToResolve, 0, limit)
	for rows.Next() {
		var r refToResolve
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

func (d *Database) fetchUnresolvedFieldRefsBatch(filePath *string, afterID int64, limit int) ([]fieldRefToResolve, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if filePath == nil {
		rows, err = d.db.Query(`SELECT id, target_raw FROM field_refs WHERE target_id IS NULL AND id > ? ORDER BY id LIMIT ?`, afterID, limit)
	} else {
		rows, err = d.db.Query(`SELECT id, target_raw FROM field_refs WHERE target_id IS NULL AND file_path = ? AND id > ? ORDER BY id LIMIT ?`, *filePath, afterID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query field refs: %w", err)
	}
	defer rows.Close()

	refs := make([]fieldRefToResolve, 0, limit)
	for rows.Next() {
		var r fieldRefToResolve
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

func (d *Database) resolveRefBatch(res *resolver.Resolver, refs []refToResolve, result *ReferenceResolutionResult) error {
	result.Total += len(refs)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE refs SET target_id = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range refs {
		resolved := res.Resolve(ref.targetRaw)
		if resolved.Ambiguous {
			result.Ambiguous++
			result.Unresolved++
			continue
		}
		if resolved.TargetID != "" {
			if _, err := stmt.Exec(resolved.TargetID, ref.id); err != nil {
				return err
			}
			result.Resolved++
		} else {
			result.Unresolved++
		}
	}

	return tx.Commit()
}

func (d *Database) resolveFieldRefBatch(res *resolver.Resolver, refs []fieldRefToResolve, result *ReferenceResolutionResult) error {
	result.FieldTotal += len(refs)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE field_refs SET target_id = ?, resolution_status = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range refs {
		resolved := res.Resolve(ref.targetRaw)
		if resolved.Ambiguous {
			result.FieldAmbiguous++
			result.FieldUnresolved++
			if _, err := stmt.Exec(nil, "ambiguous", ref.id); err != nil {
				return err
			}
			continue
		}
		if resolved.TargetID != "" {
			if _, err := stmt.Exec(resolved.TargetID, "resolved", ref.id); err != nil {
				return err
			}
			result.FieldResolved++
		} else {
			if _, err := stmt.Exec(nil, "missing", ref.id); err != nil {
				return err
			}
			result.FieldUnresolved++
		}
	}

	return tx.Commit()
}
