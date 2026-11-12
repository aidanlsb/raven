package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// validateCtx is the subject of the query currently being validated, plus the
// immediately enclosing query (for `this`) and optional array-element context.
type validateCtx struct {
	root       QueryType
	typeName   string
	typeDef    *schema.TypeDefinition
	outer      *validateCtx
	inArray    bool
	elemType   schema.FieldType
	elemTarget string
}

func (c *validateCtx) clone() *validateCtx {
	if c == nil {
		return nil
	}
	copied := *c
	return &copied
}

func (v *Validator) contextForQuery(q *Query, outer *validateCtx) (*validateCtx, error) {
	ctx := &validateCtx{root: q.Type, typeName: q.TypeName, outer: outer}
	if q.Type == QueryTypeObject && strings.EqualFold(q.TypeName, "section") {
		return nil, &ValidationError{
			Message:    "type:section is not a valid query root",
			Suggestion: "Use the bare section root, for example: section .title==Tasks",
		}
	}
	if !v.hasSchema() {
		return ctx, nil
	}
	switch q.Type {
	case QueryTypeObject:
		td, exists := v.schema.Types[q.TypeName]
		if !exists {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("unknown type '%s'", q.TypeName),
				Suggestion: fmt.Sprintf("Available types: %s", strings.Join(v.availableTypes(), ", ")),
			}
		}
		ctx.typeDef = td
	case QueryTypeTrait:
		if _, exists := v.schema.Traits[q.TypeName]; !exists {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("unknown trait '%s'", q.TypeName),
				Suggestion: fmt.Sprintf("Available traits: %s", strings.Join(v.availableTraits(), ", ")),
			}
		}
	}
	return ctx, nil
}

func (v *Validator) validateQueryCtx(q *Query, outer *validateCtx) error {
	if q == nil {
		return nil
	}
	ctx, err := v.contextForQuery(q, outer)
	if err != nil {
		return err
	}
	if err := v.validatePredicateCtx(ctx, q.Predicate); err != nil {
		return err
	}
	return v.validateOrderBy(q, ctx.typeDef)
}

func (v *Validator) validatePredicateCtx(ctx *validateCtx, pred Predicate) error {
	if pred == nil {
		return nil
	}

	switch p := pred.(type) {
	case *OrPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validatePredicateCtx(ctx, subPred); err != nil {
				return err
			}
		}
		return nil
	case *NotPredicate:
		return v.validatePredicateCtx(ctx, p.Inner)
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validatePredicateCtx(ctx, subPred); err != nil {
				return err
			}
		}
		return nil
	}

	if verr := predicateAllowedAtRoot(ctx.root, pred); verr != nil {
		return verr
	}

	switch ctx.root {
	case QueryTypeObject:
		return v.validateLegalObjectPredicateCtx(ctx, pred)
	case QueryTypeTrait:
		return v.validateLegalTraitPredicateCtx(ctx, pred)
	case QueryTypeSection:
		return v.validateLegalSectionPredicateCtx(ctx, pred)
	case QueryTypeLink:
		return v.validateLegalLinkPredicateCtx(ctx, pred)
	default:
		return nil
	}
}

func (v *Validator) validateLegalObjectPredicateCtx(ctx *validateCtx, pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		return v.validateFieldPredicateCtx(ctx, p)
	case *StringFuncPredicate:
		return v.validateObjectStringFuncPredicateCtx(ctx, p)
	case *ArrayQuantifierPredicate:
		return v.validateArrayQuantifierPredicateCtx(ctx, p)
	case *HasPredicate:
		return v.validateQueryCtx(p.SubQuery, ctx)
	case *ContainsPredicate:
		return v.validateQueryCtx(p.SubQuery, ctx)
	case *RefsPredicate:
		return v.validateRefsSubquery(p.SubQuery, ctx)
	case *LinksPredicate:
		return validateLinkPredicate(p.LinkPredicate)
	case *RefdPredicate:
		return v.validateRefdSubquery(p.SubQuery, ctx)
	case *ContentPredicate:
		return validateContentTerm(p)
	}
	return nil
}

