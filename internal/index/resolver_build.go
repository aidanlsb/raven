package index

import (
	"database/sql"
	"strings"

	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// AllObjectIDs returns all object IDs (for reference resolution).
func (d *Database) AllObjectIDs() ([]string, error) {
	return indexschema.AllObjectIDs(d.db)
}

// AllAliases returns a map from alias to object ID for all objects with aliases.
// This is used for reference resolution where [[alias]] should resolve to the object.
// If multiple objects have the same alias, the first one encountered in ID order wins.
// Use FindDuplicateAliases to detect and report conflicts.
func (d *Database) AllAliases() (map[string]string, error) {
	return indexschema.AllAliases(d.db)
}

// ResolverOptions is retained as an index-package compatibility alias.
type ResolverOptions = indexschema.ResolverOptions

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
	return indexschema.BuildResolver(db, opts)
}

type resolverQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func buildResolverSnapshot(db *sql.DB, opts ResolverOptions) (*resolver.Resolver, int64, error) {
	return indexschema.BuildResolverSnapshot(db, opts)
}

func defaultDailyDir(dailyDir string) string {
	return indexschema.DefaultDailyDirectory(dailyDir)
}

// AllNameFieldValues returns a map from name_field values to candidate object IDs.
// It queries each type's name_field and extracts the corresponding field value.
func (d *Database) AllNameFieldValues(sch *schema.Schema) (map[string][]string, error) {
	return indexschema.AllNameFieldValues(d.db, sch)
}

func allAliasesFromDB(db resolverQuerier) (map[string]string, error) {
	return indexschema.AllAliases(db)
}

func allAliasMatchesFromDB(db resolverQuerier) (map[string][]string, error) {
	return indexschema.AllAliasMatches(db)
}

func buildTypeNameFields(sch *schema.Schema) map[string]string {
	return indexschema.BuildTypeNameFields(sch)
}

func extractNameFieldValue(typeNameFields map[string]string, objType string, fieldsJSON string) (string, bool) {
	return indexschema.ExtractNameFieldValue(typeNameFields, objType, fieldsJSON)
}

// FindDuplicateAliases finds cases where multiple objects use the same alias.
// This is a validation issue that should be reported to the user.
func (d *Database) FindDuplicateAliases() ([]resolver.AliasCollision, error) {
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

	var duplicates []resolver.AliasCollision
	for rows.Next() {
		var alias, idsConcat string
		if err := rows.Scan(&alias, &idsConcat); err != nil {
			return nil, err
		}
		ids := strings.Split(idsConcat, "|")
		duplicates = append(duplicates, resolver.AliasCollision{
			Alias:         alias,
			ObjectIDs:     ids,
			ConflictsWith: "alias",
		})
	}

	return duplicates, rows.Err()
}
