package query

// This file is the single source of truth for which predicate kinds are legal
// at which query root (object/trait/asset/section). Both the validator
// (validator.go) and the SQL executor (sql_predicate_dispatch.go) consult this
// table so the legality rules cannot drift apart.
//
// Only the coarse "is predicate kind K legal at root R" decision lives here.
// Finer, value-level rules (e.g. traits only allow .value, object fields must
// exist in the schema, section fields must be known columns) stay in the
// validator and the entity-specific SQL builders, since they depend on schema
// or on per-field semantics rather than on the predicate kind alone.

// predKind identifies a predicate node kind for capability lookups. It is the
// "PredicateKind" half of the (QueryType, PredicateKind) capability key.
type predKind int

const (
	// predKindComposite covers boolean composition (OR/NOT/GROUP) and any node
	// that is not subject to a per-root capability rule (e.g. array element
	// predicates, which are only reachable inside array quantifiers). These are
	// never rejected by the capability table itself.
	predKindComposite predKind = iota
	predKindField
	predKindStringFunc
	predKindArray
	predKindHas
	predKindContains
	predKindIn
	predKindWithin
	predKindRefs
	predKindRefd
	predKindContent
	predKindValue
	predKindAt
)

// predicateKindOf classifies a predicate node for capability lookups.
func predicateKindOf(pred Predicate) predKind {
	switch pred.(type) {
	case *FieldPredicate:
		return predKindField
	case *StringFuncPredicate:
		return predKindStringFunc
	case *ArrayQuantifierPredicate:
		return predKindArray
	case *HasPredicate:
		return predKindHas
	case *ContainsPredicate:
		return predKindContains
	case *InPredicate:
		return predKindIn
	case *WithinPredicate:
		return predKindWithin
	case *RefsPredicate:
		return predKindRefs
	case *RefdPredicate:
		return predKindRefd
	case *ContentPredicate:
		return predKindContent
	case *ValuePredicate:
		return predKindValue
	case *AtPredicate:
		return predKindAt
	default:
		return predKindComposite
	}
}

// predicateCapability describes why a predicate kind is not allowed at a root.
// The message/suggestion pair is surfaced to users verbatim (as a
// ValidationError), so these strings are part of the stable error vocabulary.
type predicateCapability struct {
	message    string
	suggestion string
}

// disallowedPredicates lists, per query root, the predicate kinds that are not
// legal there together with the user-facing explanation. A (root, kind) pair
// that is absent from this table is legal (subject to the finer value-level
// checks performed elsewhere).
var disallowedPredicates = map[QueryType]map[predKind]predicateCapability{
	QueryTypeObject: {
		predKindValue: {
			message:    "value predicate is only valid for trait queries",
			suggestion: "Use .value==X in trait queries, or use .field==X for type fields",
		},
		predKindIn: {
			message:    "in() predicate is only valid for trait and section queries",
			suggestion: "Use in(type:...) or in(section ...) on traits or sections",
		},
		predKindWithin: {
			message:    "within() predicate is only valid for trait and section queries",
			suggestion: "Use within(type:...) or within(section ...) on traits or sections",
		},
		predKindAt: {
			message:    "at() predicate is only valid for trait queries",
			suggestion: "Use at(trait:...) to find traits co-located with other traits",
		},
	},
	QueryTypeTrait: {
		predKindRefd: {
			message:    "refd() predicate is only valid for type queries",
			suggestion: "Use refd(...) with type queries, or use refs(...) in trait queries",
		},
		predKindHas: {
			message:    "has() predicate is only valid for type and section queries",
			suggestion: "Use has(trait:...) or has(section ...) in type and section queries",
		},
		predKindContains: {
			message:    "contains() predicate is only valid for type and section queries",
			suggestion: "Use contains(trait:...) or contains(section ...) in type and section queries",
		},
	},
	QueryTypeAsset: {
		predKindRefs: {
			message:    "refs() predicate is not valid for asset queries",
			suggestion: "Assets do not have outbound references; use asset refd(...) to find assets referenced by objects or traits",
		},
		predKindArray: {
			message:    "array predicates are not valid for asset queries",
			suggestion: "Asset fields are scalar metadata fields",
		},
		predKindContent: {
			message:    "content() predicate is not valid for asset queries",
			suggestion: "Filter assets by derived metadata fields such as .filename, .extension, .media_type, or .size_bytes",
		},
		predKindHas: {
			message:    "has() predicate is not valid for asset queries",
			suggestion: "Assets do not have traits; use asset refd(...) to filter by referencing objects or traits",
		},
		predKindContains: {
			message:    "contains() predicate is not valid for asset queries",
			suggestion: "Assets do not contain Raven sections or traits",
		},
		predKindIn: {
			message:    "scope predicates are not valid for asset queries",
			suggestion: "Assets are path-backed resources, not markdown scopes",
		},
		predKindWithin: {
			message:    "scope predicates are not valid for asset queries",
			suggestion: "Assets are path-backed resources, not markdown scopes",
		},
		predKindAt: {
			message:    "trait-location predicates are not valid for asset queries",
			suggestion: "Use asset refd(trait:...) to find assets referenced by matching trait lines",
		},
		predKindValue: {
			message:    "value predicates are not valid for asset queries",
			suggestion: "Use asset fields such as .filename, .extension, .media_type, or .size_bytes",
		},
	},
	QueryTypeSection: {
		predKindArray: {
			message:    "array predicates are not valid for section queries",
			suggestion: "Sections only expose scalar built-in fields",
		},
		predKindValue: {
			message:    "value predicates are not valid for section queries",
			suggestion: "Use section fields such as .title, .slug, or .level",
		},
		predKindAt: {
			message:    "at() predicate is only valid for trait queries",
			suggestion: "Use at(trait:...) to find traits co-located with other traits",
		},
	},
}

// predicateAllowedAtRoot reports whether the given predicate kind is legal at
// the query root. It returns a *ValidationError describing the illegal
// combination when it is not, and nil when it is (or when the predicate is a
// composite/element node with no per-root restriction).
//
// This is the shared legality gate: the validator calls it to reject illegal
// queries with stable messages, and the executor calls it defensively so its
// SQL dispatch cannot silently diverge from the validator.
func predicateAllowedAtRoot(root QueryType, pred Predicate) *ValidationError {
	kind := predicateKindOf(pred)
	if kind == predKindComposite {
		return nil
	}
	if rules, ok := disallowedPredicates[root]; ok {
		if entry, ok := rules[kind]; ok {
			return &ValidationError{Message: entry.message, Suggestion: entry.suggestion}
		}
	}
	return nil
}