func (v *Validator) validateLegalTraitPredicateCtx(ctx *validateCtx, pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		return v.validateTraitFieldPredicateCtx(ctx, p)
	case *StringFuncPredicate:
		if err := v.validatePathUsesThis(ctx, p.path()); err != nil {
			return err
		}
		if p.IsElementRef || p.Field != "value" || p.path().hasHops() || p.path().This {
			fieldLabel := "." + p.Field
			if p.IsElementRef {
				fieldLabel = "_"
			} else if p.path().This || p.path().hasHops() {
				fieldLabel = p.path().String()
			}
			return &ValidationError{
				Message:    fmt.Sprintf("string functions on trait queries only support .value, got %s", fieldLabel),
				Suggestion: `Use includes(.value, "..."), startswith(.value, "..."), endswith(.value, "..."), or matches(.value, "..."). Use content("...") to search trait line content.`,
			}
		}
		return validateRegexPattern(p)
	case *ArrayQuantifierPredicate:
		return v.validateTraitArrayQuantifierPredicateCtx(ctx, p)
	case *InPredicate:
		return v.validateScopeSubquery(QueryTypeTrait, "in", p.SubQuery, ctx)
	case *WithinPredicate:
		return v.validateScopeSubquery(QueryTypeTrait, "within", p.SubQuery, ctx)
	case *RefsPredicate:
		return v.validateRefsSubquery(p.SubQuery, ctx)
	case *LinksPredicate:
		return validateLinkPredicate(p.LinkPredicate)
	case *ContentPredicate:
		return validateContentTerm(p)
	case *AtPredicate:
		if p.SubQuery != nil {
			if p.SubQuery.Type != QueryTypeTrait {
				return &ValidationError{
					Message:    "at: requires a trait subquery",
					Suggestion: "Use at(trait:name) to find traits co-located with other traits",
				}
			}
			return v.validateQueryCtx(p.SubQuery, ctx)
		}
	}
	return nil
}

func (v *Validator) validateLegalSectionPredicateCtx(ctx *validateCtx, pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		if err := v.validateSimpleRootFieldPath(ctx, p.path(), "section"); err != nil {
			return err
		}
		if err := v.validateNonObjectPathRHS(ctx, p.rhs(), "section"); err != nil {
			return err
		}
		if _, ok := sectionFieldColumn("s", p.Field); !ok {
			return &ValidationError{
				Message:    fmt.Sprintf("section has no field '%s'", p.Field),
				Suggestion: fmt.Sprintf("Available section fields: %s", strings.Join(availableSectionFields(), ", ")),
			}
		}
		if isNumericSectionField(p.Field) && p.rhs().Path == nil {
			return validateNumericFieldPredicate("section", p)
		}
	case *StringFuncPredicate:
		if err := v.validatePathUsesThis(ctx, p.path()); err != nil {
			return err
		}
		if p.IsElementRef {
			return &ValidationError{
				Message:    "string function placeholder '_' is not valid for section queries",
				Suggestion: `Use includes(.title, "..."), startswith(.slug, "..."), or matches(.file_path, "...")`,
			}
		}
		if p.path().hasHops() || p.path().This {
			return &ValidationError{
				Message:    fmt.Sprintf("section queries do not support field paths (%s)", p.path().String()),
				Suggestion: "Use a built-in section field such as .title or .slug",
			}
		}
		if _, ok := sectionFieldColumn("s", p.Field); !ok {
			return &ValidationError{
				Message:    fmt.Sprintf("section has no field '%s'", p.Field),
				Suggestion: fmt.Sprintf("Available section fields: %s", strings.Join(availableSectionFields(), ", ")),
			}
		}
		if isNumericSectionField(p.Field) {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates are not valid for numeric section field '.%s'", p.Field),
				Suggestion: "Use comparison predicates for numeric section fields",
			}
		}
		return validateRegexPattern(p)
	case *InPredicate:
		return v.validateScopeSubquery(QueryTypeSection, "in", p.SubQuery, ctx)
	case *WithinPredicate:
		return v.validateScopeSubquery(QueryTypeSection, "within", p.SubQuery, ctx)
	case *HasPredicate:
		return v.validateQueryCtx(p.SubQuery, ctx)
	case *ContainsPredicate:
		return v.validateQueryCtx(p.SubQuery, ctx)
	case *RefsPredicate:
		return v.validateRefsSubquery(p.SubQuery, ctx)
	case *LinksPredicate:
		return validateLinkPredicate(p.LinkPredicate)
	case *RefdPredicate:
		return v.validateRefdSubquery(p.SubQuery, ctx)
	case *ContentPredicate:
		return validateContentTerm(p)
	}
	return nil
}

