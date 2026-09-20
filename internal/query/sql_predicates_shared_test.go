package query

import "testing"

func TestRefResolvedIDPreferredMatchSQL(t *testing.T) {
	t.Parallel()

	got := refResolvedIDPreferredMatchSQL("r", "?")
	want := "(r.target_id = ? OR (r.target_id IS NULL AND r.target_raw = ?))"
	if got != want {
		t.Fatalf("bound form = %q, want %q", got, want)
	}

	got = refResolvedIDPreferredMatchSQL("r", "o.id")
	want = "(r.target_id = o.id OR (r.target_id IS NULL AND r.target_raw = o.id))"
	if got != want {
		t.Fatalf("join form = %q, want %q", got, want)
	}

	cond, args := refResolvedIDPreferredMatch("fr", "companies/acme", "acme")
	if cond != refResolvedIDPreferredMatchSQL("fr", "?") {
		t.Fatalf("bound helper SQL = %q, want the shared fragment", cond)
	}
	if len(args) != 2 || args[0] != "companies/acme" || args[1] != "acme" {
		t.Fatalf("args = %#v, want (resolvedID, rawQuery)", args)
	}
}
