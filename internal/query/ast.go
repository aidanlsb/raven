// Package query implements the Raven query language parser and executor.
package query

// QueryType represents the type of query (object or trait).
type QueryType int

const (
	QueryTypeObject QueryType = iota
	QueryTypeTrait
)

// Query represents a parsed query.
type Query struct {
	Type       QueryType
	TypeName   string      // Object type or trait name
	Predicates []Predicate // Filters to apply
}

// Predicate represents a filter condition in a query.
type Predicate interface {
	predicateNode()
	Negated() bool
}

// basePredicate provides common functionality for predicates.
type basePredicate struct {
	negated bool
}

func (b basePredicate) Negated() bool { return b.negated }

// FieldPredicate filters by object field value.
// Syntax: .field:value, .field:*, !.field:value
type FieldPredicate struct {
	basePredicate
	Field    string
	Value    string // "*" means "exists"
	IsExists bool   // true if Value is "*"
}

func (FieldPredicate) predicateNode() {}

// HasPredicate filters objects by whether they contain matching traits.
// Syntax: has:trait, has:{trait:name value:...}
type HasPredicate struct {
	basePredicate
	SubQuery *Query // A trait query
}

func (HasPredicate) predicateNode() {}

// ParentPredicate filters by direct parent matching.
// Syntax: parent:type, parent:{object:type ...}, parent:[[target]]
type ParentPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // An object query (mutually exclusive with Target)
}

func (ParentPredicate) predicateNode() {}

// AncestorPredicate filters by any ancestor matching.
// Syntax: ancestor:type, ancestor:{object:type ...}, ancestor:[[target]]
type AncestorPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // An object query (mutually exclusive with Target)
}

func (AncestorPredicate) predicateNode() {}

// ChildPredicate filters by having a direct child matching.
// Syntax: child:type, child:{object:type ...}, child:[[target]]
type ChildPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // An object query (mutually exclusive with Target)
}

func (ChildPredicate) predicateNode() {}

// DescendantPredicate filters by having any descendant matching (at any depth).
// Syntax: descendant:type, descendant:{object:type ...}, descendant:[[target]]
type DescendantPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // An object query (mutually exclusive with Target)
}

func (DescendantPredicate) predicateNode() {}

// ContainsPredicate filters objects by whether they contain matching traits anywhere
// in their subtree (self or any descendant object).
// Syntax: contains:{trait:name ...}
type ContainsPredicate struct {
	basePredicate
	SubQuery *Query // A trait query
}

func (ContainsPredicate) predicateNode() {}

// RefsPredicate filters objects by what they reference.
// Syntax: refs:[[target]], refs:{object:type ...}
type RefsPredicate struct {
	basePredicate
	Target   string // Specific target like "projects/website" (mutually exclusive with SubQuery)
	SubQuery *Query // Subquery to match targets (mutually exclusive with Target)
}

func (RefsPredicate) predicateNode() {}

// ContentPredicate filters objects by full-text search on their content.
// Syntax: content:"search terms", content:"exact phrase"
type ContentPredicate struct {
	basePredicate
	SearchTerm string // The search term or phrase
}

func (ContentPredicate) predicateNode() {}

// ValuePredicate filters traits by value.
// Syntax: value:val, !value:val
type ValuePredicate struct {
	basePredicate
	Value string
}

func (ValuePredicate) predicateNode() {}

// SourcePredicate filters traits by source location.
// Syntax: source:inline, source:frontmatter
type SourcePredicate struct {
	basePredicate
	Source string // "inline" or "frontmatter"
}

func (SourcePredicate) predicateNode() {}

// OnPredicate filters traits by direct parent object.
// Syntax: on:type, on:{object:type ...}, on:[[target]]
type OnPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // An object query (mutually exclusive with Target)
}

func (OnPredicate) predicateNode() {}

// WithinPredicate filters traits by any ancestor object.
// Syntax: within:type, within:{object:type ...}, within:[[target]]
type WithinPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // An object query (mutually exclusive with Target)
}

func (WithinPredicate) predicateNode() {}

// OrPredicate represents an OR combination of predicates.
// Syntax: (pred1 | pred2)
type OrPredicate struct {
	basePredicate
	Left  Predicate
	Right Predicate
}

func (OrPredicate) predicateNode() {}

// GroupPredicate represents a parenthesized group of predicates.
// Used for explicit precedence control.
type GroupPredicate struct {
	basePredicate
	Predicates []Predicate
}

func (GroupPredicate) predicateNode() {}