func (v *Validator) validateLegalLinkPredicateCtx(ctx *validateCtx, pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		if err := v.validateSimpleRootFieldPath(ctx, p.path(), "link"); err != nil {
			return err
		}
		if err := v.validateNonObjectPathRHS(ctx, p.rhs(), "link"); err != nil {
			return err
		}
		return validateLinkPredicate(p)
	case *StringFuncPredicate:
		if err := v.validatePathUsesThis(ctx, p.path()); err != nil {
			return err
		}
		if p.path().hasHops() || p.path().This {
			return &ValidationError{
				Message:    fmt.Sprintf("link queries do not support field paths (%s)", p.path().String()),
				Suggestion: "Use a built-in link field such as .ext or .raw_target",
			}
		}
		return validateLinkPredicate(p)
	case *WithinPredicate:
		return v.validateScopeSubquery(QueryTypeLink, "within", p.SubQuery, ctx)
	}
	return nil
}

func (v *Validator) validateNonObjectPathRHS(ctx *validateCtx, rhs CompareRHS, rootName string) error {
	if rhs.Path == nil {
		return nil
	}
	if rhs.Path.This {
		return v.validatePathUsesThis(ctx, *rhs.Path)
	}
	return &ValidationError{
		Message:    fmt.Sprintf("unquoted %s on the right-hand side is a path, not a literal", rhs.Path.String()),
		Suggestion: fmt.Sprintf(`Quote dotted literals, for example .ext==".pdf". Field-to-field comparison is supported on type queries, not %s queries`, rootName),
	}
}

func (v *Validator) validateSimpleRootFieldPath(ctx *validateCtx, path FieldPath, rootName string) error {
	if err := v.validatePathUsesThis(ctx, path); err != nil {
		return err
	}
	if path.This || path.hasHops() {
		return &ValidationError{
			Message:    fmt.Sprintf("%s queries do not support field paths (%s)", rootName, path.String()),
			Suggestion: fmt.Sprintf("Use a built-in %s field", rootName),
		}
	}
	return nil
}

func (v *Validator) validateTraitFieldPredicateCtx(ctx *validateCtx, p *FieldPredicate) error {
	path := p.path()
	if err := v.validatePathUsesThis(ctx, path); err != nil {
		return err
	}
	if path.This {
		_, err := v.resolvePath(ctx, path)
		if err != nil {
			return err
		}
		return v.validateCompareRHSCtx(ctx, p.rhs(), p.CompareOp, p.IsExists)
	}
	if path.hasHops() || path.first() != "value" {
		return &ValidationError{
			Message:    "field predicates other than .value are only valid for type queries",
			Suggestion: "Use .value==X for trait values, or .field==X in type queries",
		}
	}
	return v.validateCompareRHSCtx(ctx, p.rhs(), p.CompareOp, p.IsExists)
}

func (v *Validator) validateFieldPredicateCtx(ctx *validateCtx, p *FieldPredicate) error {
	if _, err := v.resolvePath(ctx, p.path()); err != nil {
		return err
	}
	return v.validateCompareRHSCtx(ctx, p.rhs(), p.CompareOp, p.IsExists)
}

