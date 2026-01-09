package dates

import (
	"testing"
	"time"
)

func TestIsValidDate(t *testing.T) {
	valid := []string{"2025-01-01", "2024-12-31", "2000-06-15"}
	for _, d := range valid {
		if !IsValidDate(d) {
			t.Fatalf("expected %q to be valid", d)
		}
	}

	invalid := []string{"2025/01/01", "01-01-2025", "2025-13-01", "2025-01-32", "not-a-date", "", "2025-02-30"}
	for _, d := range invalid {
		if IsValidDate(d) {
			t.Fatalf("expected %q to be invalid", d)
		}
	}
}

func TestIsValidDatetime(t *testing.T) {
	valid := []string{
		"2025-01-01T10:30:00Z",
		"2025-01-01T10:30",
		"2025-01-01T10:30:45",
		"2025-06-15T14:00:00+05:00",
	}
	for _, dt := range valid {
		if !IsValidDatetime(dt) {
			t.Fatalf("expected %q to be valid", dt)
		}
	}

	invalid := []string{"2025-01-01", "10:30", "not-a-datetime", ""}
	for _, dt := range invalid {
		if IsValidDatetime(dt) {
			t.Fatalf("expected %q to be invalid", dt)
		}
	}
}

func TestParseDateArg(t *testing.T) {
	now := time.Date(2025, 2, 15, 10, 0, 0, 0, time.UTC)

	today, err := ParseDateArg("", now)
	if err != nil || !today.Equal(now) {
		t.Fatalf("empty arg should default to now, got %v err=%v", today, err)
	}

	d, err := ParseDateArg("2025-02-01", now)
	if err != nil || d.Year() != 2025 || d.Month() != time.February || d.Day() != 1 {
		t.Fatalf("expected 2025-02-01, got %v err=%v", d, err)
	}

	_, err = ParseDateArg("02-01-2025", now)
	if err == nil {
		t.Fatalf("expected error for invalid date arg")
	}
}

