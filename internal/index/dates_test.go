package index

import (
	"strings"
	"testing"
	"time"
)

func TestParseDateFilter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC)
	today := "2026-03-04"
	yesterday := "2026-03-03"
	tomorrow := "2026-03-05"

	tests := []struct {
		name          string
		filter        string
		wantCondition string // partial match
		wantArgs      []interface{}
	}{
		{
			name:          "today",
			filter:        "today",
			wantCondition: "= ?",
			wantArgs:      []interface{}{today},
		},
		{
			name:          "yesterday",
			filter:        "yesterday",
			wantCondition: "= ?",
			wantArgs:      []interface{}{yesterday},
		},
		{
			name:          "tomorrow",
			filter:        "tomorrow",
			wantCondition: "= ?",
			wantArgs:      []interface{}{tomorrow},
		},
		{
			name:          "specific date",
			filter:        "2025-02-01",
			wantCondition: "= ?",
			wantArgs:      []interface{}{"2025-02-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, err := ParseDateFilterWithOptions(tt.filter, "field", DateFilterOptions{
				Now: now,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(condition, tt.wantCondition) {
				t.Errorf("condition %q does not contain %q", condition, tt.wantCondition)
			}

			if len(args) != len(tt.wantArgs) {
				t.Errorf("got %d args, want %d", len(args), len(tt.wantArgs))
			}
			for i, want := range tt.wantArgs {
				if i < len(args) && args[i] != want {
					t.Errorf("arg[%d]: got %v, want %v", i, args[i], want)
				}
			}
		})
	}
}

func TestParseDateFilterInvalidDates(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC)
	tests := []string{
		"2025-13-45",
		"2025-02-30",
		"not-a-date",
		"past",
		"this-week",
	}

	for _, filter := range tests {
		t.Run(filter, func(t *testing.T) {
			_, _, err := ParseDateFilterWithOptions(filter, "field", DateFilterOptions{
				Now: now,
			})
			if err == nil {
				t.Fatalf("expected error for %q, got nil", filter)
			}
		})
	}
}

func TestParseDateFilterCaseInsensitive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC)
	today := "2026-03-04"

	tests := []string{"TODAY", "Today", "  today  "}

	for _, filter := range tests {
		t.Run(filter, func(t *testing.T) {
			condition, args, err := ParseDateFilterWithOptions(filter, "field", DateFilterOptions{
				Now: now,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(condition, "= ?") {
				t.Errorf("expected = ?, got %s", condition)
			}
			if len(args) != 1 || args[0] != today {
				t.Errorf("expected args [%s], got %v", today, args)
			}
		})
	}
}

func TestTryParseDateComparisonWithOptions_InstantOrdering(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC)

	cond, args, ok, err := TryParseDateComparisonWithOptions("today", "<", "value", DateFilterOptions{
		Now: now,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatalf("expected date comparison parse")
	}
	if cond != "value < ?" {
		t.Fatalf("cond = %q", cond)
	}
	if len(args) != 1 || args[0] != "2026-03-04" {
		t.Fatalf("args = %v", args)
	}
}
