package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// buildPathFieldPredicateSQL compiles .hop.leaf, this.field, and field-to-field
// comparisons. Simple `.field==literal` stays on the existing field builders.
func (e *Executor) buildPathFieldPredicateSQL(p *FieldPredicate, alias, typeName string) (string, []interface{}, error) {
	start, err := e.pathStartFrame(p.path(), alias, typeName)
	if err != nil {
		return "", nil, err
	}

	rhs := p.rhs()
	if p.IsExists {
		cond, args, err := e.applyOnPath(start, p.path(), func(land subjectFrame, leaf string) (string, []interface{}, error) {
			return e.leafExistsSQL(land, leaf)
		})
		if err != nil {
			return "", nil, err
		}
		return wrapNot(cond, p.Negated()), args, nil
	}

	if rhs.Path == nil {
		cond, args, err := e.applyOnPath(start, p.path(), func(land subjectFrame, leaf string) (string, []interface{}, error) {
			tmp := *p
			tmp.basePredicate.negated = false
			tmp.Path = fieldPathFromName(leaf)
			tmp.Field = leaf
			tmp.RHS = CompareRHS{}
			return e.buildLandedFieldCompareSQL(&tmp, land)
		})
		if err != nil {
			return "", nil, err
		}
		return wrapNot(cond, p.Negated()), args, nil
	}

	cond, args, err := e.buildPathToPathCompareSQL(start, p.path(), *rhs.Path, p.CompareOp)
	if err != nil {
		return "", nil, err
	}
	return wrapNot(cond, p.Negated()), args, nil
}

func (e *Executor) buildPathStringFuncPredicateSQL(p *StringFuncPredicate, alias, typeName string) (string, []interface{}, error) {
	start, err := e.pathStartFrame(p.path(), alias, typeName)
	if err != nil {
		return "", nil, err
	}
	cond, args, err := e.applyOnPath(start, p.path(), func(land subjectFrame, leaf string) (string, []interface{}, error) {
		tmp := *p
		tmp.basePredicate.negated = false
		tmp.Path = fieldPathFromName(leaf)
		tmp.Field = leaf
		return e.buildLandedStringFuncSQL(&tmp, land)
	})
	if err != nil {
		return "", nil, err
	}
	return wrapNot(cond, p.Negated()), args, nil
}

func (e *Executor) buildPathArrayQuantifierPredicateSQL(p *ArrayQuantifierPredicate, alias, typeName string) (string, []interface{}, error) {
	start, err := e.pathStartFrame(p.path(), alias, typeName)
	if err != nil {
		return "", nil, err
	}
	cond, args, err := e.applyOnPath(start, p.path(), func(land subjectFrame, leaf string) (string, []interface{}, error) {
		tmp := *p
		tmp.basePredicate.negated = false
		tmp.Path = fieldPathFromName(leaf)
		tmp.Field = leaf
		return e.buildArrayQuantifierPredicateSQL(&tmp, land.alias, land.typeName)
	})
	if err != nil {
		return "", nil, err
	}
	return wrapNot(cond, p.Negated()), args, nil
}

func (e *Executor) pathStartFrame(path FieldPath, alias, typeName string) (subjectFrame, error) {
	if path.This {
		return e.thisSubject()
	}
	if frame, ok := e.currentSubject(); ok {
		if alias != "" {
			frame.alias = alias
		}
		if typeName != "" {
			frame.typeName = typeName
		}
		return frame, nil
	}
	root := QueryTypeObject
	return subjectFrame{alias: alias, root: root, typeName: typeName}, nil
}

// applyOnPath walks named ref hops, then calls leaf on the landed subject.
// The last path component is the leaf field. Bare `this` is identity (leaf "").
func (e *Executor) applyOnPath(start subjectFrame, path FieldPath, leaf func(subjectFrame, string) (string, []interface{}, error)) (string, []interface{}, error) {
	if path.IsIdentity() {
		return leaf(start, "")
	}
	hops := path.Fields[:len(path.Fields)-1]
	leafField := path.Fields[len(path.Fields)-1]
	if len(hops) == 0 {
		return leaf(start, leafField)
	}
	return e.walkRefHops(start, hops, leafField, leaf)
}

