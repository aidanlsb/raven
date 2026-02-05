package query

import (
	"database/sql"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// Executor executes queries against the database.
type Executor struct {
	db             *sql.DB
	resolver       *resolver.Resolver // Cached resolver for target resolution
	dailyDirectory string             // Used for date shorthand refs (e.g. [[2026-01-01]])
	schema         *schema.Schema
}

// NewExecutor creates a new query executor.
func NewExecutor(db *sql.DB) *Executor {
	return &Executor{db: db, dailyDirectory: "daily"}
}

// SetSchema injects a schema for type-aware query semantics.
func (e *Executor) SetSchema(sch *schema.Schema) {
	e.schema = sch
}
