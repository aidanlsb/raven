package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// AllObjectIDs returns all object IDs (for reference resolution).
func (d *Database) AllObjectIDs() ([]string, error) {
	return allObjectIDsFromDB(d.db)
}

// AllAliases returns a map from alias to object ID for all objects with aliases.
// This is used for reference resolution where [[alias]] should resolve to the object.
// If multiple objects have the same alias, the first one encountered in ID order wins.
// Use FindDuplicateAliases to detect and report conflicts.
func (d *Database) AllAliases() (map[string]string, error) {
	return allAliasesFromDB(d.db)
}

// ResolverOptions configures resolver creation.
type ResolverOptions struct {
	// DailyDirectory is the directory for daily notes (default: "daily").
	DailyDirectory string

	// Schema enables name_field resolution for semantic matching.
	// When provided, [[The Prose Edda]] can resolve to books/the-prose-edda
	// if the book type has name_field: title.
	Schema *schema.Schema

	// ExtraIDs are additional object IDs to include in the resolver.
	// Useful for hypothetical resolution (e.g., testing if refs will
	// resolve after a move operation).
	ExtraIDs []string

	// ExtraAssetIDs are additional asset IDs to include in the resolver.
	// Useful for hypothetical asset moves.
	ExtraAssetIDs []string
}

// Resolver builds the canonical resolver for this vault index.
//
// This is the ONE resolver factory that handles all cases:
// - Object IDs (full path + short name resolution)
// - Aliases (e.g., [[The Queen]] → people/freya)
// - Name field values (e.g., [[The Prose Edda]] → books/the-prose-edda) - when Schema provided
// - Date shorthand (e.g., [[2025-02-01]] → 2025-02-01)
// - Extra IDs for hypothetical resolution
//
// Use this method for all resolver creation to ensure consistent behavior.
func (d *Database) Resolver(opts ResolverOptions) (*resolver.Resolver, error) {
	return BuildResolver(d.db, opts)
}

// BuildResolver builds the canonical resolver from a database handle.
//
// This shared helper allows all subsystems (index/query/check) to use identical
// resolver semantics without re-implementing object ID and alias loading logic.
func BuildResolver(db *sql.DB, opts ResolverOptions) (*resolver.Resolver, error) {
	res, _, err := buildResolverSnapshot(db, opts)
	return res, err
}

type resolverQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func buildResolverSnapshot(db *sql.DB, opts ResolverOptions) (*resolver.Resolver, int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	generation, err := resolverGeneration(tx)
	if err != nil {
		return nil, 0, err
	}
	res, err := buildResolver(tx, opts)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return res, generation, nil
}

func buildResolver(db resolverQuerier, opts ResolverOptions) (*resolver.Resolver, error) {
	dailyDir := defaultDailyDir(opts.DailyDirectory)

	objectIDs, err := allObjectIDsFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get object IDs: %w", err)
	}
	assetIDs, err := allAssetIDsFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset IDs: %w", err)
	}

	aliases, err := allAliasesFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases: %w", err)
	}
	aliasMatches, err := allAliasMatchesFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get alias matches: %w", err)
	}

	// Add extra IDs if provided (for hypothetical resolution)
	objectIDs = appendExtraIDs(objectIDs, opts.ExtraIDs)
	assetIDs = appendExtraIDs(assetIDs, opts.ExtraAssetIDs)

	// Include name_field values if schema is provided
	resolverOpts := resolver.Options{
		DailyDirectory: dailyDir,
		Aliases:        aliases,
		AliasMatches:   aliasMatches,
		AssetIDs:       assetIDs,
	}
	if opts.Schema != nil {
		nameFieldMap, err := allNameFieldValuesFromDB(db, opts.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to get name field values: %w", err)
		}
		resolverOpts.NameFieldMap = nameFieldMap
	}

	return resolver.New(objectIDs, resolverOpts), nil
}

func defaultDailyDir(dailyDir string) string {
	if dailyDir == "" {
		return "daily"
	}
	return dailyDir
}

// appendExtraIDs appends extra IDs to objectIDs, preserving order and de-duplicating.
// Empty extra IDs are ignored.
func appendExtraIDs(objectIDs []string, extraIDs []string) []string {
	if len(extraIDs) == 0 {
		return objectIDs
	}
	seen := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		seen[id] = struct{}{}
	}
	for _, id := range extraIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		objectIDs = append(objectIDs, id)
		seen[id] = struct{}{}
	}
	return objectIDs
}

// AllNameFieldValues returns a map from name_field values to candidate object IDs.
// It queries each type's name_field and extracts the corresponding field value.
func (d *Database) AllNameFieldValues(sch *schema.Schema) (map[string][]string, error) {
	return allNameFieldValuesFromDB(d.db, sch)
}

