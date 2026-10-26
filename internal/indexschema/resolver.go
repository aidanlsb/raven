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

	aliases, aliasMatches, err := AllAliasData(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases: %w", err)
	}

	objectIDs = appendExtraIDs(objectIDs, opts.ExtraIDs)

	resolverOpts := resolver.Options{
		DailyDirectory: dailyDir,
		Aliases:        aliases,
		AliasMatches:   aliasMatches,
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
	rows, err := db.Query("SELECT id FROM objects UNION SELECT id FROM sections")
	if err != nil {
		if isMissingRelationError(err, "sections") {
			return queryIDs(db, "SELECT id FROM objects")
		}
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

func queryIDs(db ResolverQuerier, query string) ([]string, error) {
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

// AllAliasData returns the deterministic first object ID per alias and the
// full multi-map used for ambiguity, from a single query.
func AllAliasData(db ResolverQuerier) (map[string]string, map[string][]string, error) {
	rows, err := db.Query("SELECT alias, id FROM objects WHERE alias IS NOT NULL AND alias != '' ORDER BY id")
	if err != nil {
		if isMissingRelationError(err, "objects") || isMissingColumnError(err, "alias") {
			return map[string]string{}, map[string][]string{}, nil
		}
		return nil, nil, err
	}
	defer rows.Close()

	aliases := make(map[string]string)
	aliasMatches := make(map[string][]string)
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, nil, err
		}
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = id
		}
		aliasMatches[alias] = append(aliasMatches[alias], id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return aliases, aliasMatches, nil
}

// AllAliases returns the deterministic first object ID for each alias.
func AllAliases(db ResolverQuerier) (map[string]string, error) {
	aliases, _, err := AllAliasData(db)
	return aliases, err
}

// AllAliasMatches returns every object ID for each alias.
func AllAliasMatches(db ResolverQuerier) (map[string][]string, error) {
	_, matches, err := AllAliasData(db)
	return matches, err
}

func isMissingRelationError(err error, name string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") && strings.Contains(msg, strings.ToLower(name))
}

func isMissingColumnError(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such column") && strings.Contains(msg, strings.ToLower(column))
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

	for objType, nameField := range typeNameFields {
		jsonPath := "$." + nameField
		rows, err := db.Query(
			`SELECT id, json_extract(fields, ?) FROM objects WHERE type = ? AND json_type(fields, ?) = 'text' ORDER BY id`,
			jsonPath, objType, jsonPath,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var name sql.NullString
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, err
			}
			if !name.Valid || name.String == "" {
				continue
			}
			nameFieldMap[name.String] = append(nameFieldMap[name.String], id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	return nameFieldMap, nil
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
