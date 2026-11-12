package query

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

type subjectFrame struct {
	alias    string
	root     QueryType
	typeName string
}

type Executor struct {
	db                         *sql.DB
	resolver                   *resolver.Resolver // Cached resolver for target resolution
	dailyDirectory             string             // Used for date shorthand refs (e.g. [[2026-01-01]])
	schema                     *schema.Schema
	now                        time.Time
	nowFn                      func() time.Time
	fieldRefAmbiguityCache     map[fieldRefAmbiguityKey]fieldRefAmbiguityResult
	ambiguousFieldRefQueryHook func()
	aliasSeq                   int
	queryStack                 []subjectFrame
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
	scoped.aliasSeq = 0
	scoped.queryStack = nil
	return &scoped
}

func (e *Executor) nextAlias(prefix string) string {
	e.aliasSeq++
	return fmt.Sprintf("%s%d", prefix, e.aliasSeq)
}

func (e *Executor) pushQuerySubject(frame subjectFrame) func() {
	e.queryStack = append(e.queryStack, frame)
	return func() {
		if len(e.queryStack) == 0 {
			return
		}
		e.queryStack = e.queryStack[:len(e.queryStack)-1]
	}
}

func (e *Executor) currentSubject() (subjectFrame, bool) {
	if len(e.queryStack) == 0 {
		return subjectFrame{}, false
	}
	return e.queryStack[len(e.queryStack)-1], true
}

func (e *Executor) thisSubject() (subjectFrame, error) {
	if len(e.queryStack) < 2 {
		return subjectFrame{}, fmt.Errorf("this is only valid in a nested query")
	}
	return e.queryStack[len(e.queryStack)-2], nil
}
