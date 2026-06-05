package docssvc

import (
	"testing"
	"testing/fstest"
)

func TestSearchFSPaginatesWithHasMore(t *testing.T) {
	docsFS := fstest.MapFS{
		"index.yaml": {
			Data: []byte(`sections:
  guide:
    topics:
      alpha:
        path: alpha.md
      beta:
        path: beta.md
`),
		},
		"guide/alpha.md": {Data: []byte("# Alpha\n\nquery one\nquery two\n")},
		"guide/beta.md":  {Data: []byte("# Beta\n\nquery three\nquery four\n")},
	}

	first, err := SearchFS(docsFS, ".", "query", "", 2, 0)
	if err != nil {
		t.Fatalf("SearchFS first page: %v", err)
	}
	if first.Returned != 2 || first.Limit != 2 || first.Offset != 0 {
		t.Fatalf("first page metadata = returned %d limit %d offset %d", first.Returned, first.Limit, first.Offset)
	}
	if !first.HasMore {
		t.Fatalf("first page HasMore = false, want true")
	}
	if got := first.Matches[0].Line; got != 3 {
		t.Fatalf("first match line = %d, want 3", got)
	}

	second, err := SearchFS(docsFS, ".", "query", "", 2, 2)
	if err != nil {
		t.Fatalf("SearchFS second page: %v", err)
	}
	if second.Returned != 2 || second.Limit != 2 || second.Offset != 2 {
		t.Fatalf("second page metadata = returned %d limit %d offset %d", second.Returned, second.Limit, second.Offset)
	}
	if second.HasMore {
		t.Fatalf("second page HasMore = true, want false")
	}
	if got := second.Matches[0].Topic; got != "beta" {
		t.Fatalf("second page first topic = %q, want beta", got)
	}
}

func TestSearchFSExactLimitDoesNotReportHasMore(t *testing.T) {
	docsFS := fstest.MapFS{
		"index.yaml": {
			Data: []byte(`sections:
  guide:
    topics:
      alpha:
        path: alpha.md
`),
		},
		"guide/alpha.md": {Data: []byte("# Alpha\n\nquery one\nquery two\n")},
	}

	result, err := SearchFS(docsFS, ".", "query", "", 2, 0)
	if err != nil {
		t.Fatalf("SearchFS: %v", err)
	}
	if result.HasMore {
		t.Fatalf("HasMore = true, want false")
	}
	if result.Returned != 2 {
		t.Fatalf("Returned = %d, want 2", result.Returned)
	}
}

func TestSearchFSRejectsInvalidOffset(t *testing.T) {
	_, err := SearchFS(fstest.MapFS{}, ".", "query", "", 2, -1)
	if err == nil {
		t.Fatalf("SearchFS error = nil, want invalid offset error")
	}
}
