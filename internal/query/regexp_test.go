package query

import (
	"fmt"
	"regexp"
	"sync"
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

func TestCachedRegexpInvalidPatternIsNotCached(t *testing.T) {
	resetRegexpCacheForTest()
	t.Cleanup(resetRegexpCacheForTest)

	for i := 0; i < 2; i++ {
		if _, err := cachedRegexp("("); err == nil {
			t.Fatalf("call %d: expected invalid regexp error", i+1)
		}
	}
	if got := regexpCacheLenForTest(); got != 0 {
		t.Fatalf("cache size = %d, want 0 after invalid patterns", got)
	}
}

func TestCachedRegexpConcurrentSamePatternReusesCompiledPattern(t *testing.T) {
	resetRegexpCacheForTest()
	t.Cleanup(resetRegexpCacheForTest)

	const goroutines = 32
	pattern := "^(cache|race)-me$"
	start := make(chan struct{})
	results := make([]*regexp.Regexp, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], errs[idx] = cachedRegexp(pattern)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: cachedRegexp error = %v", i, err)
		}
	}
	first := results[0]
	if first == nil {
		t.Fatal("expected compiled regexp, got nil")
	}
	for i, result := range results[1:] {
		if result != first {
			t.Fatalf("goroutine %d: expected pointer reuse, got %p want %p", i+1, result, first)
		}
	}
	if got := regexpCacheLenForTest(); got != 1 {
		t.Fatalf("cache size = %d, want 1", got)
	}
}

func TestCachedRegexpConcurrentDifferentPatterns(t *testing.T) {
	resetRegexpCacheForTest()
	t.Cleanup(resetRegexpCacheForTest)

	patterns := []string{
		"^alpha$",
		"^beta$",
		"^gamma$",
		"^delta$",
		"^epsilon$",
		"^zeta$",
	}
	start := make(chan struct{})
	errs := make(chan error, len(patterns))

	var wg sync.WaitGroup
	wg.Add(len(patterns))
	for _, pattern := range patterns {
		go func(pattern string) {
			defer wg.Done()
			<-start
			_, err := cachedRegexp(pattern)
			errs <- err
		}(pattern)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("cachedRegexp error = %v", err)
		}
	}
	if got := regexpCacheLenForTest(); got != len(patterns) {
		t.Fatalf("cache size = %d, want %d", got, len(patterns))
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
