package indexschema

import "testing"

func TestSanitizeFTSQuery_QuotesPunctuationTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "column-like token",
			in:   "type:project",
			want: `"type:project"`,
		},
		{
			name: "url",
			in:   "https://github.com",
			want: `"https://github.com"`,
		},
		{
			name: "leading hyphen",
			in:   "-todo",
			want: `"-todo"`,
		},
		{
			name: "at-mention",
			in:   "@todo",
			want: `"@todo"`,
		},
		{
			name: "grouped words become AND so FTS does not treat them as a function",
			in:   "foo (bar)",
			want: `"foo" AND ("bar")`,
		},
		{
			name: "boolean grouping keeps parentheses without extra AND",
			in:   "(foo OR bar) AND baz",
			want: `("foo" OR "bar") AND "baz"`,
		},
		{
			name: "bare words are quoted",
			in:   "hello world",
			want: `"hello" "world"`,
		},
		{
			name: "boolean operators and quoted phrase",
			in:   `michael-truell OR "Michael Truell"`,
			want: `"michael-truell" OR "Michael Truell"`,
		},
		{
			name: "prefix query is preserved",
			in:   "foo*",
			want: "foo*",
		},
		{
			name: "prefix query with punctuation is quoted",
			in:   "type:pro*",
			want: `"type:pro*"`,
		},
		{
			name: "NEAR distance is preserved",
			in:   "hello NEAR/3 world",
			want: `"hello" NEAR/3 "world"`,
		},
		{
			name: "AND/OR/NOT stay operators",
			in:   "foo AND bar OR NOT baz",
			want: `"foo" AND "bar" OR NOT "baz"`,
		},
		{
			name: "function-like token",
			in:   "content()",
			want: `"content()"`,
		},
		{
			name: "dotted identifier",
			in:   "inputs.project",
			want: `"inputs.project"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFTSQuery(tt.in)
			if got != tt.want {
				t.Fatalf("sanitizeFTSQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildFTSContentQuery_QuotesColumnLikeTokens(t *testing.T) {
	t.Parallel()

	got := BuildFTSContentQuery(`type:project OR "plain phrase"`)
	want := `content: ("type:project" OR "plain phrase")`
	if got != want {
		t.Fatalf("BuildFTSContentQuery() = %q, want %q", got, want)
	}
}

func TestBuildFTSSearchQuery_QuotesAtMention(t *testing.T) {
	t.Parallel()

	got := BuildFTSSearchQuery("@todo")
	want := `{title content}: ("@todo")`
	if got != want {
		t.Fatalf("BuildFTSSearchQuery() = %q, want %q", got, want)
	}
}