func allObjectIDsFromDB(db resolverQuerier) ([]string, error) {
	query := "SELECT id FROM objects"
	hasSections, err := objectsTableHasColumn(db, "sections", "id")
	if err != nil {
		return nil, err
	}
	if hasSections {
		query += "\nUNION\nSELECT id FROM sections"
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func allAssetIDsFromDB(db resolverQuerier) ([]string, error) {
	exists, err := objectsTableHasColumn(db, "assets", "id")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	rows, err := db.Query("SELECT id FROM assets")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func allAliasesFromDB(db resolverQuerier) (map[string]string, error) {
	hasAliasColumn, err := objectsTableHasColumn(db, "objects", "alias")
	if err != nil {
		return nil, err
	}
	if !hasAliasColumn {
		return map[string]string{}, nil
	}

	rows, err := db.Query("SELECT alias, id FROM objects WHERE alias IS NOT NULL AND alias != '' ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make(map[string]string)
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, err
		}
		// First one wins (deterministic due to ORDER BY id).
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = id
		}
	}

	return aliases, rows.Err()
}

func allAliasMatchesFromDB(db resolverQuerier) (map[string][]string, error) {
	hasAliasColumn, err := objectsTableHasColumn(db, "objects", "alias")
	if err != nil {
		return nil, err
	}
	if !hasAliasColumn {
		return map[string][]string{}, nil
	}

	rows, err := db.Query("SELECT alias, id FROM objects WHERE alias IS NOT NULL AND alias != '' ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliasMatches := make(map[string][]string)
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, err
		}
		aliasMatches[alias] = append(aliasMatches[alias], id)
	}

	return aliasMatches, rows.Err()
}

func objectsTableHasColumn(db resolverQuerier, tableName, columnName string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}

	return false, rows.Err()
}

func allNameFieldValuesFromDB(db resolverQuerier, sch *schema.Schema) (map[string][]string, error) {
	nameFieldMap := make(map[string][]string)

	if sch == nil {
		return nameFieldMap, nil
	}

	// Build a map of type -> name_field
	typeNameFields := buildTypeNameFields(sch)

	if len(typeNameFields) == 0 {
		return nameFieldMap, nil
	}

	// Query all objects and extract name_field values
	rows, err := db.Query(`SELECT id, type, fields FROM objects WHERE type != '' AND fields != '{}' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		id, objType, fieldsJSON, ok := scanNameFieldRow(rows)
		if !ok {
			continue
		}
		nameStr, ok := extractNameFieldValue(typeNameFields, objType, fieldsJSON)
		if !ok {
			continue
		}
		nameFieldMap[nameStr] = append(nameFieldMap[nameStr], id)
	}

	return nameFieldMap, rows.Err()
}

func buildTypeNameFields(sch *schema.Schema) map[string]string {
	typeNameFields := make(map[string]string)
	if sch == nil {
		return typeNameFields
	}
	for typeName, typeDef := range sch.Types {
		if typeDef != nil && typeDef.NameField != "" {
			typeNameFields[typeName] = typeDef.NameField
		}
	}
	return typeNameFields
}

func scanNameFieldRow(rows *sql.Rows) (id string, objType string, fieldsJSON string, ok bool) {
	if err := rows.Scan(&id, &objType, &fieldsJSON); err != nil {
		return "", "", "", false
	}
	return id, objType, fieldsJSON, true
}

func extractNameFieldValue(typeNameFields map[string]string, objType string, fieldsJSON string) (string, bool) {
	nameField, ok := typeNameFields[objType]
	if !ok {
		return "", false
	}

	// Parse fields JSON and extract name_field value
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return "", false
	}
	nameValue, ok := fields[nameField]
	if !ok {
		return "", false
	}
	nameStr, ok := nameValue.(string)
	if !ok || nameStr == "" {
		return "", false
	}
	return nameStr, true
}

// DuplicateAlias represents multiple objects sharing the same alias.
type DuplicateAlias struct {
	Alias     string   // The duplicated alias
	ObjectIDs []string // All object IDs using this alias
}

// FindDuplicateAliases finds cases where multiple objects use the same alias.
// This is a validation issue that should be reported to the user.
func (d *Database) FindDuplicateAliases() ([]DuplicateAlias, error) {
	// Find aliases that appear more than once
	rows, err := d.db.Query(`
		SELECT alias, GROUP_CONCAT(id, '|') as ids
		FROM objects
		WHERE alias IS NOT NULL AND alias != ''
		GROUP BY alias
		HAVING COUNT(*) > 1
		ORDER BY alias
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var duplicates []DuplicateAlias
	for rows.Next() {
		var alias, idsConcat string
		if err := rows.Scan(&alias, &idsConcat); err != nil {
			return nil, err
		}
		ids := strings.Split(idsConcat, "|")
		duplicates = append(duplicates, DuplicateAlias{
			Alias:     alias,
			ObjectIDs: ids,
		})
	}

	return duplicates, rows.Err()
}
