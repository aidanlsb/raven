package schema

import "testing"

func TestFieldTypeFamilies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		typ          FieldType
		elem         FieldType
		hasElem      bool
		isArray      bool
		isRef        bool
		isEnum       bool
		isBool       bool
		isStringLike bool
		isValid      bool
	}{
		{"string", FieldTypeString, "", false, false, false, false, false, true, true},
		{"string[]", FieldTypeStringArray, FieldTypeString, true, true, false, false, false, false, true},
		{"number", FieldTypeNumber, "", false, false, false, false, false, false, true},
		{"number[]", FieldTypeNumberArray, FieldTypeNumber, true, true, false, false, false, false, true},
		{"url", FieldTypeURL, "", false, false, false, false, false, true, true},
		{"url[]", FieldTypeURLArray, FieldTypeURL, true, true, false, false, false, false, true},
		{"date", FieldTypeDate, "", false, false, false, false, false, true, true},
		{"date[]", FieldTypeDateArray, FieldTypeDate, true, true, false, false, false, false, true},
		{"datetime", FieldTypeDatetime, "", false, false, false, false, false, true, true},
		{"datetime[]", FieldTypeDatetimeArray, FieldTypeDatetime, true, true, false, false, false, false, true},
		{"enum", FieldTypeEnum, "", false, false, false, true, false, true, true},
		{"enum[]", FieldTypeEnumArray, FieldTypeEnum, true, true, false, true, false, false, true},
		{"bool", FieldTypeBool, "", false, false, false, false, true, false, true},
		{"bool[]", FieldTypeBoolArray, FieldTypeBool, true, true, false, false, true, false, true},
		{"ref", FieldTypeRef, "", false, false, true, false, false, true, true},
		{"ref[]", FieldTypeRefArray, FieldTypeRef, true, true, true, false, false, false, true},
		{"empty", FieldType(""), "", false, false, false, false, false, false, false},
		{"unknown", FieldType("unknown"), "", false, false, false, false, false, false, false},
		{"unknown array suffix", FieldType("unknown[]"), "", false, false, false, false, false, false, false},
		{"boolean alias", FieldType("boolean"), "", false, false, false, false, false, false, false},
		{"reference alias", FieldType("reference"), "", false, false, false, false, false, false, false},
		{"reference array alias", FieldType("reference[]"), "", false, false, false, false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem, ok := tt.typ.ElementType()
			if ok != tt.hasElem {
				t.Errorf("ElementType() ok = %v, want %v", ok, tt.hasElem)
			}
			if elem != tt.elem {
				t.Errorf("ElementType() = %q, want %q", elem, tt.elem)
			}
			if got := tt.typ.IsArray(); got != tt.isArray {
				t.Errorf("IsArray() = %v, want %v", got, tt.isArray)
			}
			if got := tt.typ.IsRef(); got != tt.isRef {
				t.Errorf("IsRef() = %v, want %v", got, tt.isRef)
			}
			if got := tt.typ.IsEnum(); got != tt.isEnum {
				t.Errorf("IsEnum() = %v, want %v", got, tt.isEnum)
			}
			if got := tt.typ.IsBool(); got != tt.isBool {
				t.Errorf("IsBool() = %v, want %v", got, tt.isBool)
			}
			if got := tt.typ.IsStringLike(); got != tt.isStringLike {
				t.Errorf("IsStringLike() = %v, want %v", got, tt.isStringLike)
			}
			if got := IsValidFieldType(tt.typ); got != tt.isValid {
				t.Errorf("IsValidFieldType() = %v, want %v", got, tt.isValid)
			}
		})
	}
}

func TestDeclaredFieldTypesHaveMatchingArrayForms(t *testing.T) {
	t.Parallel()

	declared := []FieldType{
		FieldTypeString, FieldTypeStringArray,
		FieldTypeNumber, FieldTypeNumberArray,
		FieldTypeURL, FieldTypeURLArray,
		FieldTypeDate, FieldTypeDateArray,
		FieldTypeDatetime, FieldTypeDatetimeArray,
		FieldTypeEnum, FieldTypeEnumArray,
		FieldTypeBool, FieldTypeBoolArray,
		FieldTypeRef, FieldTypeRefArray,
	}

	scalars := make(map[FieldType]struct{})
	arrays := make(map[FieldType]FieldType)
	for _, ft := range declared {
		if !IsValidFieldType(ft) {
			t.Errorf("declared type %q is not valid", ft)
		}
		if ft.IsArray() {
			elem, ok := ft.ElementType()
			if !ok {
				t.Errorf("%q.IsArray() is true but ElementType() failed", ft)
				continue
			}
			if elem.IsArray() {
				t.Errorf("element type of %q is %q, which is also an array", ft, elem)
			}
			arrays[ft] = elem
			continue
		}
		if elem, ok := ft.ElementType(); ok {
			t.Errorf("scalar %q unexpectedly has element type %q", ft, elem)
		}
		scalars[ft] = struct{}{}
	}

	if len(scalars) != len(arrays) {
		t.Errorf("scalar count %d != array count %d", len(scalars), len(arrays))
	}
	for arrayType, elem := range arrays {
		if _, ok := scalars[elem]; !ok {
			t.Errorf("array type %q maps to %q, which is not a declared scalar", arrayType, elem)
		}
	}
}
