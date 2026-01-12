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

	// Sort and group specifications (optional)
	Sort  *SortSpec
	Group *GroupSpec

	// Limit restricts the number of results (0 = no limit)
	Limit int
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

// CompareOp represents a comparison operator.
type CompareOp int

const (
	CompareEq  CompareOp = iota // = (default, equals)
	CompareLt                   // <
	CompareGt                   // >
	CompareLte                  // <=
	CompareGte                  // >=
)

func (op CompareOp) String() string {
	switch op {
	case CompareLt:
		return "<"
	case CompareGt:
		return ">"
	case CompareLte:
		return "<="
	case CompareGte:
		return ">="
	default:
		return "="
	}
}

// FieldPredicate filters by object field value.
// Syntax: .field:value, .field:*, .field:<value, !.field:value
type FieldPredicate struct {
	basePredicate
	Field     string
	Value     string    // "*" means "exists"
	IsExists  bool      // true if Value is "*"
	CompareOp CompareOp // comparison operator (default: equals)
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
// Syntax: parent:type, parent:{object:type ...}, parent:[[target]], parent:_
type ParentPredicate struct {
	basePredicate
	Target    string // Specific target ID (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // An object query (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if parent:_ (binds to current result in sort/group context)
}

func (ParentPredicate) predicateNode() {}

// AncestorPredicate filters by any ancestor matching.
// Syntax: ancestor:type, ancestor:{object:type ...}, ancestor:[[target]], ancestor:_
type AncestorPredicate struct {
	basePredicate
	Target    string // Specific target ID (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // An object query (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if ancestor:_ (binds to current result in sort/group context)
}

func (AncestorPredicate) predicateNode() {}

// ChildPredicate filters by having a direct child matching.
// Syntax: child:type, child:{object:type ...}, child:[[target]], child:_
type ChildPredicate struct {
	basePredicate
	Target    string // Specific target ID (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // An object query (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if child:_ (binds to current result in sort/group context)
}

func (ChildPredicate) predicateNode() {}

// DescendantPredicate filters by having any descendant matching (at any depth).
// Syntax: descendant:type, descendant:{object:type ...}, descendant:[[target]], descendant:_
type DescendantPredicate struct {
	basePredicate
	Target    string // Specific target ID (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // An object query (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if descendant:_ (binds to current result in sort/group context)
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
// Syntax: refs:[[target]], refs:{object:type ...}, refs:_
type RefsPredicate struct {
	basePredicate
	Target    string // Specific target like "projects/website" (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // Subquery to match targets (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if refs:_ (binds to current result in sort/group context)
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
// Syntax: value:val, value:<val, value:>val, !value:val
type ValuePredicate struct {
	basePredicate
	Value     string
	CompareOp CompareOp // comparison operator (default: equals)
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
// Syntax: on:type, on:{object:type ...}, on:[[target]], on:_
type OnPredicate struct {
	basePredicate
	Target    string // Specific target ID (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // An object query (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if on:_ (binds to current result in sort/group context)
}

func (OnPredicate) predicateNode() {}

// WithinPredicate filters traits by any ancestor object.
// Syntax: within:type, within:{object:type ...}, within:[[target]], within:_
type WithinPredicate struct {
	basePredicate
	Target    string // Specific target ID (mutually exclusive with SubQuery and IsSelfRef)
	SubQuery  *Query // An object query (mutually exclusive with Target and IsSelfRef)
	IsSelfRef bool   // True if within:_ (binds to current result in sort/group context)
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

// AtPredicate filters traits by co-location (same file:line).
// Syntax: at:{trait:name ...}, at:[[target]], at:_
// For traits only - matches traits at the same file and line.
type AtPredicate struct {
	basePredicate
	Target    string // Specific trait ID (if referencing a known trait)
	SubQuery  *Query // A trait query to match against
	IsSelfRef bool   // True if at:_ (binds to current result in sort/group context)
}

func (AtPredicate) predicateNode() {}

// RefdPredicate filters objects/traits by what references them (inverse of refs:).
// Syntax: refd:{object:type ...}, refd:{trait:name ...}, refd:[[target]], refd:_
type RefdPredicate struct {
	basePredicate
	Target    string // Specific source ID
	SubQuery  *Query // Query matching the sources that reference this
	IsSelfRef bool   // True if refd:_ (binds to current result in sort/group context)
}

func (RefdPredicate) predicateNode() {}

// AggregationType represents how to aggregate multiple values for sort/group.
type AggregationType int

const (
	AggFirst AggregationType = iota // Default: first by position
	AggMin                          // Minimum value
	AggMax                          // Maximum value
	AggCount                        // Count of matches
)

// SortSpec represents a sort specification.
type SortSpec struct {
	Aggregation AggregationType
	Descending  bool

	// One of these will be set:
	Path     *PathExpr // Direct path: _.parent.status
	SubQuery *Query    // Subquery: {trait:due at:_}
}

// GroupSpec represents a group specification.
type GroupSpec struct {
	Aggregation AggregationType // Usually not used, but count: could be useful

	// One of these will be set:
	Path     *PathExpr // Direct path: _.refs:project
	SubQuery *Query    // Subquery: {object:project refd:_}
}

// PathExpr represents a path expression from the result reference (_).
type PathExpr struct {
	Steps []PathStep
}

// PathStep represents one step in a path expression.
type PathStep struct {
	Kind PathStepKind
	Name string // Field name, type name, etc.
}

// PathStepKind represents the kind of path step.
type PathStepKind int

const (
	PathStepField    PathStepKind = iota // .fieldname
	PathStepParent                       // .parent
	PathStepAncestor                     // .ancestor:type
	PathStepRefs                         // .refs:type
	PathStepValue                        // .value
)
