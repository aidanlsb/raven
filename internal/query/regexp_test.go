package query

import (
	"fmt"
	"regexp"
	"testing"
)

func TestCachedRegexpReusesCompiledPattern(t *testing.T) {
	resetRegexpCacheForTest()
	t.Cleanup(resetRegexpCacheForTest)

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
	resetRegexpCacheForTest()
	t.Cleanup(resetRegexpCacheForTest)

	if _, err := cachedRegexp("("); err == nil {
		t.Fatal("expected invalid regexp error")
	}
}

func TestCachedRegexpEvictsLeastRecentlyUsedEntry(t *testing.T) {
	resetRegexpCacheForTest()
	t.Cleanup(resetRegexpCacheForTest)

	firstPattern := "^0$"
	first, err := cachedRegexp(firstPattern)
	if err != nil {
		t.Fatalf("cachedRegexp(%q) error = %v", firstPattern, err)
	}

	evictedPattern := "^1$"
	var evicted *regexp.Regexp
	for i := 1; i < regexpCacheMaxEntries; i++ {
		compiled, err := cachedRegexp(fmt.Sprintf("^%d$", i))
		if err != nil {
			t.Fatalf("cachedRegexp(%d) error = %v", i, err)
		}
		if i == 1 {
			evicted = compiled
		}
	}

	if _, err := cachedRegexp(firstPattern); err != nil {
		t.Fatalf("cachedRegexp(%q) refresh error = %v", firstPattern, err)
	}

	if _, err := cachedRegexp("^overflow$"); err != nil {
		t.Fatalf("cachedRegexp(overflow) error = %v", err)
	}

	if got := regexpCacheLenForTest(); got != regexpCacheMaxEntries {
		t.Fatalf("cache size = %d, want %d", got, regexpCacheMaxEntries)
	}

	reloadedFirst, err := cachedRegexp(firstPattern)
	if err != nil {
		t.Fatalf("cachedRegexp(%q) reload error = %v", firstPattern, err)
	}
	if reloadedFirst != first {
		t.Fatalf("expected %q to remain cached", firstPattern)
	}

	reloadedEvicted, err := cachedRegexp(evictedPattern)
	if err != nil {
		t.Fatalf("cachedRegexp(%q) reload error = %v", evictedPattern, err)
	}
	if reloadedEvicted == evicted {
		t.Fatalf("expected %q to be evicted and recompiled", evictedPattern)
	}
}
