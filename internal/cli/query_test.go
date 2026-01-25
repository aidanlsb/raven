package cli

import (
	"testing"
)

func TestJoinQueryArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "single arg unchanged",
			args: []string{`trait:due content:"hello world"`},
			want: `trait:due content:"hello world"`,
		},
		{
			name: "multiple args joined with space",
			args: []string{"trait:due", ".value==past"},
			want: "trait:due .value==past",
		},
		{
			name: "content with shell-stripped quotes gets re-quoted",
			args: []string{"trait:due", "content:hello world"},
			want: `trait:due content:"hello world"`,
		},
		{
			name: "negated content with shell-stripped quotes gets re-quoted",
			args: []string{"trait:due", "!content:hello world"},
			want: `trait:due !content:"hello world"`,
		},
		{
			name: "content already quoted stays quoted",
			args: []string{"trait:due", `content:"hello world"`},
			want: `trait:due content:"hello world"`,
		},
		{
			name: "content with single word gets quoted",
			args: []string{"trait:due", "content:hello"},
			want: `trait:due content:"hello"`,
		},
		{
			name: "mixed predicates",
			args: []string{"trait:due", "content:my task", ".value==past"},
			want: `trait:due content:"my task" .value==past`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinQueryArgs(tt.args)
			if got != tt.want {
				t.Errorf("joinQueryArgs(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