func (v *Validator) validateObjectStringFuncPredicateCtx(ctx *validateCtx, p *StringFuncPredicate) error {
	if p.IsElementRef {
		return &ValidationError{
			Message:    "string function placeholder '_' is only valid inside any()/all()/none()",
			Suggestion: `Use includes(.field, "..."), startswith(.field, "..."), endswith(.field, "..."), or matches(.field, "...") for type fields.`,
		}
	}
	resolved, err := v.resolvePath(ctx, p.path())
	if err != nil {
		return err
	}
	if resolved != nil && resolved.leafDef != nil {
		if resolved.leafDef.Type.IsArray() {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates require a scalar field, but '%s' is %s", p.path().String(), resolved.leafDef.Type),
				Suggestion: fmt.Sprintf(`Use any(%s, includes(_, "...")) for array fields`, p.path().String()),
			}
		}
		if !resolved.leafDef.Type.IsStringLike() {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates are not valid for field '%s' of type %s", p.path().String(), resolved.leafDef.Type),
				Suggestion: "Use comparison predicates (.field==value, .field!=value, .field<value, etc.) for non-string fields",
			}
		}
	}
	return validateRegexPattern(p)
}

func (v *Validator) validateArrayQuantifierPredicateCtx(ctx *validateCtx, p *ArrayQuantifierPredicate) error {
	resolved, err := v.resolvePath(ctx, p.path())
	if err != nil {
		return err
	}
	if resolved == nil || resolved.leafDef == nil {
		if !v.hasSchema() {
			return v.validateArrayElementPredicateCtx(arrayElementCtx(ctx, schema.FieldTypeString, ""), p.ElementPred)
		}
		return &ValidationError{Message: fmt.Sprintf("cannot apply any()/all()/none() to %s", p.path().String())}
	}
	elemType, ok := resolved.leafDef.Type.ElementType()
	if !ok {
		return &ValidationError{
			Message:    fmt.Sprintf("array predicates any()/all()/none() require an array field, but '%s' is %s", p.path().String(), resolved.leafDef.Type),
			Suggestion: "Use any()/all()/none() only with [] fields, or use scalar field predicates on non-array fields",
		}
	}
	elemTarget := ""
	if elemType.IsRef() {
		elemTarget = resolved.leafDef.Target
	}
	return v.validateArrayElementPredicateCtx(arrayElementCtx(ctx, elemType, elemTarget), p.ElementPred)
}

func (v *Validator) validateTraitArrayQuantifierPredicateCtx(ctx *validateCtx, p *ArrayQuantifierPredicate) error {
	if p.path().hasHops() || p.path().This || p.Field != "value" {
		return &ValidationError{
			Message:    fmt.Sprintf("array predicates on trait queries only support .value, got %s", p.path().String()),
			Suggestion: "Use any(.value, ...), all(.value, ...), or none(.value, ...) for array-valued traits",
		}
	}
	if !v.hasSchema() {
		return v.validateArrayElementPredicateCtx(arrayElementCtx(ctx, schema.FieldTypeString, ""), p.ElementPred)
	}
	traitDef := v.schema.Traits[ctx.typeName]
	if traitDef == nil {
		return &ValidationError{
			Message:    fmt.Sprintf("unknown trait '%s'", ctx.typeName),
			Suggestion: fmt.Sprintf("Available traits: %s", strings.Join(v.availableTraits(), ", ")),
		}
	}
	elemType, ok := traitDef.Type.ElementType()
	if !ok {
		return &ValidationError{
			Message:    fmt.Sprintf("array predicates any()/all()/none() require an array-valued trait, but trait '%s' is %s", ctx.typeName, traitDef.Type),
			Suggestion: "Use any()/all()/none() only with [] trait types, or use .value predicates on scalar traits",
		}
	}
	return v.validateArrayElementPredicateCtx(arrayElementCtx(ctx, elemType, ""), p.ElementPred)
}

func arrayElementCtx(parent *validateCtx, elemType schema.FieldType, elemTarget string) *validateCtx {
	child := parent.clone()
	child.inArray = true
	child.elemType = elemType
	child.elemTarget = elemTarget
	return child
}

