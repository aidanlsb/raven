package schemasvc

import (
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
)

// objectsByType returns every indexed object of the given type via the RQL
// executor. Schema maintenance (remove/update guards) uses it to detect whether
// a type still has instances before a destructive edit.
//
// It runs a bare type query with no predicate, so no schema, resolver, or
// daily-directory wiring is required beyond the database handle — the query is
// equivalent to the RQL `type:<name>` with no filter.
func objectsByType(db *index.Database, typeName string) ([]model.Object, error) {
	executor := query.NewExecutor(db.DB())
	result, err := executor.Run(&query.Query{
		Type:     query.QueryTypeObject,
		TypeName: typeName,
	}, query.RunRequest{})
	if err != nil {
		return nil, err
	}
	return result.Objects, nil
}

// traitsByType returns every indexed trait of the given trait type via the RQL
// executor. Schema maintenance uses it to detect whether a trait still has
// instances before removal. Like objectsByType, it runs a bare trait query
// (RQL `trait:<name>` with no value filter).
func traitsByType(db *index.Database, traitType string) ([]model.Trait, error) {
	executor := query.NewExecutor(db.DB())
	result, err := executor.Run(&query.Query{
		Type:     query.QueryTypeTrait,
		TypeName: traitType,
	}, query.RunRequest{})
	if err != nil {
		return nil, err
	}
	return result.Traits, nil
}
