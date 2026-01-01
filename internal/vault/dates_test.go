package vault

import (
	"testing"
	"time"
)

func TestParseDateArg(t *testing.T) {
	// Get current time for comparisons
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	tomorrow := today.AddDate(0, 0, 1)

	t.Run("empty defaults to today", func(t *testing.T) {
		result, err := ParseDateArg("")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !sameDay(result, today) {
			t.Errorf("got %v, want %v", result, today)
		}
	})

	t.Run("today", func(t *testing.T) {
		result, err := ParseDateArg("today")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !sameDay(result, today) {
			t.Errorf("got %v, want %v", result, today)
		}
	})

	t.Run("TODAY (case insensitive)", func(t *testing.T) {
		result, err := ParseDateArg("TODAY")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !sameDay(result, today) {
			t.Errorf("got %v, want %v", result, today)
		}
	})

	t.Run("yesterday", func(t *testing.T) {
		result, err := ParseDateArg("yesterday")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !sameDay(result, yesterday) {
			t.Errorf("got %v, want %v", result, yesterday)
		}
	})

	t.Run("tomorrow", func(t *testing.T) {
		result, err := ParseDateArg("tomorrow")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !sameDay(result, tomorrow) {
			t.Errorf("got %v, want %v", result, tomorrow)
		}
	})

	t.Run("specific date", func(t *testing.T) {
		result, err := ParseDateArg("2025-02-15")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Year() != 2025 || result.Month() != time.February || result.Day() != 15 {
			t.Errorf("got %v, want 2025-02-15", result)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := ParseDateArg("02-15-2025")
		if err == nil {
			t.Error("expected error for invalid format")
		}
	})

	t.Run("garbage", func(t *testing.T) {
		_, err := ParseDateArg("not-a-date")
		if err == nil {
			t.Error("expected error for garbage input")
		}
	})
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func TestFormatDateISO(t *testing.T) {
	date := time.Date(2025, 2, 15, 10, 30, 0, 0, time.UTC)
	result := FormatDateISO(date)
	if result != "2025-02-15" {
		t.Errorf("FormatDateISO = %q, want %q", result, "2025-02-15")
	}
}

func TestFormatDateFriendly(t *testing.T) {
	date := time.Date(2025, 2, 15, 10, 30, 0, 0, time.UTC)
	result := FormatDateFriendly(date)
	if result != "Saturday, February 15, 2025" {
		t.Errorf("FormatDateFriendly = %q, want %q", result, "Saturday, February 15, 2025")
	}
}