func (e *Executor) walkRefHops(start subjectFrame, hops []string, leafField string, leaf func(subjectFrame, string) (string, []interface{}, error)) (string, []interface{}, error) {
	type hopFrame struct {
		src   subjectFrame
		field string
		dest  subjectFrame
	}

	current := start
	frames := make([]hopFrame, 0, len(hops))
	for _, field := range hops {
		dest := subjectFrame{
			alias:    e.nextAlias("obj"),
			root:     QueryTypeObject,
			typeName: e.fieldTargetType(current.typeName, field),
		}
		frames = append(frames, hopFrame{src: current, field: field, dest: dest})
		current = dest
	}

	inner, args, err := leaf(current, leafField)
	if err != nil {
		return "", nil, err
	}

	for i := len(frames) - 1; i >= 0; i-- {
		h := frames[i]
		refAlias := e.nextAlias("fr")
		joinMatch := refResolvedIDPreferredMatchSQL(refAlias, h.dest.alias+".id")
		sql := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			JOIN objects %s ON %s
			WHERE %s.source_id = %s.id AND %s.field_name = ? AND %s
		)`, refAlias, h.dest.alias, joinMatch, refAlias, h.src.alias, refAlias, inner)
		args = append([]interface{}{h.field}, args...)
		inner = sql
	}
	return inner, args, nil
}

func (e *Executor) leafExistsSQL(land subjectFrame, leaf string) (string, []interface{}, error) {
	if leaf == "" {
		return "1=1", nil, nil
	}
	if land.root == QueryTypeTrait && leaf == "value" {
		return fmt.Sprintf("%s.value IS NOT NULL", land.alias), nil, nil
	}
	if e.isRefField(land.typeName, leaf) {
		refAlias := e.nextAlias("fr")
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			WHERE %s.source_id = %s.id AND %s.field_name = ?
		)`, refAlias, refAlias, land.alias, refAlias)
		return cond, []interface{}{leaf}, nil
	}
	cond, args := fieldExistsCond(land.alias, jsonFieldPath(leaf), false)
	return cond, args, nil
}

func (e *Executor) buildLandedFieldCompareSQL(p *FieldPredicate, land subjectFrame) (string, []interface{}, error) {
	switch land.root {
	case QueryTypeTrait:
		if p.Field != "value" && p.Field != "" {
			return "", nil, fmt.Errorf("unsupported trait field predicate: .%s", p.Field)
		}
		p.Field = "value"
		return e.buildTraitValueFieldPredicateSQL(p, land.alias)
	case QueryTypeSection:
		return e.buildSectionFieldPredicateSQL(p, land.alias)
	case QueryTypeLink:
		return e.buildLinkPredicateSQL(p, land.alias)
	default:
		return e.buildFieldPredicateSQL(p, land.alias, land.typeName)
	}
}

func (e *Executor) buildLandedStringFuncSQL(p *StringFuncPredicate, land subjectFrame) (string, []interface{}, error) {
	switch land.root {
	case QueryTypeTrait:
		return e.buildTraitStringFuncPredicateSQL(p, land.alias)
	case QueryTypeSection:
		return e.buildSectionStringFuncPredicateSQL(p, land.alias)
	case QueryTypeLink:
		return e.buildLinkPredicateSQL(p, land.alias)
	default:
		return e.buildStringFuncPredicateSQL(p, land.alias, land.typeName)
	}
}

func (e *Executor) buildPathToPathCompareSQL(start subjectFrame, lhs, rhs FieldPath, op CompareOp) (string, []interface{}, error) {
	lhsRef := e.pathLeafIsRef(start, lhs)
	rhsRef := e.pathLeafIsRef(start, rhs)
	if (lhs.IsIdentity() || lhsRef) && (rhs.IsIdentity() || rhsRef) {
		if op != CompareEq && op != CompareNeq {
			return "", nil, fmt.Errorf("unsupported comparison for reference paths (use == or !=)")
		}
		cond, args, err := e.buildRefIdentityCompareSQL(start, lhs, rhs)
		if err != nil {
			return "", nil, err
		}
		if op == CompareNeq {
			cond = "NOT (" + cond + ")"
		}
		return cond, args, nil
	}
	return e.buildScalarPathCompareSQL(start, lhs, rhs, op)
}