func (v *Validator) validateArrayElementPredicateCtx(ctx *validateCtx, pred Predicate) error {
	if pred == nil {
		return nil
	}
	switch p := pred.(type) {
	case *ElementEqualityPredicate:
		if err := v.validateCompareRHSCtx(ctx, p.rhs(), p.CompareOp, false); err != nil {
			return err
		}
		if p.IsRefValue && !ctx.elemType.IsRef() {
			return &ValidationError{
				Message:    fmt.Sprintf("reference element comparison is only valid for ref[] fields, not %s[]", ctx.elemType),
				Suggestion: "Use [[target]] comparisons only for ref[] fields",
			}
		}
		return nil
	case *FieldPredicate:
		if !ctx.elemType.IsRef() {
			return &ValidationError{
				Message:    fmt.Sprintf("field paths on array elements require ref[] elements, not %s[]", ctx.elemType),
				Suggestion: `Use _==value for scalar array elements, or any(.field, .hop==value) on ref[] fields`,
			}
		}
		return v.validateFieldPredicateCtx(ctx, p)
	case *StringFuncPredicate:
		if p.IsElementRef {
			if !ctx.elemType.IsStringLike() {
				return &ValidationError{
					Message:    fmt.Sprintf("string functions are not valid for array elements of type %s", ctx.elemType),
					Suggestion: "Use element comparisons (_==value, _!=value, _<value, etc.) for non-string array element types",
				}
			}
			return validateRegexPattern(p)
		}
		if !ctx.elemType.IsRef() {
			return &ValidationError{
				Message:    "array element string functions must use '_' as the first argument",
				Suggestion: `Use includes(_, "..."), startswith(_, "..."), endswith(_, "..."), or matches(_, "...")`,
			}
		}
		return v.validateObjectStringFuncPredicateCtx(ctx, p)
	case *OrPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateArrayElementPredicateCtx(ctx, subPred); err != nil {
				return err
			}
		}
		return nil
	case *NotPredicate:
		return v.validateArrayElementPredicateCtx(ctx, p.Inner)
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateArrayElementPredicateCtx(ctx, subPred); err != nil {
				return err
			}
		}
		return nil
	default:
		return &ValidationError{
			Message:    fmt.Sprintf("unsupported array element predicate type %T", pred),
			Suggestion: `Use _==value, _==this, .field==value, or string functions like includes(_, "...")`,
		}
	}
}

func (v *Validator) validateCompareRHSCtx(ctx *validateCtx, rhs CompareRHS, op CompareOp, exists bool) error {
	if exists || rhs.Path == nil {
		return nil
	}
	_, err := v.resolvePath(ctx, *rhs.Path)
	_ = op
	return err
}

func (v *Validator) validatePathUsesThis(ctx *validateCtx, path FieldPath) error {
	if !path.This {
		return nil
	}
	if ctx == nil || ctx.outer == nil {
		return topLevelThisError(path)
	}
	return nil
}

func topLevelThisError(path FieldPath) error {
	return &ValidationError{
		Message:    fmt.Sprintf("%s is only valid in a nested query", path.String()),
		Suggestion: "Use this inside refs(...), refd(...), or another nested query to refer to the immediately enclosing subject, for example type:person refd(type:task .owner==this)",
	}
}

type resolvedPath struct {
	leafTypeName string
	leafDef      *schema.FieldDefinition
	leafField    string
	identity     bool
}

func (v *Validator) resolvePath(ctx *validateCtx, path FieldPath) (*resolvedPath, error) {
	if err := v.validatePathUsesThis(ctx, path); err != nil {
		return nil, err
	}

	start := ctx
	if path.This {
		start = ctx.outer
	}

	if path.IsIdentity() {
		return &resolvedPath{identity: true, leafTypeName: start.typeName}, nil
	}

	if ctx.inArray && !path.This {
		return v.resolvePathOnType(ctx.elemTarget, path.Fields, path)
	}

	switch start.root {
	case QueryTypeObject:
		if isDateVirtualField(start.typeName, path.Fields[0]) && !path.hasHops() {
			return &resolvedPath{leafTypeName: start.typeName, leafField: path.Fields[0]}, nil
		}
		return v.resolvePathOnTypeDef(start.typeName, start.typeDef, path.Fields, path)
	case QueryTypeTrait:
		if len(path.Fields) == 1 && path.Fields[0] == "value" {
			return &resolvedPath{leafTypeName: start.typeName, leafField: "value"}, nil
		}
		return nil, &ValidationError{
			Message:    fmt.Sprintf("trait queries do not support field path %s", path.String()),
			Suggestion: "Use .value for the trait value, or this.field to read the enclosing subject",
		}
	case QueryTypeSection, QueryTypeLink:
		if path.hasHops() {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("%s queries do not support field paths (%s)", queryTypeName(start.root), path.String()),
				Suggestion: "Use a built-in field of that root",
			}
		}
		return &resolvedPath{leafTypeName: start.typeName, leafField: path.first()}, nil
	default:
		return v.resolvePathOnTypeDef(start.typeName, start.typeDef, path.Fields, path)
	}
}

