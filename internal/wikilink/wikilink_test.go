package wikilink

import "testing"

func TestParseExact(t *testing.T) {
	tests := []struct {
		in          string
		wantTarget  string
		wantDisplay *string
		wantOK      bool
	}{
		{in: "[[people/freya]]", wantTarget: "people/freya", wantOK: true},
		{in: " [[people/freya]] ", wantTarget: "people/freya", wantOK: true},
		{
			in:         "[[people/freya|Lady Freya]]",
			wantTarget: "people/freya",
			wantDisplay: func() *string {
				s := "Lady Freya"
				return &s
			}(),
			wantOK: true,
		},
		{in: "[[]]", wantOK: false},
		{in: "people/freya", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			target, display, ok := ParseExact(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if target != tt.wantTarget {
				t.Fatalf("target=%q, want %q", target, tt.wantTarget)
			}
			if (display == nil) != (tt.wantDisplay == nil) {
				t.Fatalf("display nil=%v, want %v", display == nil, tt.wantDisplay == nil)
			}
			if display != nil && *display != *tt.wantDisplay {
				t.Fatalf("display=%q, want %q", *display, *tt.wantDisplay)
			}
		})
	}
}

func TestFindAllInLine(t *testing.T) {
	line := "See [[a]] and [[b|B]] and [[[c]]]"
	m := FindAllInLine(line, false)
	if len(m) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m))
	}
	if m[0].Target != "a" || m[1].Target != "b" {
		t.Fatalf("unexpected targets: %#v", []string{m[0].Target, m[1].Target})
	}

	m2 := FindAllInLine(line, true)
	if len(m2) != 3 {
		t.Fatalf("expected 3 matches with allowTriple=true, got %d", len(m2))
	}
	if m2[2].Target != "c" {
		t.Fatalf("expected third match target=c, got %q", m2[2].Target)
	}
}

func TestScanAt(t *testing.T) {
	input := `x [[people/freya|Freya]] y`
	end, target, literal, ok := ScanAt(input, 2)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if target != "people/freya" {
		t.Fatalf("target=%q, want %q", target, "people/freya")
	}
	if literal != "[[people/freya|Freya]]" {
		t.Fatalf("literal=%q", literal)
	}
	if end != 2+len(literal) {
		t.Fatalf("end=%d, want %d", end, 2+len(literal))
	}
}