func (e *Executor) buildRefIdentityCompareSQL(start subjectFrame, lhs, rhs FieldPath) (string, []interface{}, error) {
	rhsID, rhsArgs, err := e.pathIdentityExpr(start, rhs)
	if err != nil {
		return "", nil, err
	}

	if lhs.IsIdentity() {
		lhsFrame, err := e.pathStartFrame(lhs, start.alias, start.typeName)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("%s.id = %s", lhsFrame.alias, rhsID), rhsArgs, nil
	}

	return e.applyOnPath(start, lhs, func(land subjectFrame, field string) (string, []interface{}, error) {
		if field == "" {
			return fmt.Sprintf("%s.id = %s", land.alias, rhsID), rhsArgs, nil
		}
		if land.root == QueryTypeTrait && field == "value" {
			return e.traitValueMatchesIdentitySQL(land.alias, rhsID, rhsArgs)
		}
		if e.schema != nil && !e.isRefField(land.typeName, field) {
			return e.scalarEqualsExpr(land, field, rhsID, rhsArgs)
		}
		refAlias := e.nextAlias("fr")
		matchSQL, matchArgs := refResolvedIDPreferredMatchExpr(refAlias, rhsID, rhsArgs)
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			WHERE %s.source_id = %s.id AND %s.field_name = ? AND %s
		)`, refAlias, refAlias, land.alias, refAlias, matchSQL)
		args := append([]interface{}{field}, matchArgs...)
		return cond, args, nil
	})
}

func (e *Executor) traitValueMatchesIdentitySQL(traitAlias, idExpr string, idArgs []interface{}) (string, []interface{}, error) {
	refAlias := e.nextAlias("fr")
	matchSQL, matchArgs := refResolvedIDPreferredMatchExpr(refAlias, idExpr, idArgs)
	cond := fmt.Sprintf(`(LOWER(%[1]s.value) = LOWER(%[2]s) OR EXISTS (
		SELECT 1 FROM refs %[3]s
		WHERE %[3]s.file_path = %[1]s.file_path AND %[3]s.line_number = %[1]s.line_number AND %[4]s
	))`, traitAlias, idExpr, refAlias, matchSQL)
	args := append([]interface{}{}, idArgs...)
	args = append(args, matchArgs...)
	return cond, args, nil
}

func (e *Executor) scalarEqualsExpr(land subjectFrame, field, expr string, exprArgs []interface{}) (string, []interface{}, error) {
	fieldExpr, fieldArgs := e.scalarFieldExpr(land, field)
	cond := fmt.Sprintf("LOWER(CAST(%s AS TEXT)) = LOWER(CAST(%s AS TEXT))", fieldExpr, expr)
	args := append(fieldArgs, exprArgs...)
	return cond, args, nil
}

func (e *Executor) buildScalarPathCompareSQL(start subjectFrame, lhs, rhs FieldPath, op CompareOp) (string, []interface{}, error) {
	lhsExpr, lhsArgs, err := e.pathScalarExpr(start, lhs)
	if err != nil {
		return "", nil, err
	}
	rhsExpr, rhsArgs, err := e.pathScalarExpr(start, rhs)
	if err != nil {
		return "", nil, err
	}
	sqlOp := compareOpToSQL(op)
	var cond string
	switch {
	case op == CompareEq || op == CompareNeq:
		cond = fmt.Sprintf("LOWER(CAST(%s AS TEXT)) %s LOWER(CAST(%s AS TEXT))", lhsExpr, sqlOp, rhsExpr)
	case e.pathLeafIsNumber(start, lhs) || e.pathLeafIsNumber(start, rhs):
		cond = fmt.Sprintf("CAST(%s AS REAL) %s CAST(%s AS REAL)", lhsExpr, sqlOp, rhsExpr)
	default:
		cond = fmt.Sprintf("%s %s %s", lhsExpr, sqlOp, rhsExpr)
	}
	return cond, append(lhsArgs, rhsArgs...), nil
}

func (e *Executor) pathScalarExpr(start subjectFrame, path FieldPath) (string, []interface{}, error) {
	frame, err := e.pathStartFrame(path, start.alias, start.typeName)
	if err != nil {
		return "", nil, err
	}
	if path.IsIdentity() {
		if frame.root == QueryTypeTrait {
			return frame.alias + ".value", nil, nil
		}
		return frame.alias + ".id", nil, nil
	}
	hops := path.Fields[:len(path.Fields)-1]
	leaf := path.Fields[len(path.Fields)-1]
	if len(hops) == 0 {
		expr, args := e.scalarFieldExpr(frame, leaf)
		return expr, args, nil
	}
	return e.pathScalarSubquery(frame, hops, leaf)
}

func (e *Executor) pathScalarSubquery(start subjectFrame, hops []string, leaf string) (string, []interface{}, error) {
	currentAlias := start.alias
	curType := start.typeName
	var b strings.Builder
	args := make([]interface{}, 0, len(hops)+1)
	b.WriteString("(SELECT __leaf__ FROM ")
	for i, field := range hops {
		refAlias := e.nextAlias("fr")
		objAlias := e.nextAlias("obj")
		if i == 0 {
			fmt.Fprintf(&b, "refs %s JOIN objects %s ON %s WHERE %s.source_id = %s.id AND %s.field_name = ?",
				refAlias, objAlias, refResolvedIDPreferredMatchSQL(refAlias, objAlias+".id"),
				refAlias, currentAlias, refAlias)
		} else {
			fmt.Fprintf(&b, " JOIN refs %s ON %s.source_id = %s.id AND %s.field_name = ? JOIN objects %s ON %s",
				refAlias, refAlias, currentAlias, refAlias, objAlias, refResolvedIDPreferredMatchSQL(refAlias, objAlias+".id"))
		}
		args = append(args, field)
		currentAlias = objAlias
		curType = e.fieldTargetType(curType, field)
	}
	b.WriteString(" LIMIT 1)")
	leafExpr, leafArgs := e.scalarFieldExpr(subjectFrame{alias: currentAlias, root: QueryTypeObject, typeName: curType}, leaf)
	sql := strings.Replace(b.String(), "__leaf__", leafExpr, 1)
	return sql, append(args, leafArgs...), nil
}

func (e *Executor) scalarFieldExpr(land subjectFrame, field string) (string, []interface{}) {
	if land.root == QueryTypeTrait && (field == "value" || field == "") {
		return land.alias + ".value", nil
	}
	if isDateVirtualField(land.typeName, field) {
		return dateVirtualFieldExpr(land.alias), nil
	}
	if land.root == QueryTypeSection {
		if col, ok := sectionFieldColumn(land.alias, field); ok {
			return col, nil
		}
	}
	if land.root == QueryTypeLink {
		if col, ok := linkColumnExpr(land.alias, field); ok {
			return col, nil
		}
	}
	return fmt.Sprintf("json_extract(%s.fields, ?)", land.alias), []interface{}{jsonFieldPath(field)}
}

func (e *Executor) pathIdentityExpr(start subjectFrame, path FieldPath) (string, []interface{}, error) {
	frame, err := e.pathStartFrame(path, start.alias, start.typeName)
	if err != nil {
		return "", nil, err
	}
	if path.IsIdentity() {
		if frame.root == QueryTypeTrait {
			return frame.alias + ".value", nil, nil
		}
		return frame.alias + ".id", nil, nil
	}
	return e.nestedRefIdentityExpr(frame, path.Fields)
}

func (e *Executor) nestedRefIdentityExpr(start subjectFrame, fields []string) (string, []interface{}, error) {
	if len(fields) == 0 {
		return start.alias + ".id", nil, nil
	}
	if len(fields) == 1 {
		refAlias := e.nextAlias("fr")
		sql := fmt.Sprintf("(SELECT COALESCE(%s.target_id, %s.target_raw) FROM refs %s WHERE %s.source_id = %s.id AND %s.field_name = ? LIMIT 1)",
			refAlias, refAlias, refAlias, refAlias, start.alias, refAlias)
		return sql, []interface{}{fields[0]}, nil
	}
	refAlias := e.nextAlias("fr")
	objAlias := e.nextAlias("obj")
	innerStart := subjectFrame{alias: objAlias, root: QueryTypeObject, typeName: e.fieldTargetType(start.typeName, fields[0])}
	inner, innerArgs, err := e.nestedRefIdentityExpr(innerStart, fields[1:])
	if err != nil {
		return "", nil, err
	}
	sql := fmt.Sprintf(`(SELECT %s FROM refs %s JOIN objects %s ON %s WHERE %s.source_id = %s.id AND %s.field_name = ? LIMIT 1)`,
		inner, refAlias, objAlias, refResolvedIDPreferredMatchSQL(refAlias, objAlias+".id"), refAlias, start.alias, refAlias)
	return sql, append([]interface{}{fields[0]}, innerArgs...), nil
}

func (e *Executor) pathLeafIsRef(start subjectFrame, path FieldPath) bool {
	if path.IsIdentity() {
		return true
	}
	frame, err := e.pathStartFrame(path, start.alias, start.typeName)
	if err != nil {
		return false
	}
	typeName := frame.typeName
	for i, field := range path.Fields {
		if i == len(path.Fields)-1 {
			if frame.root == QueryTypeTrait && field == "value" {
				return e.traitValueIsRef(typeName)
			}
			return e.isRefField(typeName, field)
		}
		typeName = e.fieldTargetType(typeName, field)
	}
	return false
}

func (e *Executor) pathLeafIsNumber(start subjectFrame, path FieldPath) bool {
	if path.IsIdentity() || e.schema == nil {
		return false
	}
	frame, err := e.pathStartFrame(path, start.alias, start.typeName)
	if err != nil {
		return false
	}
	typeName := frame.typeName
	typeDef := e.schema.Types[typeName]
	for i, field := range path.Fields {
		if typeDef == nil {
			return false
		}
		fieldDef := typeDef.Fields[field]
		if fieldDef == nil {
			return false
		}
		if i == len(path.Fields)-1 {
			return fieldDef.Type == schema.FieldTypeNumber
		}
		if !fieldDef.Type.IsRef() {
			return false
		}
		typeName = fieldDef.Target
		typeDef = e.schema.Types[typeName]
	}
	return false
}

func (e *Executor) fieldTargetType(typeName, field string) string {
	if e.schema == nil || typeName == "" {
		return ""
	}
	typeDef := e.schema.Types[typeName]
	if typeDef == nil {
		return ""
	}
	fieldDef := typeDef.Fields[field]
	if fieldDef == nil {
		return ""
	}
	return fieldDef.Target
}

func (e *Executor) traitValueIsRef(traitName string) bool {
	if e.schema == nil || traitName == "" {
		return false
	}
	traitDef := e.schema.Traits[traitName]
	return traitDef != nil && traitDef.Type.IsRef()
}

func elementPredNeedsObject(pred Predicate) bool {
	if pred == nil {
		return false
	}
	switch p := pred.(type) {
	case *FieldPredicate:
		return !p.path().IsIdentity()
	case *StringFuncPredicate:
		return !p.IsElementRef
	case *OrPredicate:
		for _, sub := range p.Predicates {
			if elementPredNeedsObject(sub) {
				return true
			}
		}
	case *GroupPredicate:
		for _, sub := range p.Predicates {
			if elementPredNeedsObject(sub) {
				return true
			}
		}
	case *NotPredicate:
		return elementPredNeedsObject(p.Inner)
	}
	return false
}

func (e *Executor) buildElementThisEqualitySQL(p *ElementEqualityPredicate, refAlias string) (string, []interface{}, error) {
	rhs := p.rhs()
	if rhs.Path == nil {
		return "", nil, fmt.Errorf("element equality expected a path RHS")
	}
	thisFrame, err := e.thisSubject()
	if err != nil {
		return "", nil, err
	}
	idExpr, idArgs, err := e.pathIdentityExpr(thisFrame, *rhs.Path)
	if err != nil {
		return "", nil, err
	}
	match, matchArgs := refResolvedIDPreferredMatchExpr(refAlias, idExpr, idArgs)
	if p.CompareOp == CompareNeq {
		match = "NOT " + match
	}
	return wrapNot(match, p.Negated()), matchArgs, nil
}
