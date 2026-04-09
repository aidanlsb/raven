package query

import (
	"database/sql"
	"time"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// Executor executes queries against the database.
type Executor struct {
	db                         *sql.DB
	resolver                   *resolver.Resolver // Cached resolver for target resolution
	dailyDirectory             string             // Used for date shorthand refs (e.g. [[2026-01-01]])
	schema                     *schema.Schema
	now                        time.Time
	nowFn                      func() time.Time
	fieldRefAmbiguityCache     map[fieldRefAmbiguityKey]fieldRefAmbiguityResult
	ambiguousFieldRefQueryHook func()
}

// NewExecutor creates a new query executor.
func NewExecutor(db *sql.DB) *Executor {
	return &Executor{db: db, dailyDirectory: "daily", nowFn: time.Now}
}

// SetSchema injects a schema for type-aware query semantics.
func (e *Executor) SetSchema(sch *schema.Schema) {
	e.schema = sch
}

func (e *Executor) currentTime() time.Time {
	if e.nowFn != nil {
		return e.nowFn()
	}
	return time.Now()
}

func (e *Executor) queryNow() time.Time {
	if !e.now.IsZero() {
		return e.now
	}
	return e.currentTime()
}

func (e *Executor) withExecutionNow() *Executor {
	scoped := *e
	scoped.now = e.currentTime()
	scoped.fieldRefAmbiguityCache = make(map[fieldRefAmbiguityKey]fieldRefAmbiguityResult)
	return &scoped
}
