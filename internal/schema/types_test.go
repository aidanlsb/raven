package schema

import (
	"encoding/json"
	"testing"
)

func TestNewSchema(t *testing.T) {
	s := NewSchema()

	if s.Types == nil {
		t.Fatal("Types should not be nil")
	}

	if s.Traits == nil {
		t.Fatal("Traits should not be nil")
	}

	// Check built-in types
	if _, ok := s.Types["page"]; !ok {
		t.Error("expected 'page' type to exist")
	}

	if _, ok := s.Types["section"]; !ok {
		t.Error("expected 'section' type to exist")
	}

	// Section should have title and level fields
	section := s.Types["section"]
	if _, ok := section.Fields["title"]; !ok {
		t.Error("section should have 'title' field")
	}
	if _, ok := section.Fields["level"]; !ok {
		t.Error("section should have 'level' field")
	}
}

func TestTraitDefinitionIsBoolean(t *testing.T) {
	tests := []struct {
		name     string
		traitDef TraitDefinition
		want     bool
	}{
		{"empty type", TraitDefinition{Type: ""}, true},
		{"bool type", TraitDefinition{Type: FieldTypeBool}, true},
		{"boolean type", TraitDefinition{Type: "boolean"}, true},
		{"date type", TraitDefinition{Type: FieldTypeDate}, false},
		{"string type", TraitDefinition{Type: FieldTypeString}, false},
		{"enum type", TraitDefinition{Type: FieldTypeEnum}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.traitDef.IsBoolean(); got != tt.want {
				t.Errorf("IsBoolean() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldValueString(t *testing.T) {
	t.Run("String value", func(t *testing.T) {
		fv := String("hello")
		s, ok := fv.AsString()
		if !ok {
			t.Error("expected AsString to succeed")
		}
		if s != "hello" {
			t.Errorf("expected 'hello', got %q", s)
		}
	})

	t.Run("Date value as string", func(t *testing.T) {
		fv := Date("2025-01-01")
		s, ok := fv.AsString()
		if !ok {
			t.Error("expected AsString to succeed for date")
		}
		if s != "2025-01-01" {
			t.Errorf("expected '2025-01-01', got %q", s)
		}
	})

	t.Run("Datetime value as string", func(t *testing.T) {
		fv := Datetime("2025-01-01T10:00")
		s, ok := fv.AsString()
		if !ok {
			t.Error("expected AsString to succeed for datetime")
		}
		if s != "2025-01-01T10:00" {
			t.Errorf("expected '2025-01-01T10:00', got %q", s)
		}
	})

	t.Run("Ref value as string", func(t *testing.T) {
		fv := Ref("people/freya")
		s, ok := fv.AsString()
		if !ok {
			t.Error("expected AsString to succeed for ref")
		}
		if s != "people/freya" {
			t.Errorf("expected 'people/freya', got %q", s)
		}
	})

	t.Run("Number not a string", func(t *testing.T) {
		fv := Number(42)
		_, ok := fv.AsString()
		if ok {
			t.Error("expected AsString to fail for number")
		}
	})
}

func TestFieldValueNumber(t *testing.T) {
	fv := Number(42.5)
	n, ok := fv.AsNumber()
	if !ok {
		t.Error("expected AsNumber to succeed")
	}
	if n != 42.5 {
		t.Errorf("expected 42.5, got %v", n)
	}

	// String is not a number
	fv2 := String("42")
	_, ok = fv2.AsNumber()
	if ok {
		t.Error("expected AsNumber to fail for string")
	}
}

func TestFieldValueBool(t *testing.T) {
	fv := Bool(true)
	b, ok := fv.AsBool()
	if !ok {
		t.Error("expected AsBool to succeed")
	}
	if !b {
		t.Error("expected true")
	}

	fv2 := Bool(false)
	b, ok = fv2.AsBool()
	if !ok {
		t.Error("expected AsBool to succeed")
	}
	if b {
		t.Error("expected false")
	}

	// String is not a bool
	fv3 := String("true")
	_, ok = fv3.AsBool()
	if ok {
		t.Error("expected AsBool to fail for string")
	}
}

func TestFieldValueArray(t *testing.T) {
	items := []FieldValue{String("a"), String("b"), String("c")}
	fv := Array(items)

	arr, ok := fv.AsArray()
	if !ok {
		t.Error("expected AsArray to succeed")
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 items, got %d", len(arr))
	}

	// Non-array
	fv2 := String("not an array")
	_, ok = fv2.AsArray()
	if ok {
		t.Error("expected AsArray to fail for string")
	}
}

func TestFieldValueRef(t *testing.T) {
	fv := Ref("people/freya")

	r, ok := fv.AsRef()
	if !ok {
		t.Error("expected AsRef to succeed")
	}
	if r != "people/freya" {
		t.Errorf("expected 'people/freya', got %q", r)
	}

	if !fv.IsRef() {
		t.Error("expected IsRef to be true")
	}

	// String is not a ref
	fv2 := String("people/freya")
	_, ok = fv2.AsRef()
	if ok {
		t.Error("expected AsRef to fail for string")
	}
	if fv2.IsRef() {
		t.Error("expected IsRef to be false for string")
	}
}

func TestFieldValueNull(t *testing.T) {
	fv := Null()
	if !fv.IsNull() {
		t.Error("expected IsNull to be true")
	}

	fv2 := String("not null")
	if fv2.IsNull() {
		t.Error("expected IsNull to be false")
	}
}

func TestFieldValueRaw(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		fv := String("hello")
		if fv.Raw() != "hello" {
			t.Error("Raw() should return underlying value")
		}
	})

	t.Run("date", func(t *testing.T) {
		fv := Date("2025-01-01")
		if fv.Raw() != "2025-01-01" {
			t.Error("Raw() should return date string")
		}
	})

	t.Run("datetime", func(t *testing.T) {
		fv := Datetime("2025-01-01T10:00")
		if fv.Raw() != "2025-01-01T10:00" {
			t.Error("Raw() should return datetime string")
		}
	})

	t.Run("ref", func(t *testing.T) {
		fv := Ref("people/freya")
		if fv.Raw() != "people/freya" {
			t.Error("Raw() should return ref string")
		}
	})

	t.Run("array", func(t *testing.T) {
		fv := Array([]FieldValue{String("a"), String("b")})
		raw := fv.Raw().([]interface{})
		if len(raw) != 2 {
			t.Errorf("expected 2 items, got %d", len(raw))
		}
		if raw[0] != "a" || raw[1] != "b" {
			t.Error("array items should be unwrapped")
		}
	})

	t.Run("number", func(t *testing.T) {
		fv := Number(42)
		if fv.Raw() != float64(42) {
			t.Error("Raw() should return number")
		}
	})

	t.Run("bool", func(t *testing.T) {
		fv := Bool(true)
		if fv.Raw() != true {
			t.Error("Raw() should return bool")
		}
	})
}

func TestFieldValueMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		fv   FieldValue
		want string
	}{
		{"string", String("hello"), `"hello"`},
		{"number", Number(42), `42`},
		{"bool true", Bool(true), `true`},
		{"bool false", Bool(false), `false`},
		{"null", Null(), `null`},
		{"date", Date("2025-01-01"), `"2025-01-01"`},
		{"ref", Ref("people/freya"), `"people/freya"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.fv)
			if err != nil {
				t.Fatalf("MarshalJSON failed: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("got %s, want %s", string(data), tt.want)
			}
		})
	}

	t.Run("array", func(t *testing.T) {
		fv := Array([]FieldValue{String("a"), String("b")})
		data, err := json.Marshal(fv)
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if string(data) != `["a","b"]` {
			t.Errorf("got %s, want %s", string(data), `["a","b"]`)
		}
	})
}

func TestNewFieldValue(t *testing.T) {
	fv := NewFieldValue("test")
	s, ok := fv.AsString()
	if !ok {
		t.Error("expected AsString to succeed")
	}
	if s != "test" {
		t.Errorf("expected 'test', got %q", s)
	}
}
