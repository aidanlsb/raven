// Package query implements the Raven query language parser and executor.
package query

// QueryType represents the parsed query root.
type QueryType int

const (
	QueryTypeObject QueryType = iota
	QueryTypeTrait
	QueryTypeAsset
	QueryTypeSection
)

// Query represents a parsed query.
type Query struct {
	Type      QueryType
	TypeName  string    // Type name or trait name; empty for asset queries
	Predicate Predicate // Filter to apply (may be nil)
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
	CompareEq  CompareOp = iota // == (equals)
	CompareNeq                  // != (not equals)
	CompareLt                   // <
	CompareGt                   // >
	CompareLte                  // <=
	CompareGte                  // >=
)

func (op CompareOp) String() string {
	switch op {
	case CompareNeq:
		return "!="
	case CompareLt:
		return "<"
	case CompareGt:
		return ">"
	case CompareLte:
		return "<="
	case CompareGte:
		return ">="
	default:
		return "=="
	}
}

// FieldPredicate filters by type field value.
// Syntax: .field==value, .field>value, exists(.field)
// For string matching, use StringFuncPredicate (includes, startswith, endswith, matches).
type FieldPredicate struct {
	basePredicate
	Field      string
	Value      string    // "*" means "exists"
	IsExists   bool      // true if Value is "*"
	CompareOp  CompareOp // comparison operator (==, !=, <, >, <=, >=)
	IsRefValue bool      // true if the value came from a [[ref]] token
}

func (FieldPredicate) predicateNode() {}

// HasPredicate filters scopes by whether they directly contain matching sections or traits.
// Syntax: has(section ...), has(trait:name ...)
type HasPredicate struct {
	basePredicate
	SubQuery *Query // A trait query
}

func (HasPredicate) predicateNode() {}

// InPredicate filters scoped results by their direct containing scope.
// Syntax: in(type:<name> ...), in(section ...), in([[target]])
type InPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // A scope query (mutually exclusive with Target)
}

func (InPredicate) predicateNode() {}

// ContainsPredicate filters scopes by whether they recursively contain matching sections or traits.
// Syntax: contains(section ...), contains(trait:name ...)
type ContainsPredicate struct {
	basePredicate
	SubQuery *Query
}

func (ContainsPredicate) predicateNode() {}

// RefsPredicate filters type-query results by what they reference.
// Syntax: refs([[target]]), refs(target), refs(type:<name> ...)
type RefsPredicate struct {
	basePredicate
	Target   string // Specific target like "projects/website" (mutually exclusive with SubQuery)
	SubQuery *Query // Subquery to match targets (mutually exclusive with Target)
}

func (RefsPredicate) predicateNode() {}

// ContentPredicate filters type-query results by full-text search on their content.
// Syntax: content("search terms"), content("exact phrase")
type ContentPredicate struct {
	basePredicate
	SearchTerm string // The search term or phrase
}

func (ContentPredicate) predicateNode() {}

// ValuePredicate filters traits by value.
//
// Deprecated: Use FieldPredicate with Field="value" instead.
// The parser now creates FieldPredicate for .value== syntax.
// This type is retained for internal SQL generation helpers.
type ValuePredicate struct {
	basePredicate
	Value     string
	CompareOp CompareOp // comparison operator (==, !=, <, >, <=, >=)
}

func (ValuePredicate) predicateNode() {}

// WithinPredicate filters scoped results by any containing scope.
// Syntax: within(type:<name> ...), within(section ...), within([[target]])
type WithinPredicate struct {
	basePredicate
	Target   string // Specific target ID (mutually exclusive with SubQuery)
	SubQuery *Query // A scope query (mutually exclusive with Target)
}

func (WithinPredicate) predicateNode() {}

// OrPredicate represents an OR combination of two or more predicates.
// Syntax: (pred1 | pred2 | pred3)
type OrPredicate struct {
	basePredicate
	Predicates []Predicate
}

