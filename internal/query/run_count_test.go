package query

import "testing"

func TestInferredPageTotal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		limit     int
		offset    int
		returned  int
		wantTotal int
		wantOK    bool
	}{
		{name: "unpaged", limit: 0, offset: 0, returned: 12, wantTotal: 12, wantOK: true},
		{name: "short first page", limit: 10, offset: 0, returned: 3, wantTotal: 3, wantOK: true},
		{name: "full first page needs count", limit: 10, offset: 0, returned: 10, wantOK: false},
		{name: "short later page", limit: 10, offset: 20, returned: 4, wantTotal: 24, wantOK: true},
		{name: "empty later page needs count", limit: 10, offset: 20, returned: 0, wantOK: false},
		{name: "offset only with rows", limit: 0, offset: 5, returned: 7, wantTotal: 12, wantOK: true},
		{name: "offset only empty needs count", limit: 0, offset: 5, returned: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := inferredPageTotal(tt.limit, tt.offset, tt.returned)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && got != tt.wantTotal {
				t.Fatalf("total = %d, want %d", got, tt.wantTotal)
			}
		})
	}
}

func TestRunInfersTotalOnShortPage(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	q, err := Parse("type:project")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	run, err := exec.Run(q, RunRequest{Limit: 50})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.Total != 2 || run.Returned != 2 {
		t.Fatalf("total=%d returned=%d, want 2/2", run.Total, run.Returned)
	}
}
