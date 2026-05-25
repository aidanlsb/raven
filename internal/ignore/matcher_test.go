package ignore

import "testing"

func TestMatcherGitignoreSemantics(t *testing.T) {
	t.Parallel()

	matcher, err := NewMatcher([]string{
		"AGENTS.md",
		".cursor/",
		"*.plan.md",
		"assets/generated/**",
		"/root-only.md",
		"logs/**",
		"!logs/keep.md",
		"**/node_modules/",
		"a/**/b.md",
	})
	if err != nil {
		t.Fatalf("NewMatcher returned error: %v", err)
	}

	for _, tt := range []struct {
		name string
		path string
		dir  bool
		want bool
	}{
		{name: "basename file", path: "AGENTS.md", want: true},
		{name: "nested basename file", path: "nested/AGENTS.md", want: true},
		{name: "directory itself", path: ".cursor", dir: true, want: true},
		{name: "directory child", path: ".cursor/plans/work.plan.md", want: true},
		{name: "extension glob", path: "notes/work.plan.md", want: true},
		{name: "double star", path: "assets/generated/deep/file.png", want: true},
		{name: "root anchored match", path: "root-only.md", want: true},
		{name: "root anchored miss", path: "nested/root-only.md", want: false},
		{name: "negated file", path: "logs/keep.md", want: false},
		{name: "double star prefix", path: "project/node_modules/pkg/index.js", want: true},
		{name: "double star middle zero dirs", path: "a/b.md", want: true},
		{name: "double star middle nested dirs", path: "a/x/y/b.md", want: true},
		{name: "non excluded", path: "notes/work.md", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matcher.Match(tt.path, tt.dir); got != tt.want {
				t.Fatalf("Match(%q, %t) = %t, want %t", tt.path, tt.dir, got, tt.want)
			}
		})
	}
}

func TestNormalizePatterns(t *testing.T) {
	t.Parallel()

	got := NormalizePatterns([]string{" AGENTS.md ", "", "AGENTS.md", "*.plan.md", "bad\npattern"})
	want := []string{"AGENTS.md", "AGENTS.md", "*.plan.md"}
	if len(got) != len(want) {
		t.Fatalf("NormalizePatterns length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizePatterns[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
