package fieldmutation

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestCoerceFieldValueForDefinitionAtDateInputs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeDate}

	got := coerceFieldValueForDefinitionAt(fieldvalue.String("today"), def, now)
	if raw, _ := got.AsString(); raw != "2026-04-05" || !got.IsDate() {
		t.Fatalf("coerced date = %q IsDate=%v, want 2026-04-05 date value", raw, got.IsDate())
	}
}

func TestCoerceFieldValueForDefinitionAtDateArrayInputs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeDateArray}

	got := coerceFieldValueForDefinitionAt(fieldvalue.Array([]fieldvalue.FieldValue{
		fieldvalue.String("yesterday"),
		fieldvalue.String("2026-04-05"),
		fieldvalue.String("not-a-date"),
	}), def, now)

	arr, ok := got.AsArray()
	if !ok || len(arr) != 3 {
		t.Fatalf("coerced array = %#v, ok=%v", got.Raw(), ok)
	}
	if raw, _ := arr[0].AsString(); raw != "2026-04-04" || !arr[0].IsDate() {
		t.Fatalf("first item = %q IsDate=%v, want 2026-04-04 date value", raw, arr[0].IsDate())
	}
	if raw, _ := arr[1].AsString(); raw != "2026-04-05" || !arr[1].IsDate() {
		t.Fatalf("second item = %q IsDate=%v, want 2026-04-05 date value", raw, arr[1].IsDate())
	}
	if raw, _ := arr[2].AsString(); raw != "not-a-date" || arr[2].IsDate() {
		t.Fatalf("third item = %q IsDate=%v, want unchanged string", raw, arr[2].IsDate())
	}
}

func TestCoerceFieldValueForDefinitionAtDateTargetRef(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeRef, Target: "date"}

	got := coerceFieldValueForDefinitionAt(fieldvalue.String("tomorrow"), def, now)
	if raw, _ := got.AsString(); raw != "2026-04-06" || got.IsDate() || got.IsRef() {
		t.Fatalf("coerced date ref = %q IsDate=%v IsRef=%v, want plain 2026-04-06 string", raw, got.IsDate(), got.IsRef())
	}
}

func TestCoerceFieldValueForDefinitionAtDateTargetRefArray(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	def := &schema.FieldDefinition{Type: schema.FieldTypeRefArray, Target: "date"}

	got := coerceFieldValueForDefinitionAt(fieldvalue.Array([]fieldvalue.FieldValue{
		fieldvalue.String("today"),
		fieldvalue.String("daily/2026-04-06"),
	}), def, now)

	arr, ok := got.AsArray()
	if !ok || len(arr) != 2 {
		t.Fatalf("coerced array = %#v, ok=%v", got.Raw(), ok)
	}
	if raw, _ := arr[0].AsString(); raw != "2026-04-05" || arr[0].IsDate() {
		t.Fatalf("first item = %q IsDate=%v, want plain 2026-04-05 string", raw, arr[0].IsDate())
	}
	if raw, _ := arr[1].AsString(); raw != "daily/2026-04-06" {
		t.Fatalf("second item = %q, want unchanged daily ID", raw)
	}
}

