package schema

import (
	"testing"
)

func TestValidationError(t *testing.T) {
	err := ValidationError{Field: "name", Message: "is required"}
	expected := "Field 'name': is required"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestValidateFields(t *testing.T) {
	t.Run("required field missing", func(t *testing.T) {
		fields := map[string]FieldValue{}
		defs := map[string]*FieldDefinition{
			"name": {Type: FieldTypeString, Required: true},
		}

		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errors))
		}
		if errors[0].Field != "name" {
			t.Errorf("expected error on 'name' field")
		}
	})

	t.Run("required field with default does not error", func(t *testing.T) {
		fields := map[string]FieldValue{}
		defs := map[string]*FieldDefinition{
			"status": {Type: FieldTypeString, Required: true, Default: "active"},
		}

		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Fatalf("expected 0 errors, got %d: %v", len(errors), errors)
		}
	})

	t.Run("required field present", func(t *testing.T) {
		fields := map[string]FieldValue{
			"name": String("Freya"),
		}
		defs := map[string]*FieldDefinition{
			"name": {Type: FieldTypeString, Required: true},
		}

		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Fatalf("expected 0 errors, got %d: %v", len(errors), errors)
		}
	})

	t.Run("null required field", func(t *testing.T) {
		fields := map[string]FieldValue{
			"name": Null(),
		}
		defs := map[string]*FieldDefinition{
			"name": {Type: FieldTypeString, Required: true},
		}

		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("reserved fields skipped", func(t *testing.T) {
		fields := map[string]FieldValue{
			"id":   String("test-id"),
			"type": String("person"),
			"tags": Array([]FieldValue{String("a")}),
		}
		defs := map[string]*FieldDefinition{}

		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Fatalf("expected 0 errors for reserved fields, got %d: %v", len(errors), errors)
		}
	})

	t.Run("unknown fields allowed", func(t *testing.T) {
		fields := map[string]FieldValue{
			"unknown": String("value"),
		}
		defs := map[string]*FieldDefinition{}

		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Fatalf("expected 0 errors for unknown fields, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueString(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"name": {Type: FieldTypeString},
	}

	t.Run("valid string", func(t *testing.T) {
		fields := map[string]FieldValue{"name": String("Freya")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid string (number)", func(t *testing.T) {
		fields := map[string]FieldValue{"name": Number(42)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueStringArray(t *testing.T) {
	// Note: "tags" is a reserved field, so use "labels" instead
	defs := map[string]*FieldDefinition{
		"labels": {Type: FieldTypeStringArray},
	}

	t.Run("valid string array", func(t *testing.T) {
		fields := map[string]FieldValue{
			"labels": Array([]FieldValue{String("a"), String("b")}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid - not an array", func(t *testing.T) {
		fields := map[string]FieldValue{"labels": String("not-array")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("invalid - array with non-string", func(t *testing.T) {
		fields := map[string]FieldValue{
			"labels": Array([]FieldValue{String("a"), Number(42)}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueNumber(t *testing.T) {
	t.Run("valid number", func(t *testing.T) {
		defs := map[string]*FieldDefinition{
			"age": {Type: FieldTypeNumber},
		}
		fields := map[string]FieldValue{"age": Number(42)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid number (string)", func(t *testing.T) {
		defs := map[string]*FieldDefinition{
			"age": {Type: FieldTypeNumber},
		}
		fields := map[string]FieldValue{"age": String("42")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("number below min", func(t *testing.T) {
		min := float64(0)
		defs := map[string]*FieldDefinition{
			"level": {Type: FieldTypeNumber, Min: &min},
		}
		fields := map[string]FieldValue{"level": Number(-1)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error for below min, got %d", len(errors))
		}
	})

	t.Run("number above max", func(t *testing.T) {
		max := float64(10)
		defs := map[string]*FieldDefinition{
			"level": {Type: FieldTypeNumber, Max: &max},
		}
		fields := map[string]FieldValue{"level": Number(11)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error for above max, got %d", len(errors))
		}
	})

	t.Run("number within range", func(t *testing.T) {
		min := float64(1)
		max := float64(10)
		defs := map[string]*FieldDefinition{
			"level": {Type: FieldTypeNumber, Min: &min, Max: &max},
		}
		fields := map[string]FieldValue{"level": Number(5)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})
}

func TestValidateFieldValueNumberArray(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"scores": {Type: FieldTypeNumberArray},
	}

	t.Run("valid number array", func(t *testing.T) {
		fields := map[string]FieldValue{
			"scores": Array([]FieldValue{Number(1), Number(2), Number(3)}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid - not an array", func(t *testing.T) {
		fields := map[string]FieldValue{"scores": Number(42)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("invalid - array with non-number", func(t *testing.T) {
		fields := map[string]FieldValue{
			"scores": Array([]FieldValue{Number(1), String("two")}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueDate(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"due": {Type: FieldTypeDate},
	}

	t.Run("valid date", func(t *testing.T) {
		fields := map[string]FieldValue{"due": Date("2025-01-15")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid date format", func(t *testing.T) {
		fields := map[string]FieldValue{"due": String("01-15-2025")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("invalid date - not a date", func(t *testing.T) {
		fields := map[string]FieldValue{"due": Number(42)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("invalid date - impossible date", func(t *testing.T) {
		fields := map[string]FieldValue{"due": String("2025-13-45")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error for invalid date, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueDateArray(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"dates": {Type: FieldTypeDateArray},
	}

	t.Run("valid date array", func(t *testing.T) {
		fields := map[string]FieldValue{
			"dates": Array([]FieldValue{Date("2025-01-01"), Date("2025-01-02")}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid - not an array", func(t *testing.T) {
		fields := map[string]FieldValue{"dates": Date("2025-01-01")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("invalid - array with invalid date", func(t *testing.T) {
		fields := map[string]FieldValue{
			"dates": Array([]FieldValue{Date("2025-01-01"), String("not-a-date")}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueDatetime(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"time": {Type: FieldTypeDatetime},
	}

	t.Run("valid datetime RFC3339", func(t *testing.T) {
		fields := map[string]FieldValue{"time": Datetime("2025-01-15T10:30:00Z")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("valid datetime short format", func(t *testing.T) {
		fields := map[string]FieldValue{"time": Datetime("2025-01-15T10:30")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("valid datetime with seconds", func(t *testing.T) {
		fields := map[string]FieldValue{"time": Datetime("2025-01-15T10:30:45")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid datetime", func(t *testing.T) {
		fields := map[string]FieldValue{"time": String("not-a-datetime")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueEnum(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"status": {Type: FieldTypeEnum, Values: []string{"active", "paused", "done"}},
	}

	t.Run("valid enum", func(t *testing.T) {
		fields := map[string]FieldValue{"status": String("active")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid enum value", func(t *testing.T) {
		fields := map[string]FieldValue{"status": String("invalid")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("enum missing values definition", func(t *testing.T) {
		defsNoValues := map[string]*FieldDefinition{
			"status": {Type: FieldTypeEnum}, // No Values
		}
		fields := map[string]FieldValue{"status": String("active")}
		errors := ValidateFields(fields, defsNoValues, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error for missing values, got %d", len(errors))
		}
	})

	t.Run("enum wrong type", func(t *testing.T) {
		fields := map[string]FieldValue{"status": Number(1)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueBool(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"active": {Type: FieldTypeBool},
	}

	t.Run("valid bool true", func(t *testing.T) {
		fields := map[string]FieldValue{"active": Bool(true)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("valid bool false", func(t *testing.T) {
		fields := map[string]FieldValue{"active": Bool(false)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid bool (string)", func(t *testing.T) {
		fields := map[string]FieldValue{"active": String("true")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueRef(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"person": {Type: FieldTypeRef, Target: "person"},
	}

	t.Run("valid ref", func(t *testing.T) {
		fields := map[string]FieldValue{"person": Ref("people/freya")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("string also valid as ref", func(t *testing.T) {
		fields := map[string]FieldValue{"person": String("people/freya")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors for string as ref, got %v", errors)
		}
	})

	t.Run("invalid ref (number)", func(t *testing.T) {
		fields := map[string]FieldValue{"person": Number(42)}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateFieldValueRefArray(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"attendees": {Type: FieldTypeRefArray, Target: "person"},
	}

	t.Run("valid ref array", func(t *testing.T) {
		fields := map[string]FieldValue{
			"attendees": Array([]FieldValue{Ref("people/freya"), Ref("people/thor")}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("string array also valid as ref array", func(t *testing.T) {
		fields := map[string]FieldValue{
			"attendees": Array([]FieldValue{String("people/freya"), String("people/thor")}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})

	t.Run("invalid - not an array", func(t *testing.T) {
		fields := map[string]FieldValue{"attendees": Ref("people/freya")}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})

	t.Run("invalid - array with non-ref", func(t *testing.T) {
		fields := map[string]FieldValue{
			"attendees": Array([]FieldValue{Ref("people/freya"), Number(42)}),
		}
		errors := ValidateFields(fields, defs, nil)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestValidateNullValue(t *testing.T) {
	defs := map[string]*FieldDefinition{
		"optional": {Type: FieldTypeString}, // Not required
	}

	fields := map[string]FieldValue{"optional": Null()}
	errors := ValidateFields(fields, defs, nil)
	if len(errors) != 0 {
		t.Errorf("null value should be valid for optional field, got %v", errors)
	}
}

func TestIsValidDate(t *testing.T) {
	valid := []string{"2025-01-01", "2024-12-31", "2000-06-15"}
	for _, d := range valid {
		if !isValidDate(d) {
			t.Errorf("expected %q to be valid", d)
		}
	}

	invalid := []string{"2025/01/01", "01-01-2025", "2025-13-01", "2025-01-32", "not-a-date", ""}
	for _, d := range invalid {
		if isValidDate(d) {
			t.Errorf("expected %q to be invalid", d)
		}
	}
}

func TestIsValidDatetime(t *testing.T) {
	valid := []string{
		"2025-01-01T10:30:00Z",
		"2025-01-01T10:30",
		"2025-01-01T10:30:45",
		"2025-06-15T14:00:00+05:00",
	}
	for _, dt := range valid {
		if !isValidDatetime(dt) {
			t.Errorf("expected %q to be valid", dt)
		}
	}

	invalid := []string{"2025-01-01", "10:30", "not-a-datetime", ""}
	for _, dt := range invalid {
		if isValidDatetime(dt) {
			t.Errorf("expected %q to be invalid", dt)
		}
	}
}

func TestValidateNameField(t *testing.T) {
	t.Run("no name_field is valid", func(t *testing.T) {
		typeDef := &TypeDefinition{
			Fields: map[string]*FieldDefinition{
				"title": {Type: FieldTypeString, Required: true},
			},
		}
		err := ValidateNameField(typeDef)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("name_field referencing valid string field", func(t *testing.T) {
		typeDef := &TypeDefinition{
			NameField: "title",
			Fields: map[string]*FieldDefinition{
				"title": {Type: FieldTypeString, Required: true},
			},
		}
		err := ValidateNameField(typeDef)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("name_field referencing non-existent field", func(t *testing.T) {
		typeDef := &TypeDefinition{
			NameField: "foo",
			Fields: map[string]*FieldDefinition{
				"title": {Type: FieldTypeString, Required: true},
			},
		}
		err := ValidateNameField(typeDef)
		if err == nil {
			t.Error("expected error for non-existent field")
		}
	})

	t.Run("name_field referencing non-string field", func(t *testing.T) {
		typeDef := &TypeDefinition{
			NameField: "count",
			Fields: map[string]*FieldDefinition{
				"count": {Type: FieldTypeNumber, Required: true},
			},
		}
		err := ValidateNameField(typeDef)
		if err == nil {
			t.Error("expected error for non-string field")
		}
	})
}

func TestValidateSchema(t *testing.T) {
	t.Run("valid schema with name_field", func(t *testing.T) {
		sch := &Schema{
			Types: map[string]*TypeDefinition{
				"book": {
					NameField: "title",
					Fields: map[string]*FieldDefinition{
						"title": {Type: FieldTypeString, Required: true},
					},
				},
			},
		}
		issues := ValidateSchema(sch)
		if len(issues) != 0 {
			t.Errorf("expected no issues, got %v", issues)
		}
	})

	t.Run("invalid name_field in schema", func(t *testing.T) {
		sch := &Schema{
			Types: map[string]*TypeDefinition{
				"book": {
					NameField: "nonexistent",
					Fields: map[string]*FieldDefinition{
						"title": {Type: FieldTypeString, Required: true},
					},
				},
			},
		}
		issues := ValidateSchema(sch)
		if len(issues) != 1 {
			t.Errorf("expected 1 issue, got %d", len(issues))
		}
	})
}
