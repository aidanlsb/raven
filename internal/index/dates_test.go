package index

import (
	"testing"
	"time"
)

func TestTryParseTemporalComparisonWithOptions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		filter        string
		op            string
		wantCondition string
		wantArg       interface{}
		wantOK        bool
		wantErr       bool
	}{
		{
			name:          "relative today equals compares by date",
			filter:        "today",
			op:            "=",
			wantCondition: "date(value) = date(?)",
			wantArg:       "2026-03-04",
			wantOK:        true,
		},
		{
			name:          "relative tomorrow less-than",
			filter:        "tomorrow",
			op:            "<",
			wantCondition: "date(value) < date(?)",
			wantArg:       "2026-03-05",
			wantOK:        true,
		},
		{
			name:          "absolute date greater-or-equal",
			filter:        "2025-02-01",
			op:            ">=",
			wantCondition: "date(value) >= date(?)",
			wantArg:       "2025-02-01",
			wantOK:        true,
		},
		{
			name:          "datetime literal compares by datetime",
			filter:        "2026-04-05T12:30",
			op:            "=",
			wantCondition: "datetime(value) = datetime(?)",
			wantArg:       "2026-04-05T12:30",
			wantOK:        true,
		},
		{
			name:   "non-temporal value is not a date filter",
			filter: "active",
			op:     "=",
			wantOK: false,
		},
		{
			name:    "invalid date literal errors",
			filter:  "2025-13-45",
			op:      "=",
			wantErr: true,
		},
		{
			name:    "unsupported operator errors",
			filter:  "today",
			op:      "LIKE",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, ok, err := TryParseTemporalComparisonWithOptions(tt.filter, tt.op, "value", DateFilterOptions{
				Now: now,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if condition != tt.wantCondition {
				t.Errorf("condition = %q, want %q", condition, tt.wantCondition)
			}
			if len(args) != 1 {
				t.Fatalf("args len = %d, want 1", len(args))
			}
			if args[0] != tt.wantArg {
				t.Errorf("arg = %v, want %v", args[0], tt.wantArg)
			}
		})
	}
}

func TestTryParseTemporalComparisonWithOptionsDefaultsNow(t *testing.T) {
	t.Parallel()

	// A zero Now falls back to time.Now(); the parse should still succeed and
	// yield a date comparison for a relative keyword.
	condition, args, ok, err := TryParseTemporalComparisonWithOptions("today", "=", "value", DateFilterOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected relative keyword to parse")
	}
	if condition != "date(value) = date(?)" {
		t.Errorf("condition = %q", condition)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
}
