package dates

import (
	"testing"
	"time"
)

func TestNormalizeRelativeDateKeyword(t *testing.T) {
	if got, ok := NormalizeRelativeDateKeyword(" today "); !ok || got != "today" {
		t.Fatalf("NormalizeRelativeDateKeyword(today) = %q, %v", got, ok)
	}
	if _, ok := NormalizeRelativeDateKeyword("this-week"); ok {
		t.Fatalf("expected this-week to be rejected")
	}
}

func TestResolveRelativeDateKeyword_Instants(t *testing.T) {
	now := time.Date(2026, time.March, 4, 14, 30, 0, 0, time.UTC) // Wednesday

	today, ok := ResolveRelativeDateKeyword("today", now, time.Monday)
	if !ok {
		t.Fatalf("expected today to resolve")
	}
	if today.Date.Format(DateLayout) != "2026-03-04" {
		t.Fatalf("unexpected today: %s", today.Date.Format(DateLayout))
	}

	tomorrow, ok := ResolveRelativeDateKeyword("tomorrow", now, time.Monday)
	if !ok {
		t.Fatalf("expected tomorrow to resolve")
	}
	if tomorrow.Date.Format(DateLayout) != "2026-03-05" {
		t.Fatalf("unexpected tomorrow: %s", tomorrow.Date.Format(DateLayout))
	}

	yesterday, ok := ResolveRelativeDateKeyword("yesterday", now, time.Monday)
	if !ok {
		t.Fatalf("expected yesterday to resolve")
	}
	if yesterday.Date.Format(DateLayout) != "2026-03-03" {
		t.Fatalf("unexpected yesterday: %s", yesterday.Date.Format(DateLayout))
	}
}