func TestPrepareValidatedFrontmatterMutationValues(t *testing.T) {
	t.Parallel()

	baseContent := "---\ntype: task\ntitle: Ship tests\nstatus: todo\n---\n# Body\n"
	tests := []struct {
		name           string
		content        string
		frontmatter    bool
		updates        map[string]fieldvalue.FieldValue
		allowedUnknown map[string]bool
		wantErr        error
		wantErrText    string
		verify         func(*testing.T, string)
	}{
		{
			name:        "updates and coerces schema fields",
			content:     baseContent,
			frontmatter: true,
			updates: map[string]fieldvalue.FieldValue{
				"status": fieldvalue.String("done"),
				"due":    fieldvalue.String("2026-07-24"),
			},
			verify: func(t *testing.T, content string) {
				t.Helper()
				if !strings.Contains(content, "# Body") {
					t.Fatalf("updated content lost body:\n%s", content)
				}
				fm, err := parser.ParseFrontmatter(content)
				if err != nil {
					t.Fatalf("parse updated frontmatter: %v", err)
				}
				if status, _ := fm.Fields["status"].AsString(); status != "done" {
					t.Fatalf("status = %q, want done", status)
				}
				if due, _ := fm.Fields["due"].AsString(); due != "2026-07-24" {
					t.Fatalf("due = %q, want normalized date", due)
				}
			},
		},
		{
			name:           "allows explicit extra field",
			content:        baseContent,
			frontmatter:    true,
			updates:        map[string]fieldvalue.FieldValue{"legacy": fieldvalue.String("kept")},
			allowedUnknown: map[string]bool{"legacy": true},
			verify: func(t *testing.T, content string) {
				t.Helper()
				if !strings.Contains(content, "legacy: kept") {
					t.Fatalf("allowed field missing from update:\n%s", content)
				}
			},
		},
		{
			name:        "rejects unknown update",
			content:     baseContent,
			frontmatter: true,
			updates: map[string]fieldvalue.FieldValue{
				"z_unknown": fieldvalue.String("z"),
				"a_unknown": fieldvalue.String("a"),
			},
			wantErr: &UnknownFieldMutationError{},
			verify: func(t *testing.T, _ string) {
				t.Helper()
			},
		},
		{
			name:        "rejects invalid schema value",
			content:     baseContent,
			frontmatter: true,
			updates:     map[string]fieldvalue.FieldValue{"status": fieldvalue.String("blocked")},
			wantErr:     &ValidationError{},
		},
		{
			name:        "validates required fields when parsed state is absent",
			content:     baseContent,
			frontmatter: false,
			updates:     map[string]fieldvalue.FieldValue{"status": fieldvalue.String("done")},
			wantErr:     &ValidationError{},
		},
		{
			name:        "rejects content without frontmatter",
			content:     "# Body\n",
			frontmatter: true,
			updates:     map[string]fieldvalue.FieldValue{"title": fieldvalue.String("New")},
			wantErrText: "no frontmatter found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var fm *parser.Frontmatter
			if tt.frontmatter {
				var err error
				fm, err = parser.ParseFrontmatter(tt.content)
				if err != nil {
					t.Fatalf("parse seed frontmatter: %v", err)
				}
			}

			updated, warnings, err := PrepareValidatedFrontmatterMutationValues(
				tt.content,
				fm,
				"task",
				tt.updates,
				mutationTestSchema(),
				tt.allowedUnknown,
				nil,
			)
			if tt.wantErr != nil {
				if err == nil || reflect.TypeOf(err) != reflect.TypeOf(tt.wantErr) {
					t.Fatalf("error = %T %v, want %T", err, err, tt.wantErr)
				}
				var unknown *UnknownFieldMutationError
				if errors.As(err, &unknown) {
					want := []string{"a_unknown", "z_unknown"}
					if !reflect.DeepEqual(unknown.Unknown, want) {
						t.Fatalf("unknown fields = %#v, want %#v", unknown.Unknown, want)
					}
				}
				return
			}
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareValidatedFrontmatterMutationValues() error = %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %#v, want none", warnings)
			}
			if tt.verify != nil {
				tt.verify(t, updated)
			}
		})
	}
}

