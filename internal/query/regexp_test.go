package query

import "testing"

func TestCachedRegexpReusesCompiledPattern(t *testing.T) {
	pattern := "^cache-me$"

	first, err := cachedRegexp(pattern)
	if err != nil {
		t.Fatalf("cachedRegexp() error = %v", err)
	}
	second, err := cachedRegexp(pattern)
	if err != nil {
		t.Fatalf("cachedRegexp() second call error = %v", err)
	}

	if first != second {
		t.Fatalf("expected cached regexp pointer reuse, got %p and %p", first, second)
	}
}

func TestCachedRegexpRejectsInvalidPattern(t *testing.T) {
	if _, err := cachedRegexp("("); err == nil {
		t.Fatal("expected invalid regexp error")
	}
}