func (OrPredicate) predicateNode() {}

// NotPredicate represents the negation of a predicate.
// Syntax: !(pred), !pred
type NotPredicate struct {
	Inner Predicate
}

func (NotPredicate) predicateNode() {}
func (NotPredicate) Negated() bool  { return true }

// StringFuncType represents the type of string function.
type StringFuncType int

const (
	StringFuncIncludes   StringFuncType = iota // includes(.field, "str") - substring match
	StringFuncStartsWith                       // startswith(.field, "str")
	StringFuncEndsWith                         // endswith(.field, "str")
	StringFuncMatches                          // matches(.field, "pattern") - regex match
)

func (sf StringFuncType) String() string {
	switch sf {
	case StringFuncIncludes:
		return "includes"
	case StringFuncStartsWith:
		return "startswith"
	case StringFuncEndsWith:
		return "endswith"
	case StringFuncMatches:
		return "matches"
	default:
		return "unknown"
	}
}

// StringFuncPredicate represents a string function predicate.
// Syntax: includes(.field, "value"), startswith(.field, "value"), etc.
// Can also be used with _ as the field for array element context.
type StringFuncPredicate struct {
	basePredicate
	FuncType      StringFuncType
	Field         string // Field name (without .) or "_" for array element
	Value         string // The string to match against
	CaseSensitive bool   // If true, match is case-sensitive (default: false)
	IsElementRef  bool   // True if Field is "_" (array element reference)
}

func (StringFuncPredicate) predicateNode() {}

// ArrayQuantifierType represents the type of array quantifier.
type ArrayQuantifierType int

const (
	ArrayQuantifierAny  ArrayQuantifierType = iota // any(.field, predicate) - any element matches
	ArrayQuantifierAll                             // all(.field, predicate) - all elements match
	ArrayQuantifierNone                            // none(.field, predicate) - no element matches
)

func (aq ArrayQuantifierType) String() string {
	switch aq {
	case ArrayQuantifierAny:
		return "any"
	case ArrayQuantifierAll:
		return "all"
	case ArrayQuantifierNone:
		return "none"
	default:
		return "unknown"
	}
}

// ArrayQuantifierPredicate represents an array quantifier predicate.
// Syntax: any(.tags, _ == "urgent"), all(.tags, startswith(_, "feature-"))
type ArrayQuantifierPredicate struct {
	basePredicate
	Quantifier  ArrayQuantifierType
	Field       string    // The array field to iterate
	ElementPred Predicate // Predicate to test against each element (uses _ as element reference)
}

func (ArrayQuantifierPredicate) predicateNode() {}

// ElementEqualityPredicate represents _ == value or _ != value within array context.
// Syntax: _ == "urgent", _ != "deprecated"
type ElementEqualityPredicate struct {
	basePredicate
	Value      string
	CompareOp  CompareOp // == or !=
	IsRefValue bool      // true if the value came from a [[ref]] token
}

func (ElementEqualityPredicate) predicateNode() {}

// GroupPredicate represents a parenthesized group of predicates.
// Used for explicit precedence control.
type GroupPredicate struct {
	basePredicate
	Predicates []Predicate
}

func (GroupPredicate) predicateNode() {}

// AtPredicate filters traits by co-location (same file:line).
// Syntax: at(trait:name ...), at([[target]]), at(_)
// For traits only - matches traits at the same file and line.
type AtPredicate struct {
	basePredicate
	Target   string // Specific trait ID (if referencing a known trait)
	SubQuery *Query // A trait query to match against
}

func (AtPredicate) predicateNode() {}

// RefdPredicate filters objects/traits by what references them (inverse of refs()).
// Syntax: refd(type:type ...), refd(trait:name ...), refd([[target]]), refd(target)
type RefdPredicate struct {
	basePredicate
	Target   string // Specific source ID
	SubQuery *Query // Query matching the sources that reference this
}

func (RefdPredicate) predicateNode() {}
