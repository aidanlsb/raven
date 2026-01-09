package index

import (
	"strings"
	"testing"
	"time"
)

func TestParseDateFilter(t *testing.T) {
	// Get today's date for relative tests
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

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
		{
			name:          "past",
			filter:        "past",
			wantCondition: "< ?",
			wantArgs:      []interface{}{today},
		},
		{
			name:          "future",
			filter:        "future",
			wantCondition: "> ?",
			wantArgs:      []interface{}{today},
		},
		{
			name:          "this-week",
			filter:        "this-week",
			wantCondition: ">= ?",
			wantArgs:      nil, // will have 2 args, just check condition shape
		},
		{
			name:          "next-week",
			filter:        "next-week",
			wantCondition: ">= ?",
			wantArgs:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, err := ParseDateFilter(tt.filter, "field")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(condition, tt.wantCondition) {
				t.Errorf("condition %q does not contain %q", condition, tt.wantCondition)
			}

			if tt.wantArgs != nil {
				if len(args) != len(tt.wantArgs) {
					t.Errorf("got %d args, want %d", len(args), len(tt.wantArgs))
				}
				for i, want := range tt.wantArgs {
					if i < len(args) && args[i] != want {
						t.Errorf("arg[%d]: got %v, want %v", i, args[i], want)
					}
				}
			}
		})
	}
}

func TestParseDateFilterInvalidDates(t *testing.T) {
	tests := []string{
		"2025-13-45",
		"2025-02-30",
		"not-a-date",
	}

	for _, filter := range tests {
		t.Run(filter, func(t *testing.T) {
			_, _, err := ParseDateFilter(filter, "field")
			if err == nil {
				t.Fatalf("expected error for %q, got nil", filter)
			}
		})
	}
}

func TestParseDateFilterCaseInsensitive(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	tests := []string{"TODAY", "Today", "TODAY", "  today  "}

	for _, filter := range tests {
		t.Run(filter, func(t *testing.T) {
			condition, args, err := ParseDateFilter(filter, "field")
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

func TestParseDateFilterWeekBoundaries(t *testing.T) {
	// This test verifies that week calculations produce valid dates
	condition, args, err := ParseDateFilter("this-week", "field")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have two date arguments (start and end of week)
	if len(args) != 2 {
		t.Fatalf("expected 2 args for this-week, got %d", len(args))
	}

	startDate := args[0].(string)
	endDate := args[1].(string)

	// Parse dates to verify they're valid
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		t.Errorf("invalid start date: %s", startDate)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		t.Errorf("invalid end date: %s", endDate)
	}

	// End should be after start
	if !end.After(start) && !end.Equal(start) {
		t.Errorf("end date %s should be >= start date %s", endDate, startDate)
	}

	// Verify condition uses both args
	if !strings.Contains(condition, ">=") || !strings.Contains(condition, "<=") {
		t.Errorf("expected range condition, got %s", condition)
	}
}
