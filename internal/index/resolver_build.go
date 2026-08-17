package index

import (
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
// If multiple objects have the same alias, the first one encountered in ID order wins.
func (d *Database) AllAliases() (map[string]string, error) {
	return indexschema.AllAliases(d.db)
}

// Resolver builds the canonical resolver for this vault index.
func (d *Database) Resolver(opts indexschema.ResolverOptions) (*resolver.Resolver, error) {
	return indexschema.BuildResolver(d.db, opts)
}

// AllNameFieldValues returns a map from name_field values to candidate object IDs.
// It queries each type's name_field and extracts the corresponding field value.
func (d *Database) AllNameFieldValues(sch *schema.Schema) (map[string][]string, error) {
	return indexschema.AllNameFieldValues(d.db, sch)
}

// FindDuplicateAliases finds cases where multiple objects use the same alias.
func (d *Database) FindDuplicateAliases() ([]resolver.AliasCollision, error) {
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
