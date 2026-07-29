package frontmatter

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/fieldvalue"
)

func TestRenderPlacesTypeFirstAndSortsFields(t *testing.T) {
	t.Parallel()

	got, err := Render("project", map[string]fieldvalue.FieldValue{
		"zeta":  fieldvalue.String("last"),
		"alpha": fieldvalue.String("first"),
	}, nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "---\ntype: project\nalpha: first\nzeta: last\n---\n"
	if got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderEmitsOmittedBlankFields(t *testing.T) {
	t.Parallel()

	got, err := Render("project", map[string]fieldvalue.FieldValue{
		"name": fieldvalue.String("Raven"),
	}, map[string]bool{
		"owner": true,
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "---\ntype: project\nname: Raven\nowner: \n---\n"
	if got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderFieldValueRoundTrips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value fieldvalue.FieldValue
		want  interface{}
	}{
		{name: "string", value: fieldvalue.String("Raven"), want: "Raven"},
		{name: "reference", value: fieldvalue.Ref("person/freya"), want: "[[person/freya]]"},
		{
			name: "array",
			value: fieldvalue.Array([]fieldvalue.FieldValue{
				fieldvalue.String("Raven"),
				fieldvalue.Ref("person/freya"),
				fieldvalue.Null(),
			}),
			want: []interface{}{"Raven", "[[person/freya]]", nil},
		},
		{name: "null", value: fieldvalue.Null(), want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered, err := Render("", map[string]fieldvalue.FieldValue{
				"value": tt.value,
			}, nil)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			decoded := decodeRenderedFrontmatter(t, rendered)
			if got := decoded["value"]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("round-tripped value = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRenderDataPreservesUnknownKeys(t *testing.T) {
	t.Parallel()

	input := map[string]interface{}{
		"type":          "project",
		"future_scalar": "preserved",
		"future_list":   []interface{}{"one", "two"},
	}
	rendered, err := RenderData(input, nil)
	if err != nil {
		t.Fatalf("RenderData() error = %v", err)
	}

	got := decodeRenderedFrontmatter(t, rendered)
	if !reflect.DeepEqual(got, input) {
		t.Errorf("RenderData() decoded = %#v, want %#v", got, input)
	}
}

func decodeRenderedFrontmatter(t *testing.T, rendered string) map[string]interface{} {
	t.Helper()

	const delimiter = "---\n"
	if !strings.HasPrefix(rendered, delimiter) || !strings.HasSuffix(rendered, delimiter) {
		t.Fatalf("rendered frontmatter is missing delimiters: %q", rendered)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(rendered, delimiter), delimiter)

	var decoded map[string]interface{}
	if err := yaml.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	return decoded
}
