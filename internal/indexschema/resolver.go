package indexschema

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// ResolverGenerationMetaKey stores the durable resolver cache generation.
const ResolverGenerationMetaKey = "resolver_generation"

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

// ResolverQuerier is the database surface required to build a resolver.
type ResolverQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// BuildResolver builds the canonical resolver from a database handle.
func BuildResolver(db *sql.DB, opts ResolverOptions) (*resolver.Resolver, error) {
	res, _, err := BuildResolverSnapshot(db, opts)
	return res, err
}

// BuildResolverSnapshot builds a resolver and reads its durable generation from
// one consistent database snapshot.
func BuildResolverSnapshot(db *sql.DB, opts ResolverOptions) (*resolver.Resolver, int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	generation, err := ResolverGeneration(tx)
	if err != nil {
		return nil, 0, err
	}
	res, err := BuildResolverFromQuerier(tx, opts)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return res, generation, nil
}

// BuildResolverFromQuerier builds the canonical resolver from an existing
// database snapshot.
func BuildResolverFromQuerier(db ResolverQuerier, opts ResolverOptions) (*resolver.Resolver, error) {
	dailyDir := DefaultDailyDirectory(opts.DailyDirectory)

	objectIDs, err := AllObjectIDs(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get object IDs: %w", err)
	}
	assetIDs, err := AllAssetIDs(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset IDs: %w", err)
	}

	aliases, err := AllAliases(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases: %w", err)
	}
	aliasMatches, err := AllAliasMatches(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get alias matches: %w", err)
	}

	objectIDs = appendExtraIDs(objectIDs, opts.ExtraIDs)
	assetIDs = appendExtraIDs(assetIDs, opts.ExtraAssetIDs)

	resolverOpts := resolver.Options{
		DailyDirectory: dailyDir,
		Aliases:        aliases,
		AliasMatches:   aliasMatches,
		AssetIDs:       assetIDs,
	}
	if opts.Schema != nil {
		nameFieldMap, err := AllNameFieldValues(db, opts.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to get name field values: %w", err)
		}
		resolverOpts.NameFieldMap = nameFieldMap
	}

	return resolver.New(objectIDs, resolverOpts), nil
}

// DefaultDailyDirectory applies the resolver's default daily-note directory.
func DefaultDailyDirectory(dailyDir string) string {
	if dailyDir == "" {
		return "daily"
	}
	return dailyDir
}

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

// ResolverGeneration returns the durable generation for resolver-relevant
// index state.
func ResolverGeneration(db ResolverQuerier) (int64, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, ResolverGenerationMetaKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		// BuildResolver also supports the simplified legacy/test databases used
		// by query callers, which may not include Raven's meta table.
		if strings.Contains(err.Error(), "no such table: meta") {
			return 0, nil
		}
		return 0, fmt.Errorf("read resolver generation: %w", err)
	}
	generation, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse resolver generation %q: %w", raw, err)
	}
	return generation, nil
}

// AllObjectIDs returns all object and section IDs available to the resolver.
func AllObjectIDs(db ResolverQuerier) ([]string, error) {
	query := "SELECT id FROM objects"
	hasSections, err := TableHasColumn(db, "sections", "id")
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

// AllAssetIDs returns all asset IDs available to the resolver.
func AllAssetIDs(db ResolverQuerier) ([]string, error) {
	exists, err := TableHasColumn(db, "assets", "id")
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

// AllAliases returns the deterministic first object ID for each alias.
func AllAliases(db ResolverQuerier) (map[string]string, error) {
	hasAliasColumn, err := TableHasColumn(db, "objects", "alias")
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
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = id
		}
	}

	return aliases, rows.Err()
}

// AllAliasMatches returns every object ID for each alias.
func AllAliasMatches(db ResolverQuerier) (map[string][]string, error) {
	hasAliasColumn, err := TableHasColumn(db, "objects", "alias")
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

// TableHasColumn reports whether a SQLite table exposes a column.
func TableHasColumn(db ResolverQuerier, tableName, columnName string) (bool, error) {
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

// AllNameFieldValues returns name-field values mapped to candidate object IDs.
func AllNameFieldValues(db ResolverQuerier, sch *schema.Schema) (map[string][]string, error) {
	nameFieldMap := make(map[string][]string)

	if sch == nil {
		return nameFieldMap, nil
	}

	typeNameFields := BuildTypeNameFields(sch)
	if len(typeNameFields) == 0 {
		return nameFieldMap, nil
	}

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
		nameStr, ok := ExtractNameFieldValue(typeNameFields, objType, fieldsJSON)
		if !ok {
			continue
		}
		nameFieldMap[nameStr] = append(nameFieldMap[nameStr], id)
	}

	return nameFieldMap, rows.Err()
}

// BuildTypeNameFields maps object types to their configured name fields.
func BuildTypeNameFields(sch *schema.Schema) map[string]string {
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

// ExtractNameFieldValue extracts an object's configured name-field value.
func ExtractNameFieldValue(typeNameFields map[string]string, objType string, fieldsJSON string) (string, bool) {
	nameField, ok := typeNameFields[objType]
	if !ok {
		return "", false
	}

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
