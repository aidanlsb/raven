package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

// buildFieldPredicateSQL builds SQL for .field==value predicates.
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildFieldPredicateSQL(p *FieldPredicate, alias, typeName string) (string, []interface{}, error) {
	jsonPath := jsonFieldPath(p.Field)

	if p.IsExists {
		cond, args := fieldExistsCond(alias, jsonPath, p.CompareOp == CompareNeq)
		if p.Negated() {
			cond = "NOT (" + cond + ")"
		}
		return cond, args, nil
	}

	if e.isRefField(typeName, p.Field) {
		return e.buildRefFieldPredicateSQL(p, alias, typeName)
	}

	var cond string
	var args []interface{}
	value := p.Value
	altValue := ""
	mode := e.fieldEqualityMode(typeName, p.Field)

	if p.IsRefValue && (p.CompareOp == CompareEq || p.CompareOp == CompareNeq) {
		resolved, alt, err := e.resolveRefValue(p.Value)
		if err != nil {
			return "", nil, err
		}
		value = resolved
		altValue = alt
	}

	// Date/date-keyword values should use date-aware comparisons.
	if !p.IsRefValue {
		dateFieldExpr := fmt.Sprintf("json_extract(%s.fields, ?)", alias)
		dateCond, dateArgs, ok, err := buildDateFieldCompareCondition(value, p.CompareOp, dateFieldExpr, jsonPath, e.queryNow())
		if err != nil {
			return "", nil, err
		}
		if ok {
			if p.Negated() {
				dateCond = "NOT (" + dateCond + ")"
			}
			return dateCond, dateArgs, nil
		}
	}

	if p.CompareOp == CompareNeq {
		if altValue != "" {
			cond1, args1 := fieldScalarOrArrayCIEqualsCond(alias, jsonPath, value, true, mode)
			cond2, args2 := fieldScalarOrArrayCIEqualsCond(alias, jsonPath, altValue, true, mode)
			cond = "(" + cond1 + " AND " + cond2 + ")"
			args = append(args1, args2...)
		} else {
			cond, args = fieldScalarOrArrayCIEqualsCond(alias, jsonPath, value, true, mode)
		}
	} else if p.CompareOp != CompareEq {
		// Comparison operators: <, >, <=, >=
		op := compareOpToSQL(p.CompareOp)
		// Prefer numeric comparisons when RHS parses as a number. This avoids
		// lexicographic comparisons like "10" < "2".
		if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			cond = fmt.Sprintf("CAST(json_extract(%s.fields, ?) AS REAL) %s ?", alias, op)
			args = append(args, jsonPath, n)
		} else {
			cond = fmt.Sprintf("json_extract(%s.fields, ?) %s ?", alias, op)
			args = append(args, jsonPath, value)
		}
	} else {
		if altValue != "" {
			cond1, args1 := fieldScalarOrArrayCIEqualsCond(alias, jsonPath, value, false, mode)
			cond2, args2 := fieldScalarOrArrayCIEqualsCond(alias, jsonPath, altValue, false, mode)
			cond = "(" + cond1 + " OR " + cond2 + ")"
			args = append(args1, args2...)
		} else {
			cond, args = fieldScalarOrArrayCIEqualsCond(alias, jsonPath, value, false, mode)
		}
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

func buildDateFieldCompareCondition(value string, compareOp CompareOp, fieldExpr string, jsonPath string, now time.Time) (string, []interface{}, bool, error) {
	cond, dateArgs, ok, err := index.TryParseDateComparisonWithOptions(
		value,
		compareOpToSQL(compareOp),
		fieldExpr,
		index.DateFilterOptions{
			Now: now,
		},
	)
	if err != nil {
		return "", nil, false, err
	}
	if !ok {
		return "", nil, false, nil
	}

	// fieldExpr contains a placeholder for the JSON path. Inject one path argument
	// for each field expression occurrence before date boundary args.
	pathArgCount := strings.Count(cond, fieldExpr)
	args := make([]interface{}, 0, pathArgCount+len(dateArgs))
	for i := 0; i < pathArgCount; i++ {
		args = append(args, jsonPath)
	}
	args = append(args, dateArgs...)

	return cond, args, true, nil
}

// resolveRefValue resolves a reference token and returns a canonical value plus a
// fallback value (original input) when they differ.
func (e *Executor) resolveRefValue(value string) (resolved string, fallback string, err error) {
	resolved, err = e.resolveTarget(value)
	if err != nil {
		return "", "", err
	}
	if resolved != value {
		return resolved, value, nil
	}
	return resolved, "", nil
}

func (e *Executor) isRefField(typeName, fieldName string) bool {
	if e.schema == nil || typeName == "" {
		return false
	}
	typeDef := e.schema.Types[typeName]
	if typeDef == nil {
		return false
	}
	fieldDef := typeDef.Fields[fieldName]
	if fieldDef == nil {
		return false
	}
	return fieldDef.Type == schema.FieldTypeRef || fieldDef.Type == schema.FieldTypeRefArray
}

func (e *Executor) isRefArrayField(typeName, fieldName string) bool {
	if e.schema == nil || typeName == "" {
		return false
	}
	typeDef := e.schema.Types[typeName]
	if typeDef == nil {
		return false
	}
	fieldDef := typeDef.Fields[fieldName]
	if fieldDef == nil {
		return false
	}
	return fieldDef.Type == schema.FieldTypeRefArray
}

func (e *Executor) fieldEqualityMode(typeName, fieldName string) fieldEqualityMode {
	if e.schema == nil || typeName == "" {
		return fieldEqualityModeFallback
	}
	typeDef := e.schema.Types[typeName]
	if typeDef == nil {
		return fieldEqualityModeFallback
	}
	fieldDef := typeDef.Fields[fieldName]
	if fieldDef == nil {
		return fieldEqualityModeFallback
	}
	if strings.HasSuffix(string(fieldDef.Type), "[]") {
		return fieldEqualityModeArray
	}
	return fieldEqualityModeScalar
}

func fieldRefMatchCond(alias string) string {
	return fmt.Sprintf("(%s.target_id = ? OR (%s.target_id IS NULL AND %s.target_raw = ?))", alias, alias, alias)
}

type fieldRefAmbiguityKey struct {
	typeName       string
	fieldName      string
	rawValue       string
	resolvedTarget string
}

type fieldRefAmbiguityResult struct {
	err error
}

func (e *Executor) prepareRefFieldAmbiguityChecks(q *Query) error {
	if e.schema == nil || e.db == nil || q == nil || q.Type != QueryTypeObject || q.Predicate == nil {
		return nil
	}
	if e.fieldRefAmbiguityCache == nil {
		e.fieldRefAmbiguityCache = make(map[fieldRefAmbiguityKey]fieldRefAmbiguityResult)
	}

	keys := make(map[fieldRefAmbiguityKey]struct{})
	if err := e.collectRefFieldAmbiguityKeys(q, keys); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	missing := make([]fieldRefAmbiguityKey, 0, len(keys))
	for key := range keys {
		if _, ok := e.fieldRefAmbiguityCache[key]; !ok {
			missing = append(missing, key)
		}
	}
	return e.loadFieldRefAmbiguityResults(missing)
}

func (e *Executor) collectRefFieldAmbiguityKeys(q *Query, keys map[fieldRefAmbiguityKey]struct{}) error {
	if q == nil || q.Predicate == nil {
		return nil
	}
	return e.collectRefFieldAmbiguityPredicate(q.Type, q.TypeName, q.Predicate, keys)
}

func (e *Executor) collectRefFieldAmbiguityPredicate(queryType QueryType, typeName string, pred Predicate, keys map[fieldRefAmbiguityKey]struct{}) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		if queryType != QueryTypeObject || !e.isRefField(typeName, p.Field) {
			return nil
		}
		if p.CompareOp != CompareEq && p.CompareOp != CompareNeq {
			return nil
		}
		resolved, _, err := e.resolveRefValue(p.Value)
		if err != nil {
			return err
		}
		keys[fieldRefAmbiguityKey{
			typeName:       typeName,
			fieldName:      p.Field,
			rawValue:       p.Value,
			resolvedTarget: resolved,
		}] = struct{}{}
		return nil
	case *ArrayQuantifierPredicate:
		if queryType != QueryTypeObject || !e.isRefArrayField(typeName, p.Field) {
			return nil
		}
		elem, ok := p.ElementPred.(*ElementEqualityPredicate)
		if !ok {
			return nil
		}
		if elem.CompareOp != CompareEq && elem.CompareOp != CompareNeq {
			return nil
		}
		resolved, _, err := e.resolveRefValue(elem.Value)
		if err != nil {
			return err
		}
		keys[fieldRefAmbiguityKey{
			typeName:       typeName,
			fieldName:      p.Field,
			rawValue:       elem.Value,
			resolvedTarget: resolved,
		}] = struct{}{}
		return nil
	case *HasPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *ParentPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *AncestorPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *ChildPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *DescendantPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *ContainsPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *RefsPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *OnPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *WithinPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *AtPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *RefdPredicate:
		return e.collectRefFieldAmbiguityKeys(p.SubQuery, keys)
	case *OrPredicate:
		for _, sub := range p.Predicates {
			if err := e.collectRefFieldAmbiguityPredicate(queryType, typeName, sub, keys); err != nil {
				return err
			}
		}
	case *GroupPredicate:
		for _, sub := range p.Predicates {
			if err := e.collectRefFieldAmbiguityPredicate(queryType, typeName, sub, keys); err != nil {
				return err
			}
		}
	case *NotPredicate:
		return e.collectRefFieldAmbiguityPredicate(queryType, typeName, p.Inner, keys)
	}
	return nil
}