func TestPrepareFrontmatterUnset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		fields       []string
		wantErrText  string
		wantRemoved  map[string]interface{}
		wantMissing  []string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:    "removes optional fields and reports deduplicated missing fields",
			content: "---\ntype: task\ntitle: Ship tests\nnote: remember\nstatus: todo\n---\n# Body\n",
			fields:  []string{" note ", "missing", "note", "", "missing"},
			wantRemoved: map[string]interface{}{
				"note": "remember",
			},
			wantMissing:  []string{"missing"},
			wantContains: []string{"type: task", "title: Ship tests", "status: todo", "# Body"},
			wantAbsent:   []string{"note: remember"},
		},
		{
			name:         "missing only leaves content unchanged",
			content:      "---\ntype: task\ntitle: Ship tests\n---\n# Body\n",
			fields:       []string{"missing"},
			wantRemoved:  map[string]interface{}{},
			wantMissing:  []string{"missing"},
			wantContains: []string{"title: Ship tests", "# Body"},
		},
		{
			name:        "rejects reserved type",
			content:     "---\ntype: task\ntitle: Ship tests\n---\n",
			fields:      []string{"type"},
			wantErrText: "cannot unset reserved field 'type'",
		},
		{
			name:        "rejects required field",
			content:     "---\ntype: task\ntitle: Ship tests\n---\n",
			fields:      []string{"title"},
			wantErrText: "cannot unset required field 'title'",
		},
		{
			name:        "rejects missing frontmatter",
			content:     "# Body\n",
			fields:      []string{"note"},
			wantErrText: "no frontmatter found",
		},
		{
			name:        "rejects unclosed frontmatter",
			content:     "---\ntype: task\nnote: value\n",
			fields:      []string{"note"},
			wantErrText: "unclosed frontmatter",
		},
		{
			name:        "rejects malformed frontmatter",
			content:     "---\ntype: [task\n---\n",
			fields:      []string{"note"},
			wantErrText: "failed to parse frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			updated, removed, missing, err := PrepareFrontmatterUnset(tt.content, tt.fields, mutationTestSchema())
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareFrontmatterUnset() error = %v", err)
			}
			gotRemoved := make(map[string]interface{}, len(removed))
			for name, value := range removed {
				gotRemoved[name] = value.Raw()
			}
			if !reflect.DeepEqual(gotRemoved, tt.wantRemoved) {
				t.Fatalf("removed = %#v, want %#v", gotRemoved, tt.wantRemoved)
			}
			if !reflect.DeepEqual(missing, tt.wantMissing) {
				t.Fatalf("missing = %#v, want %#v", missing, tt.wantMissing)
			}
			for _, value := range tt.wantContains {
				if !strings.Contains(updated, value) {
					t.Fatalf("updated content missing %q:\n%s", value, updated)
				}
			}
			for _, value := range tt.wantAbsent {
				if strings.Contains(updated, value) {
					t.Fatalf("updated content contains removed %q:\n%s", value, updated)
				}
			}
		})
	}
}

func TestValidateRefTargets(t *testing.T) {
	t.Parallel()

	fieldDefs := map[string]*schema.FieldDefinition{
		"owner":     {Type: schema.FieldTypeRef, Target: "person"},
		"attendees": {Type: schema.FieldTypeRefArray, Target: "person"},
		"label":     {Type: schema.FieldTypeString, Target: "person"},
	}
	refCtx := &RefValidationContext{
		ResolveTargetType: func(rawReference string) (string, error) {
			return map[string]string{
				"people/alex":    "person",
				"companies/acme": "company",
			}[rawReference], nil
		},
	}

	tests := []struct {
		name        string
		fields      map[string]fieldvalue.FieldValue
		context     *RefValidationContext
		wantField   string
		wantMessage string
	}{
		{
			name:    "nil context skips filesystem validation",
			fields:  map[string]fieldvalue.FieldValue{"owner": fieldvalue.Ref("companies/acme")},
			context: nil,
		},
		{
			name:    "matching scalar target",
			fields:  map[string]fieldvalue.FieldValue{"owner": fieldvalue.Ref("people/alex")},
			context: refCtx,
		},
		{
			name:        "wrong scalar target",
			fields:      map[string]fieldvalue.FieldValue{"owner": fieldvalue.Ref("companies/acme")},
			context:     refCtx,
			wantField:   "owner",
			wantMessage: "resolves to type 'company', expected 'person'",
		},
		{
			name: "matching array targets",
			fields: map[string]fieldvalue.FieldValue{
				"attendees": fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.Ref("people/alex")}),
			},
			context: refCtx,
		},
		{
			name: "wrong array target reports field once",
			fields: map[string]fieldvalue.FieldValue{
				"attendees": fieldvalue.Array([]fieldvalue.FieldValue{
					fieldvalue.Ref("companies/acme"),
					fieldvalue.Ref("companies/acme"),
				}),
			},
			context:     refCtx,
			wantField:   "attendees",
			wantMessage: "expected 'person'",
		},
		{
			name:    "unresolved references are ignored",
			fields:  map[string]fieldvalue.FieldValue{"owner": fieldvalue.Ref("people/missing")},
			context: refCtx,
		},
		{
			name:   "index rebuild requirement blocks validation",
			fields: map[string]fieldvalue.FieldValue{"owner": fieldvalue.Ref("people/alex")},
			context: &RefValidationContext{
				Prepare: func() error {
					return ErrRefValidationIndexRebuildRequired
				},
				ResolveTargetType: refCtx.ResolveTargetType,
			},
			wantField:   "reference",
			wantMessage: "index requires a full reindex before validating writes",
		},
		{
			name:    "null references are ignored",
			fields:  map[string]fieldvalue.FieldValue{"owner": fieldvalue.Null()},
			context: refCtx,
		},
		{
			name:    "target metadata on non-reference field is ignored",
			fields:  map[string]fieldvalue.FieldValue{"label": fieldvalue.String("companies/acme")},
			context: refCtx,
		},
	}

	for _, tt := range tests {
		issues := validateRefTargets(tt.fields, fieldDefs, tt.context)
		if tt.wantField == "" {
			if len(issues) != 0 {
				t.Errorf("%s: issues = %#v, want none", tt.name, issues)
			}
			continue
		}
		if len(issues) != 1 {
			t.Errorf("%s: issues = %#v, want one", tt.name, issues)
			continue
		}
		if issues[0].Field != tt.wantField || !strings.Contains(issues[0].Message, tt.wantMessage) {
			t.Errorf("%s: issue = %#v, want field %q containing %q", tt.name, issues[0], tt.wantField, tt.wantMessage)
		}
	}
}

func TestFieldValueJSONAndLiteralHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    map[string]interface{}
		wantErr bool
	}{
		{
			name: "object values",
			raw:  `{"title":"42","done":true,"count":2,"items":["a",null]}`,
			want: map[string]interface{}{
				"title": "42",
				"done":  true,
				"count": float64(2),
				"items": []interface{}{"a", nil},
			},
		},
		{name: "empty input", raw: " ", want: nil},
		{name: "null is not object", raw: "null", wantErr: true},
		{name: "array is not object", raw: "[]", wantErr: true},
		{name: "malformed JSON", raw: "{", wantErr: true},
	}

	for _, tt := range tests {
		values, err := ParseFieldValuesJSON(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: error = nil, want error", tt.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: ParseFieldValuesJSON() error = %v", tt.name, err)
			continue
		}
		if tt.want == nil {
			if values != nil {
				t.Errorf("%s: values = %#v, want nil", tt.name, values)
			}
			continue
		}
		got := make(map[string]interface{}, len(values))
		for name, value := range values {
			got[name] = value.Raw()
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: values = %#v, want %#v", tt.name, got, tt.want)
		}
	}

	literals := SerializeFieldValueMap(map[string]fieldvalue.FieldValue{
		"null":   fieldvalue.Null(),
		"ref":    fieldvalue.Ref("people/alex"),
		"array":  fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("a"), fieldvalue.Number(2)}),
		"quoted": fieldvalue.String("true"),
		"number": fieldvalue.Number(2.5),
		"bool":   fieldvalue.Bool(false),
	})
	wantLiterals := map[string]string{
		"null":   "null",
		"ref":    "[[people/alex]]",
		"array":  "[a, 2]",
		"quoted": `"true"`,
		"number": "2.5",
		"bool":   "false",
	}
	if !reflect.DeepEqual(literals, wantLiterals) {
		t.Fatalf("SerializeFieldValueMap() = %#v, want %#v", literals, wantLiterals)
	}
}

func mutationTestSchema() *schema.Schema {
	sch := schema.New()
	sch.Types["task"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{
		"title":     {Type: schema.FieldTypeString, Required: true},
		"status":    {Type: schema.FieldTypeEnum, Values: []string{"todo", "done"}},
		"due":       {Type: schema.FieldTypeDate},
		"note":      {Type: schema.FieldTypeString},
		"owner":     {Type: schema.FieldTypeRef, Target: "person"},
		"attendees": {Type: schema.FieldTypeRefArray, Target: "person"},
	}}
	sch.Types["person"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{
		"name": {Type: schema.FieldTypeString},
	}}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{
		"name": {Type: schema.FieldTypeString},
	}}
	return sch
}
