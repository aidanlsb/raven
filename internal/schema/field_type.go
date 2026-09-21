package schema

import "strings"

// arrayElementTypes is the canonical mapping from array FieldTypes to their
// scalar element types.
var arrayElementTypes = map[FieldType]FieldType{
	FieldTypeStringArray:   FieldTypeString,
	FieldTypeNumberArray:   FieldTypeNumber,
	FieldTypeURLArray:      FieldTypeURL,
	FieldTypeDateArray:     FieldTypeDate,
	FieldTypeDatetimeArray: FieldTypeDatetime,
	FieldTypeEnumArray:     FieldTypeEnum,
	FieldTypeBoolArray:     FieldTypeBool,
	FieldTypeRefArray:      FieldTypeRef,
}

// ElementType reports the scalar element type for an array FieldType.
// The bool is false for scalar and unknown types.
func (t FieldType) ElementType() (FieldType, bool) {
	elem, ok := arrayElementTypes[t]
	return elem, ok
}

// IsArray reports whether t is a declared array FieldType.
func (t FieldType) IsArray() bool {
	_, ok := t.ElementType()
	return ok
}

// IsRef reports whether t is ref or ref[].
func (t FieldType) IsRef() bool {
	return t.scalarType() == FieldTypeRef
}

// IsEnum reports whether t is enum or enum[].
func (t FieldType) IsEnum() bool {
	return t.scalarType() == FieldTypeEnum
}

// IsBool reports whether t is bool or bool[].
func (t FieldType) IsBool() bool {
	return t.scalarType() == FieldTypeBool
}

// IsStringLike reports whether t is a scalar type that stores a string value
// (string, url, date, datetime, enum, or ref). Array types are not string-like;
// use ElementType() first when checking array elements.
func (t FieldType) IsStringLike() bool {
	switch t {
	case FieldTypeString, FieldTypeURL, FieldTypeDate, FieldTypeDatetime, FieldTypeEnum, FieldTypeRef:
		return true
	default:
		return false
	}
}

func (t FieldType) scalarType() FieldType {
	if elem, ok := t.ElementType(); ok {
		return elem
	}
	return t
}

// declaredFieldTypes is the canonical catalog of schema field types. Scalars
// are paired with their array forms. Order matches ValidFieldTypes().
var declaredFieldTypes = []FieldType{
	FieldTypeString, FieldTypeStringArray,
	FieldTypeNumber, FieldTypeNumberArray,
	FieldTypeURL, FieldTypeURLArray,
	FieldTypeDate, FieldTypeDateArray,
	FieldTypeDatetime, FieldTypeDatetimeArray,
	FieldTypeEnum, FieldTypeEnumArray,
	FieldTypeBool, FieldTypeBoolArray,
	FieldTypeRef, FieldTypeRefArray,
}

// IsScalarFieldType reports whether t is a declared scalar FieldType.
func IsScalarFieldType(t FieldType) bool {
	return IsValidFieldType(t) && !t.IsArray()
}

// ScalarFieldTypes returns declared scalar field types in canonical order.
func ScalarFieldTypes() []FieldType {
	out := make([]FieldType, 0, len(declaredFieldTypes)/2)
	for _, t := range declaredFieldTypes {
		if !t.IsArray() {
			out = append(out, t)
		}
	}
	return out
}

// IsValidFieldType reports whether fieldType is a declared schema field type.
func IsValidFieldType(fieldType FieldType) bool {
	for _, t := range declaredFieldTypes {
		if t == fieldType {
			return true
		}
	}
	return false
}

// ValidFieldTypes returns the comma-separated catalog used in user-facing
// error messages.
func ValidFieldTypes() string {
	parts := make([]string, len(declaredFieldTypes))
	for i, t := range declaredFieldTypes {
		parts[i] = string(t)
	}
	return strings.Join(parts, ", ")
}