func (e *Executor) loadFieldRefAmbiguityResults(keys []fieldRefAmbiguityKey) error {
	if e.schema == nil || e.db == nil || len(keys) == 0 {
		return nil
	}
	if e.fieldRefAmbiguityCache == nil {
		e.fieldRefAmbiguityCache = make(map[fieldRefAmbiguityKey]fieldRefAmbiguityResult)
	}

	type querySet struct {
		key        fieldRefAmbiguityKey
		candidates []string
	}

	sets := make([]querySet, 0, len(keys))
	matchIndex := make(map[string][]fieldRefAmbiguityKey)
	args := make([]interface{}, 0, len(keys)*4)
	clauses := make([]string, 0, len(keys))

	for _, key := range keys {
		if _, ok := e.fieldRefAmbiguityCache[key]; ok {
			continue
		}

		candidates := []string{key.rawValue}
		if key.resolvedTarget != "" {
			shortName := paths.ShortNameFromID(key.resolvedTarget)
			if shortName != "" && shortName != key.rawValue {
				candidates = append(candidates, shortName)
			}
		}
		candidates = dedupeStrings(candidates)
		if len(candidates) == 0 {
			e.fieldRefAmbiguityCache[key] = fieldRefAmbiguityResult{}
			continue
		}

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(candidates)), ",")
		clauses = append(clauses, fmt.Sprintf("(o.type = ? AND fr.field_name = ? AND fr.target_raw IN (%s))", placeholders))
		args = append(args, key.typeName, key.fieldName)
		for _, candidate := range candidates {
			args = append(args, candidate)
			indexKey := key.typeName + "\x00" + key.fieldName + "\x00" + candidate
			matchIndex[indexKey] = append(matchIndex[indexKey], key)
		}
		sets = append(sets, querySet{key: key, candidates: candidates})
	}

	if len(clauses) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		SELECT o.type, fr.field_name, fr.target_raw
		FROM field_refs fr
		JOIN objects o ON fr.source_id = o.id
		WHERE fr.resolution_status = 'ambiguous'
		  AND (%s)
	`, strings.Join(clauses, " OR "))

	if e.ambiguousFieldRefQueryHook != nil {
		e.ambiguousFieldRefQueryHook()
	}

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	ambiguous := make(map[fieldRefAmbiguityKey]struct{})
	for rows.Next() {
		var typeName, fieldName, targetRaw string
		if err := rows.Scan(&typeName, &fieldName, &targetRaw); err != nil {
			return err
		}
		indexKey := typeName + "\x00" + fieldName + "\x00" + targetRaw
		for _, key := range matchIndex[indexKey] {
			ambiguous[key] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, set := range sets {
		if _, ok := ambiguous[set.key]; ok {
			e.fieldRefAmbiguityCache[set.key] = fieldRefAmbiguityResult{
				err: newExecutionError(
					fmt.Sprintf("ambiguous reference in field '%s' for value '%s' (disambiguate the field value before querying)", set.key.fieldName, set.key.rawValue),
					"Use a full object ID/path in the query value to disambiguate",
					nil,
				),
			}
			continue
		}
		e.fieldRefAmbiguityCache[set.key] = fieldRefAmbiguityResult{}
	}

	return nil
}

func (e *Executor) checkAmbiguousFieldRefs(typeName, fieldName, rawValue, resolvedTarget string) error {
	if e.schema == nil || typeName == "" || e.db == nil {
		return nil
	}
	if e.fieldRefAmbiguityCache == nil {
		e.fieldRefAmbiguityCache = make(map[fieldRefAmbiguityKey]fieldRefAmbiguityResult)
	}
	key := fieldRefAmbiguityKey{
		typeName:       typeName,
		fieldName:      fieldName,
		rawValue:       rawValue,
		resolvedTarget: resolvedTarget,
	}
	if cached, ok := e.fieldRefAmbiguityCache[key]; ok {
		return cached.err
	}
	if err := e.loadFieldRefAmbiguityResults([]fieldRefAmbiguityKey{key}); err != nil {
		e.fieldRefAmbiguityCache[key] = fieldRefAmbiguityResult{err: err}
		return err
	}
	return e.fieldRefAmbiguityCache[key].err
}

func (e *Executor) buildRefFieldPredicateSQL(p *FieldPredicate, alias, typeName string) (string, []interface{}, error) {
	if p.CompareOp != CompareEq && p.CompareOp != CompareNeq {
		return "", nil, fmt.Errorf("unsupported comparison for ref field '.%s' (use == or !=)", p.Field)
	}

	resolved, _, err := e.resolveRefValue(p.Value)
	if err != nil {
		return "", nil, err
	}

	if err := e.checkAmbiguousFieldRefs(typeName, p.Field, p.Value, resolved); err != nil {
		return "", nil, err
	}

	matchCond := fieldRefMatchCond("fr")
	if p.CompareOp == CompareNeq {
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM field_refs fr
			WHERE fr.source_id = %s.id AND fr.field_name = ?
		) AND NOT EXISTS (
			SELECT 1 FROM field_refs fr
			WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
		)`, alias, alias, matchCond)
		args := []interface{}{p.Field, p.Field, resolved, p.Value}
		if p.Negated() {
			cond = "NOT (" + cond + ")"
		}
		return cond, args, nil
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM field_refs fr
		WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
	)`, alias, matchCond)
	args := []interface{}{p.Field, resolved, p.Value}
	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

func (e *Executor) buildRefArrayQuantifierPredicateSQL(p *ArrayQuantifierPredicate, elem *ElementEqualityPredicate, alias, typeName string) (string, []interface{}, error) {
	if elem.CompareOp != CompareEq && elem.CompareOp != CompareNeq {
		return "", nil, fmt.Errorf("unsupported comparison for ref array field '.%s' (use == or !=)", p.Field)
	}

	resolved, _, err := e.resolveRefValue(elem.Value)
	if err != nil {
		return "", nil, err
	}

	if err := e.checkAmbiguousFieldRefs(typeName, p.Field, elem.Value, resolved); err != nil {
		return "", nil, err
	}

	matchCond := fieldRefMatchCond("fr")
	notMatchCond := "NOT " + matchCond

	var cond string
	var args []interface{}

	switch p.Quantifier {
	case ArrayQuantifierAny:
		if elem.CompareOp == CompareEq {
			cond = fmt.Sprintf(`EXISTS (
				SELECT 1 FROM field_refs fr
				WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
			)`, alias, matchCond)
			args = []interface{}{p.Field, resolved, elem.Value}
		} else {
			cond = fmt.Sprintf(`EXISTS (
				SELECT 1 FROM field_refs fr
				WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
			)`, alias, notMatchCond)
			args = []interface{}{p.Field, resolved, elem.Value}
		}
	case ArrayQuantifierNone:
		if elem.CompareOp == CompareEq {
			cond = fmt.Sprintf(`NOT EXISTS (
				SELECT 1 FROM field_refs fr
				WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
			)`, alias, matchCond)
			args = []interface{}{p.Field, resolved, elem.Value}
		} else {
			cond = fmt.Sprintf(`NOT EXISTS (
				SELECT 1 FROM field_refs fr
				WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
			)`, alias, notMatchCond)
			args = []interface{}{p.Field, resolved, elem.Value}
		}
	case ArrayQuantifierAll:
		if elem.CompareOp == CompareEq {
			cond = fmt.Sprintf(`NOT EXISTS (
				SELECT 1 FROM field_refs fr
				WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
			)`, alias, notMatchCond)
			args = []interface{}{p.Field, resolved, elem.Value}
		} else {
			cond = fmt.Sprintf(`NOT EXISTS (
				SELECT 1 FROM field_refs fr
				WHERE fr.source_id = %s.id AND fr.field_name = ? AND %s
			)`, alias, matchCond)
			args = []interface{}{p.Field, resolved, elem.Value}
		}
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// buildStringFuncPredicateSQL builds SQL for string function predicates.
// Handles: includes(.field, "str"), startswith(.field, "str"), endswith(.field, "str"), matches(.field, "pattern")
func (e *Executor) buildStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	jsonPath := jsonFieldPath(p.Field)
	fieldExpr := fmt.Sprintf("json_extract(%s.fields, ?)", alias)

	cond, funcArgs, err := buildStringFuncCondition(p.FuncType, fieldExpr, p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}
	args := append([]interface{}{jsonPath}, funcArgs...)

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildArrayQuantifierPredicateSQL builds SQL for array quantifier predicates.
// Handles: any(.tags, _ == "urgent"), all(.tags, startswith(_, "feature-")), none(.tags, _ == "deprecated")
func (e *Executor) buildArrayQuantifierPredicateSQL(p *ArrayQuantifierPredicate, alias, typeName string) (string, []interface{}, error) {
	if e.isRefArrayField(typeName, p.Field) {
		if elem, ok := p.ElementPred.(*ElementEqualityPredicate); ok {
			return e.buildRefArrayQuantifierPredicateSQL(p, elem, alias, typeName)
		}
	}

	jsonPath := fmt.Sprintf("$.%s", p.Field)

	var cond string
	var args []interface{}

	// Build the element condition
	elemCond, elemArgs, err := e.buildElementPredicateSQL(p.ElementPred)
	if err != nil {
		return "", nil, err
	}

	switch p.Quantifier {
	case ArrayQuantifierAny:
		// EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE <elemCond>)
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE %s
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)

	case ArrayQuantifierAll:
		// NOT EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE NOT <elemCond>)
		// This means: there is no element that doesn't satisfy the condition
		cond = fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE NOT (%s)
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)

	case ArrayQuantifierNone:
		// NOT EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE <elemCond>)
		cond = fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE %s
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildElementPredicateSQL builds SQL for predicates used within array quantifiers.
// The context is json_each.value representing the current array element.
func (e *Executor) buildElementPredicateSQL(pred Predicate) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *ElementEqualityPredicate:
		return e.buildElementEqualitySQL(p)

	case *StringFuncPredicate:
		if !p.IsElementRef {
			return "", nil, fmt.Errorf("string function in array context must use _ as first argument")
		}
		return e.buildElementStringFuncSQL(p)

	case *OrPredicate:
		var conditions []string
		var args []interface{}
		for _, subPred := range p.Predicates {
			cond, predArgs, err := e.buildElementPredicateSQL(subPred)
			if err != nil {
				return "", nil, err
			}
			conditions = append(conditions, cond)
			args = append(args, predArgs...)
		}
		return "(" + strings.Join(conditions, " OR ") + ")", args, nil

	case *NotPredicate:
		cond, args, err := e.buildElementPredicateSQL(p.Inner)
		if err != nil {
			return "", nil, err
		}
		return "NOT (" + cond + ")", args, nil

	case *GroupPredicate:
		var conditions []string
		var args []interface{}
		for _, subPred := range p.Predicates {
			cond, predArgs, err := e.buildElementPredicateSQL(subPred)
			if err != nil {
				return "", nil, err
			}
			conditions = append(conditions, cond)
			args = append(args, predArgs...)
		}
		return "(" + strings.Join(conditions, " AND ") + ")", args, nil

	default:
		return "", nil, fmt.Errorf("unsupported element predicate type: %T", pred)
	}
}

// buildElementEqualitySQL builds SQL for _ == value or _ != value.
func (e *Executor) buildElementEqualitySQL(p *ElementEqualityPredicate) (string, []interface{}, error) {
	// Reuse the same value-comparison SQL semantics as value predicates:
	// - numeric RHS => numeric compare via CAST(... AS REAL)
	// - string equality => case-insensitive
	value := p.Value
	altValue := ""

	if p.IsRefValue && (p.CompareOp == CompareEq || p.CompareOp == CompareNeq) {
		resolved, alt, err := e.resolveRefValue(p.Value)
		if err != nil {
			return "", nil, err
		}
		value = resolved
		altValue = alt
	}

	condFor := func(v string) (string, []interface{}) {
		vp := &ValuePredicate{
			Value:     v,
			CompareOp: p.CompareOp,
		}
		return e.buildValueCondition(vp, "json_each.value")
	}

	cond, args := condFor(value)
	if altValue != "" && (p.CompareOp == CompareEq || p.CompareOp == CompareNeq) {
		cond2, args2 := condFor(altValue)
		if p.CompareOp == CompareEq {
			cond = "(" + cond + " OR " + cond2 + ")"
		} else {
			cond = "(" + cond + " AND " + cond2 + ")"
		}
		args = append(args, args2...)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

// buildElementStringFuncSQL builds SQL for string functions on array elements.
func (e *Executor) buildElementStringFuncSQL(p *StringFuncPredicate) (string, []interface{}, error) {
	cond, args, err := buildStringFuncCondition(p.FuncType, "json_each.value", p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}