func (v *Validator) resolvePathOnType(typeName string, fields []string, display FieldPath) (*resolvedPath, error) {
	if !v.hasSchema() {
		return &resolvedPath{leafTypeName: typeName, leafField: fields[len(fields)-1]}, nil
	}
	if typeName == "" {
		return nil, &ValidationError{
			Message:    fmt.Sprintf("cannot follow %s because the ref has no target type", display.String()),
			Suggestion: "Declare target: on the ref field in schema.yaml",
		}
	}
	typeDef, ok := v.schema.Types[typeName]
	if !ok {
		return nil, &ValidationError{
			Message:    fmt.Sprintf("unknown type '%s' while following %s", typeName, display.String()),
			Suggestion: fmt.Sprintf("Available types: %s", strings.Join(v.availableTypes(), ", ")),
		}
	}
	return v.resolvePathOnTypeDef(typeName, typeDef, fields, display)
}

func (v *Validator) resolvePathOnTypeDef(typeName string, typeDef *schema.TypeDefinition, fields []string, display FieldPath) (*resolvedPath, error) {
	if !v.hasSchema() {
		return &resolvedPath{leafTypeName: typeName, leafField: fields[len(fields)-1]}, nil
	}
	currentType := typeName
	currentDef := typeDef
	for i, field := range fields {
		fieldDef, err := v.fieldDefinitionForType(currentType, currentDef, field)
		if err != nil {
			if i == 0 && !display.This {
				return nil, err
			}
			return nil, &ValidationError{
				Message:    fmt.Sprintf("type '%s' has no field '%s' (in %s)", currentType, field, display.String()),
				Suggestion: fmt.Sprintf("Available fields: %s", strings.Join(v.availableFields(currentDef), ", ")),
			}
		}
		isLast := i == len(fields)-1
		if isLast {
			return &resolvedPath{leafTypeName: currentType, leafDef: fieldDef, leafField: field}, nil
		}
		if !fieldDef.Type.IsRef() {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("cannot follow '%s' on field '%s' of type %s (in %s)", fields[i+1], field, fieldDef.Type, display.String()),
				Suggestion: "Named paths may only hop through scalar ref fields; use any()/all()/none() for ref[] elements",
			}
		}
		if fieldDef.Type.IsArray() {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("cannot follow '%s' through array field '%s' (in %s)", fields[i+1], field, display.String()),
				Suggestion: fmt.Sprintf("Use any(.%s, .%s==...) to quantify over ref[] elements", strings.Join(fields[:i+1], "."), fields[i+1]),
			}
		}
		if fieldDef.Target == "" {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("cannot follow '%s' on '%s' because field '%s' has no target type (in %s)", fields[i+1], field, field, display.String()),
				Suggestion: fmt.Sprintf("Declare target: on '%s.%s' in schema.yaml", currentType, field),
			}
		}
		next, ok := v.schema.Types[fieldDef.Target]
		if !ok {
			return nil, &ValidationError{
				Message:    fmt.Sprintf("unknown target type '%s' for field '%s' (in %s)", fieldDef.Target, field, display.String()),
				Suggestion: fmt.Sprintf("Available types: %s", strings.Join(v.availableTypes(), ", ")),
			}
		}
		currentType = fieldDef.Target
		currentDef = next
	}
	return &resolvedPath{leafTypeName: currentType}, nil
}
