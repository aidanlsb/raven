package model

import "testing"

func TestSectionParentScopeID(t *testing.T) {
	t.Parallel()

	parentID := "project/raven#planning"
	tests := []struct {
		name    string
		section *Section
		want    string
	}{
		{
			name: "top-level section uses file object",
			section: &Section{
				FileObjectID: "project/raven",
			},
			want: "project/raven",
		},
		{
			name: "nested section uses parent section",
			section: &Section{
				FileObjectID:    "project/raven",
				ParentSectionID: &parentID,
			},
			want: parentID,
		},
		{
			name: "nil section",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.section.ParentScopeID(); got != tt.want {
				t.Errorf("ParentScopeID() = %q, want %q", got, tt.want)
			}
		})
	}
}
