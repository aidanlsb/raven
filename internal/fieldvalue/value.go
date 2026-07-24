// Package fieldvalue defines Raven's canonical typed field value.
package fieldvalue

import (
	"encoding/json"
	"strconv"
	"strings"
)

// FieldValue represents a parsed field value.
type FieldValue struct {
	value interface{}
}

// NewFieldValue creates a new FieldValue.
func NewFieldValue(v interface{}) FieldValue {
	return FieldValue{value: v}
}

// String creates a string FieldValue.
func String(s string) FieldValue {
	return FieldValue{value: s}
}

// Number creates a number FieldValue.
func Number(n float64) FieldValue {
	return FieldValue{value: n}
}

// Bool creates a boolean FieldValue.
func Bool(b bool) FieldValue {
	return FieldValue{value: b}
}

// Date creates a date FieldValue.
func Date(s string) FieldValue {
	return FieldValue{value: dateValue{s}}
}

// Datetime creates a datetime FieldValue.
func Datetime(s string) FieldValue {
	return FieldValue{value: datetimeValue{s}}
}

// Ref creates a reference FieldValue.
func Ref(s string) FieldValue {
	return FieldValue{value: refValue{s}}
}

// Array creates an array FieldValue.
func Array(items []FieldValue) FieldValue {
	return FieldValue{value: items}
}

// Null creates a null FieldValue.
func Null() FieldValue {
	return FieldValue{value: nil}
}

// Internal types distinguish string-based values.
type dateValue struct{ s string }
type datetimeValue struct{ s string }
type refValue struct{ s string }

// IsNull returns true if the value is null.
func (fv FieldValue) IsNull() bool {
	return fv.value == nil
}

// AsString returns the value as a string, if possible.
func (fv FieldValue) AsString() (string, bool) {
	switch v := fv.value.(type) {
	case string:
		return v, true
	case dateValue:
		return v.s, true
	case datetimeValue:
		return v.s, true
	case refValue:
		return v.s, true
	}
	return "", false
}

// AsNumber returns the value as a number, if possible.
func (fv FieldValue) AsNumber() (float64, bool) {
	if n, ok := fv.value.(float64); ok {
		return n, true
	}
	return 0, false
}

// AsBool returns the value as a boolean, if possible.
func (fv FieldValue) AsBool() (bool, bool) {
	if b, ok := fv.value.(bool); ok {
		return b, true
	}
	return false, false
}

// AsArray returns the value as an array, if possible.
func (fv FieldValue) AsArray() ([]FieldValue, bool) {
	if arr, ok := fv.value.([]FieldValue); ok {
		return arr, true
	}
	return nil, false
}

// AsRef returns the value as a reference path, if possible.
func (fv FieldValue) AsRef() (string, bool) {
	if r, ok := fv.value.(refValue); ok {
		return r.s, true
	}
	return "", false
}

// IsRef returns true if this is a reference value.
func (fv FieldValue) IsRef() bool {
	_, ok := fv.value.(refValue)
	return ok
}

// IsDate returns true if this is a date value (YYYY-MM-DD).
func (fv FieldValue) IsDate() bool {
	_, ok := fv.value.(dateValue)
	return ok
}

// IsDatetime returns true if this is a datetime value.
func (fv FieldValue) IsDatetime() bool {
	_, ok := fv.value.(datetimeValue)
	return ok
}

// Raw returns the underlying raw value.
func (fv FieldValue) Raw() interface{} {
	switch v := fv.value.(type) {
	case dateValue:
		return v.s
	case datetimeValue:
		return v.s
	case refValue:
		return v.s
	case []FieldValue:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = item.Raw()
		}
		return result
	default:
		return v
	}
}

// MarshalJSON implements json.Marshaler.
func (fv FieldValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(fv.Raw())
}

// FormatLiteral renders a FieldValue using Raven's inline literal syntax.
func FormatLiteral(value FieldValue) string {
	if value.IsNull() {
		return ""
	}
	if arr, ok := value.AsArray(); ok {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			parts = append(parts, FormatLiteral(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	if s, ok := value.AsString(); ok {
		return s
	}
	if n, ok := value.AsNumber(); ok {
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	if b, ok := value.AsBool(); ok {
		return strconv.FormatBool(b)
	}
	if raw := value.Raw(); raw != nil {
		if b, err := json.Marshal(raw); err == nil {
			return string(b)
		}
	}
	return ""
}

// TraitIndexString returns the index/wire string form used for trait values.
// Arrays are JSON-encoded; other values use FormatLiteral.
func TraitIndexString(value FieldValue) string {
	if value.IsNull() {
		return ""
	}
	if _, ok := value.AsArray(); ok {
		data, err := json.Marshal(value.Raw())
		if err == nil {
			return string(data)
		}
	}
	return FormatLiteral(value)
}

// FieldsFromJSON unmarshals a JSON object into typed field values.
func FieldsFromJSON(data []byte) (map[string]FieldValue, error) {
	if len(data) == 0 || string(data) == "null" {
		return map[string]FieldValue{}, nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	fields := make(map[string]FieldValue, len(raw))
	for key, value := range raw {
		fields[key] = FieldValueFromRaw(value)
	}
	return fields, nil
}

// FieldValueFromRaw converts a decoded JSON/YAML-ish value into a FieldValue.
// Strings are treated as plain strings (not wikilink refs).
func FieldValueFromRaw(value interface{}) FieldValue {
	switch v := value.(type) {
	case string:
		return String(v)
	case float64:
		return Number(v)
	case float32:
		return Number(float64(v))
	case int:
		return Number(float64(v))
	case int64:
		return Number(float64(v))
	case bool:
		return Bool(v)
	case []interface{}:
		items := make([]FieldValue, 0, len(v))
		for _, item := range v {
			items = append(items, FieldValueFromRaw(item))
		}
		return Array(items)
	case nil:
		return Null()
	default:
		return Null()
	}
}
