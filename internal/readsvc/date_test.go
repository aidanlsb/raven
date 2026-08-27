package readsvc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDateAssociationJSONContract(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(DateAssociation{
		Date: "2026-08-27", SourceType: "object", SourceID: "projects/raven",
		FieldName: "due", FilePath: "projects/raven.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{`"source_type"`, `"source_id"`, `"field_name"`, `"file_path"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON = %s, want %s", got, want)
		}
	}
	if strings.Contains(got, `"Trait"`) || strings.Contains(got, `"Object"`) {
		t.Fatalf("JSON = %s, want omitted nil detail objects", got)
	}
}
