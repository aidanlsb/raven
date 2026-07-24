package fieldvalue

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestStringKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        FieldValue
		want         string
		wantRef      bool
		wantDate     bool
		wantDateTime bool
	}{
		{name: "string", value: String("hello"), want: "hello"},
		{name: "date", value: Date("2025-01-01"), want: "2025-01-01", wantDate: true},
		{name: "datetime", value: Datetime("2025-01-01T10:00"), want: "2025-01-01T10:00", wantDateTime: true},
		{name: "reference", value: Ref("people/freya"), want: "people/freya", wantRef: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.value.AsString()
			if !ok || got != tt.want {
				t.Fatalf("AsString() = %q, %v; want %q, true", got, ok, tt.want)
			}
			if tt.value.IsRef() != tt.wantRef {
				t.Errorf("IsRef() = %v, want %v", tt.value.IsRef(), tt.wantRef)
			}
			if tt.value.IsDate() != tt.wantDate {
				t.Errorf("IsDate() = %v, want %v", tt.value.IsDate(), tt.wantDate)
			}
			if tt.value.IsDatetime() != tt.wantDateTime {
				t.Errorf("IsDatetime() = %v, want %v", tt.value.IsDatetime(), tt.wantDateTime)
			}
		})
	}

	if _, ok := Number(42).AsString(); ok {
		t.Error("number must not convert to string")
	}
	if _, ok := String("people/freya").AsRef(); ok {
		t.Error("plain string must not convert to reference")
	}
}

func TestScalarAccessors(t *testing.T) {
	t.Parallel()

	number := Number(42.5)
	if got, ok := number.AsNumber(); !ok || got != 42.5 {
		t.Errorf("AsNumber() = %v, %v; want 42.5, true", got, ok)
	}
	if _, ok := String("42.5").AsNumber(); ok {
		t.Error("string must not convert to number")
	}

	for _, want := range []bool{true, false} {
		if got, ok := Bool(want).AsBool(); !ok || got != want {
			t.Errorf("AsBool() = %v, %v; want %v, true", got, ok, want)
		}
	}
	if _, ok := String("true").AsBool(); ok {
		t.Error("string must not convert to boolean")
	}
}

func TestArrayAndNull(t *testing.T) {
	t.Parallel()

	items := []FieldValue{String("a"), Number(2), Bool(true)}
	value := Array(items)
	got, ok := value.AsArray()
	if !ok || !reflect.DeepEqual(got, items) {
		t.Fatalf("AsArray() = %#v, %v; want %#v, true", got, ok, items)
	}
	if _, ok := String("not an array").AsArray(); ok {
		t.Error("string must not convert to array")
	}
	if !Null().IsNull() {
		t.Error("Null() must be null")
	}
	if String("").IsNull() {
		t.Error("empty string must not be null")
	}
}

func TestRaw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value FieldValue
		want  interface{}
	}{
		{name: "string", value: String("hello"), want: "hello"},
		{name: "date", value: Date("2025-01-01"), want: "2025-01-01"},
		{name: "datetime", value: Datetime("2025-01-01T10:00"), want: "2025-01-01T10:00"},
		{name: "reference", value: Ref("people/freya"), want: "people/freya"},
		{name: "number", value: Number(42), want: float64(42)},
		{name: "boolean", value: Bool(true), want: true},
		{name: "array", value: Array([]FieldValue{String("a"), Ref("people/freya")}), want: []interface{}{"a", "people/freya"}},
		{name: "null", value: Null(), want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.Raw(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Raw() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value FieldValue
		want  string
	}{
		{name: "string", value: String("hello"), want: `"hello"`},
		{name: "number", value: Number(42), want: `42`},
		{name: "boolean", value: Bool(true), want: `true`},
		{name: "null", value: Null(), want: `null`},
		{name: "date", value: Date("2025-01-01"), want: `"2025-01-01"`},
		{name: "datetime", value: Datetime("2025-01-01T10:00"), want: `"2025-01-01T10:00"`},
		{name: "reference", value: Ref("people/freya"), want: `"people/freya"`},
		{name: "array", value: Array([]FieldValue{String("a"), String("b")}), want: `["a","b"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     FieldValue
		want      string
		wantIndex string
	}{
		{name: "null", value: Null(), want: "", wantIndex: ""},
		{name: "string", value: String("hello"), want: "hello", wantIndex: "hello"},
		{name: "integer", value: Number(42), want: "42", wantIndex: "42"},
		{name: "decimal", value: Number(42.5), want: "42.5", wantIndex: "42.5"},
		{name: "boolean", value: Bool(false), want: "false", wantIndex: "false"},
		{name: "array", value: Array([]FieldValue{String("a"), Number(2)}), want: "[a, 2]", wantIndex: `["a",2]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatLiteral(tt.value); got != tt.want {
				t.Errorf("FormatLiteral() = %q, want %q", got, tt.want)
			}
			if got := TraitIndexString(tt.value); got != tt.wantIndex {
				t.Errorf("TraitIndexString() = %q, want %q", got, tt.wantIndex)
			}
		})
	}
}

func TestFieldsFromJSON(t *testing.T) {
	t.Parallel()

	fields, err := FieldsFromJSON([]byte(`{"name":"Freya","age":42,"active":true,"tags":["a",null]}`))
	if err != nil {
		t.Fatalf("FieldsFromJSON() error = %v", err)
	}
	want := map[string]interface{}{
		"name":   "Freya",
		"age":    float64(42),
		"active": true,
		"tags":   []interface{}{"a", nil},
	}
	got := make(map[string]interface{}, len(fields))
	for name, value := range fields {
		got[name] = value.Raw()
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FieldsFromJSON() = %#v, want %#v", got, want)
	}

	empty, err := FieldsFromJSON([]byte("null"))
	if err != nil || len(empty) != 0 {
		t.Errorf("FieldsFromJSON(null) = %#v, %v; want empty map, nil", empty, err)
	}
}

func TestNewFieldValue(t *testing.T) {
	t.Parallel()

	value := NewFieldValue("test")
	if got, ok := value.AsString(); !ok || got != "test" {
		t.Errorf("AsString() = %q, %v; want test, true", got, ok)
	}
}
