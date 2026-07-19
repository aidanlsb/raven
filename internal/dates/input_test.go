package dates

import (
	"testing"
	"time"
)

func TestParseInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 23, 59, 0, 0, time.UTC)
	tests := []struct {
		name         string
		raw          string
		wantOK       bool
		wantKind     InputKind
		wantCalendar string
	}{
		{
			name:         "absolute date",
			raw:          "2026-04-04",
			wantOK:       true,
			wantKind:     InputDateLiteral,
			wantCalendar: "2026-04-04",
		},
		{
			name:         "relative today",
			raw:          " today ",
			wantOK:       true,
			wantKind:     InputRelativeDate,
			wantCalendar: "2026-04-05",
		},
		{
			name:         "relative tomorrow",
			raw:          "tomorrow",
			wantOK:       true,
			wantKind:     InputRelativeDate,
			wantCalendar: "2026-04-06",
		},
		{
			name:         "datetime",
			raw:          "2026-04-05T12:30",
			wantOK:       true,
			wantKind:     InputDatetimeLiteral,
			wantCalendar: "2026-04-05",
		},
		{
			name:   "non date",
			raw:    "active",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseInput(tt.raw, now)
			if err != nil {
				t.Fatalf("ParseInput returned error: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Kind != tt.wantKind {
				t.Fatalf("Kind = %v, want %v", got.Kind, tt.wantKind)
			}
			if got.CalendarDate != tt.wantCalendar {
				t.Fatalf("CalendarDate = %q, want %q", got.CalendarDate, tt.wantCalendar)
			}
		})
	}
}

func TestParseInputInvalidDateLiteral(t *testing.T) {
	t.Parallel()

	_, ok, err := ParseInput("2026-99-99", time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected invalid date error")
	}
	if ok {
		t.Fatal("expected ok=false for invalid date literal")
	}
}

func TestParseInputInvalidDatetimeLiteral(t *testing.T) {
	t.Parallel()

	_, ok, err := ParseInput("2026-04-05T99:00", time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected invalid datetime error")
	}
	if ok {
		t.Fatal("expected ok=false for invalid datetime literal")
	}
}

func TestDailyObjectIDForInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)
	got, ok, err := DailyObjectIDForInput("today", now)
	if err != nil {
		t.Fatalf("DailyObjectIDForInput returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected date input to resolve")
	}
	// The canonical daily-note object ID is a bare ISO date.
	if got != "2026-04-05" {
		t.Fatalf("DailyObjectIDForInput = %q, want 2026-04-05", got)
	}
}
