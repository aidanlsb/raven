package schemasvc

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestUpdateField_RejectsInvalidFieldSpecs(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()

	_, err := UpdateField(schemaTestRuntime(t, vault.Path), UpdateFieldRequest{
		VaultPath: vault.Path,
		TypeName:  "project",
		FieldName: "title",
		Target:    "person",
	})
	if err == nil {
		t.Fatal("expected update field to reject target on a string field")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected schemasvc error, got %T: %v", err, err)
	}
	if svcErr.Code != codes.ErrInvalidInput {
		t.Fatalf("error code = %q, want %q", svcErr.Code, codes.ErrInvalidInput)
	}
	if !strings.Contains(svcErr.Message, "--target is only valid for ref fields") {
		t.Fatalf("unexpected error message: %q", svcErr.Message)
	}
}

func TestUpdateField_UpdatesNonTypeMetadata(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()

	_, err := UpdateField(schemaTestRuntime(t, vault.Path), UpdateFieldRequest{
		VaultPath:   vault.Path,
		TypeName:    "project",
		FieldName:   "status",
		Default:     "active",
		Description: "Current project state",
	})
	if err != nil {
		t.Fatalf("UpdateField metadata returned error: %v", err)
	}

	loaded, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	status := loaded.Types["project"].Fields["status"]
	wantValues := []string{"active", "paused", "done"}
	if len(status.Values) != len(wantValues) {
		t.Fatalf("status values = %v, want %v", status.Values, wantValues)
	}
	for i := range wantValues {
		if status.Values[i] != wantValues[i] {
			t.Fatalf("status values = %v, want %v", status.Values, wantValues)
		}
	}
	if status.Default != "active" {
		t.Fatalf("status default = %#v, want active", status.Default)
	}
	if status.Description != "Current project state" {
		t.Fatalf("status description = %q, want %q", status.Description, "Current project state")
	}
	if status.Type != schema.FieldTypeEnum {
		t.Fatalf("status type = %q, want %q", status.Type, schema.FieldTypeEnum)
	}
}

func TestUpdateField_RejectsTypeAndValueRemaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request UpdateFieldRequest
		flag    string
	}{
		{
			name: "type",
			request: UpdateFieldRequest{
				TypeName:  "project",
				FieldName: "status",
				FieldType: "string",
			},
			flag: "--type",
		},
		{
			name: "values",
			request: UpdateFieldRequest{
				TypeName:  "project",
				FieldName: "status",
				Values:    "active,done",
			},
			flag: "--values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
			before := vault.ReadFile("schema.yaml")
			tt.request.VaultPath = vault.Path

			_, err := UpdateField(schemaTestRuntime(t, vault.Path), tt.request)
			assertUpdateRemapRejected(t, err, tt.flag, "schema convert field")
			if got := vault.ReadFile("schema.yaml"); got != before {
				t.Fatalf("rejected update changed schema.yaml:\n%s", got)
			}
		})
	}
}

func TestUpdateTrait_NormalizesDefaults(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(`version: 1
types: {}
traits:
  done:
    type: boolean
  priority:
    type: enum
    values: [low]
`).Build()

	_, err := UpdateTrait(schemaTestRuntime(t, vault.Path), UpdateTraitRequest{
		VaultPath: vault.Path,
		TraitName: "priority",
		Default:   "low",
	})
	if err != nil {
		t.Fatalf("UpdateTrait enum default returned error: %v", err)
	}
	_, err = UpdateTrait(schemaTestRuntime(t, vault.Path), UpdateTraitRequest{
		VaultPath: vault.Path,
		TraitName: "done",
		Default:   "true",
	})
	if err != nil {
		t.Fatalf("UpdateTrait default returned error: %v", err)
	}

	loaded, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	priority := loaded.Traits["priority"]
	wantValues := []string{"low"}
	if len(priority.Values) != len(wantValues) {
		t.Fatalf("priority values = %v, want %v", priority.Values, wantValues)
	}
	for i := range wantValues {
		if priority.Values[i] != wantValues[i] {
			t.Fatalf("priority values = %v, want %v", priority.Values, wantValues)
		}
	}
	if priority.Default != "low" {
		t.Fatalf("priority default = %#v, want low", priority.Default)
	}
	if got, ok := loaded.Traits["done"].Default.(bool); !ok || !got {
		t.Fatalf("done default = %#v, want bool(true)", loaded.Traits["done"].Default)
	}
}

func TestUpdateTrait_RejectsTypeAndValueRemaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request UpdateTraitRequest
		flag    string
	}{
		{
			name:    "type",
			request: UpdateTraitRequest{TraitName: "priority", TraitType: "bool"},
			flag:    "--type",
		},
		{
			name:    "values",
			request: UpdateTraitRequest{TraitName: "priority", Values: "low,high"},
			flag:    "--values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
			before := vault.ReadFile("schema.yaml")
			tt.request.VaultPath = vault.Path

			_, err := UpdateTrait(schemaTestRuntime(t, vault.Path), tt.request)
			assertUpdateRemapRejected(t, err, tt.flag, "schema convert trait")
			if got := vault.ReadFile("schema.yaml"); got != before {
				t.Fatalf("rejected update changed schema.yaml:\n%s", got)
			}
		})
	}
}

func assertUpdateRemapRejected(t *testing.T, err error, flag, convertCommand string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected update to reject %s", flag)
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected schemasvc error, got %T: %v", err, err)
	}
	if svcErr.Code != ErrorInvalidInput {
		t.Fatalf("error code = %q, want %q", svcErr.Code, ErrorInvalidInput)
	}
	if !strings.Contains(svcErr.Message, flag) {
		t.Fatalf("error message %q does not mention %s", svcErr.Message, flag)
	}
	if !strings.Contains(svcErr.Suggestion, convertCommand) {
		t.Fatalf("suggestion %q does not mention %q", svcErr.Suggestion, convertCommand)
	}
}
