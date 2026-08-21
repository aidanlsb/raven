package commandpayload

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestMutationPayloadJSONKeysRemainStable(t *testing.T) {
	t.Parallel()

	falseValue := false
	tests := []struct {
		name string
		data any
		keys []string
	}{
		{
			name: "new without missing refs",
			data: NewResult{ObjectMutation: ObjectMutation{
				ID: "project/raven", File: "projects/raven.md", Title: "Raven", Type: "project",
			}},
			keys: []string{"file", "id", "title", "type"},
		},
		{
			name: "set omits optional preview fields",
			data: SetResult{
				File: "projects/raven.md", ObjectID: "project/raven", Type: "project",
				UpdatedFields: map[string]string{"status": "done"},
			},
			keys: []string{"file", "object_id", "type", "updated_fields"},
		},
		{
			name: "unset retains empty collection fields",
			data: UnsetResult{
				File: "projects/raven.md", ObjectID: "project/raven", Type: "project",
				RemovedFields: map[string]string{},
			},
			keys: []string{"file", "missing_fields", "modified", "object_id", "previous_fields", "removed_fields", "type"},
		},
		{
			name: "move confirmation retains false preview",
			data: MoveConfirmationResult{
				Source: "note/a", Destination: "projects/a", NeedsConfirm: true,
			},
			keys: []string{"destination", "needs_confirm", "preview", "reason", "source"},
		},
		{
			name: "edit retains empty replacement",
			data: EditSingleResult{
				Status: "applied", Path: "note/a.md", Line: 3, OldStr: "remove", NewStr: "",
			},
			keys: []string{"context", "line", "new_str", "old_str", "path", "status"},
		},
		{
			name: "bulk preview retains warnings",
			data: SetBulkPreviewResult{
				BulkPreviewResult: BulkPreviewResult{
					Preview: true, Action: "set", Items: []BulkPreviewItem{}, Skipped: []BulkResult{},
				},
				Fields: map[string]string{},
			},
			keys: []string{"action", "fields", "items", "preview", "skipped", "total", "warnings"},
		},
		{
			name: "empty query apply retains minimal bulk shape",
			data: QueryApplyEmptyResult{
				Preview: true, Action: "delete", Items: []interface{}{},
			},
			keys: []string{"action", "items", "preview", "total"},
		},
		{
			name: "section delete retains subtree details",
			data: SectionDeleteResult{
				Section: "project/raven#tasks", File: "projects/raven.md", Status: "deleted",
			},
			keys: []string{"backlinks", "deleted_sections", "file", "line_end", "line_start", "removed_content", "section", "status"},
		},
		{
			name: "vault unset omits set-only created",
			data: VaultConfigAutoReindexResult{
				ConfigPath: "raven.yaml", Changed: false, AutoReindex: true,
			},
			keys: []string{"auto_reindex", "auto_reindex_explicit", "changed", "config_path"},
		},
		{
			name: "schema bind retains false default match",
			data: SchemaTemplateBindResult{
				Type: "project", TemplateID: "brief", AlreadySet: true, DefaultMatch: &falseValue,
			},
			keys: []string{"already_set", "default_match", "template_id", "type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := payloadJSONKeys(t, tt.data)
			sort.Strings(tt.keys)
			if !reflect.DeepEqual(got, tt.keys) {
				t.Fatalf("JSON keys = %v, want %v", got, tt.keys)
			}
		})
	}
}

func payloadJSONKeys(t *testing.T, value any) []string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
