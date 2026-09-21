package sectionsvc

import "testing"

func TestRewriteSectionRefAtLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		oldRaw  string
		newRaw  string
		want    string
	}{
		{
			name:    "wikilink and alias on indexed line",
			content: "See [[projects/site#tasks]] and [[projects/site#tasks|the tasks]].\n",
			line:    1,
			oldRaw:  "projects/site#tasks",
			newRaw:  "projects/site#done",
			want:    "See [[projects/site#done]] and [[projects/site#done|the tasks]].\n",
		},
		{
			name:    "markdown link fallback on indexed line",
			content: "Body [tasks](projects/site#tasks) and [angle](<projects/site#tasks>).\n",
			line:    1,
			oldRaw:  "projects/site#tasks",
			newRaw:  "projects/site#done",
			want:    "Body [tasks](projects/site#done) and [angle](<projects/site#done>).\n",
		},
		{
			name:    "unindexed fenced example is left alone",
			content: "See [[projects/site#tasks]]\n\n```markdown\n[[projects/site#tasks]] and [tasks](projects/site#tasks)\n```\n",
			line:    1,
			oldRaw:  "projects/site#tasks",
			newRaw:  "projects/site#done",
			want:    "See [[projects/site#done]]\n\n```markdown\n[[projects/site#tasks]] and [tasks](projects/site#tasks)\n```\n",
		},
		{
			name:    "identical target is a no-op",
			content: "See [[projects/site#tasks]].\n",
			line:    1,
			oldRaw:  "projects/site#tasks",
			newRaw:  "projects/site#tasks",
			want:    "See [[projects/site#tasks]].\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rewriteSectionRefAtLine(tt.content, tt.line, tt.oldRaw, tt.newRaw); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
