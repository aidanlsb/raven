package query

import (
	"container/list"
	"database/sql/driver"
	"fmt"
	"regexp"
	"sync"

	"modernc.org/sqlite"
)

const regexpCacheMaxEntries = 256

type regexpCacheEntry struct {
	pattern string
	re      *regexp.Regexp
}

var regexpCache = struct {
	mu       sync.Mutex
	compiled map[string]*list.Element
	order    *list.List
}{
	compiled: map[string]*list.Element{},
	order:    list.New(),
}

func init() {
	// Provide REGEXP support for matches() predicates.
	// SQLite invokes the "regexp" function with (pattern, value).
	sqlite.MustRegisterDeterministicScalarFunction("regexp", 2, regexpFunc)
}

func regexpFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("regexp expects 2 arguments")
	}

	pattern, ok := driverValueToString(args[0])
	if !ok || pattern == "" {
		return int64(0), nil
	}
	value, ok := driverValueToString(args[1])
	if !ok {
		return int64(0), nil
	}

	re, err := cachedRegexp(pattern)
	if err != nil {
		return nil, err
	}

	if re.MatchString(value) {
		return int64(1), nil
	}
	return int64(0), nil
}

func driverValueToString(v driver.Value) (string, bool) {
	switch val := v.(type) {
	case nil:
		return "", false
	case string:
		return val, true
	case []byte:
		return string(val), true
	default:
		return fmt.Sprint(val), true
	}
}

func cachedRegexp(pattern string) (*regexp.Regexp, error) {
	regexpCache.mu.Lock()
	if elem := regexpCache.compiled[pattern]; elem != nil {
		regexpCache.order.MoveToFront(elem)
		re := elem.Value.(*regexpCacheEntry).re
		regexpCache.mu.Unlock()
		return re, nil
	}
	regexpCache.mu.Unlock()

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, newExecutionError(
			fmt.Sprintf("invalid regex pattern %q: %v", pattern, err),
			"Fix the regex passed to matches() and retry.",
			err,
		)
	}

	regexpCache.mu.Lock()
	defer regexpCache.mu.Unlock()
	if elem := regexpCache.compiled[pattern]; elem != nil {
		regexpCache.order.MoveToFront(elem)
		return elem.Value.(*regexpCacheEntry).re, nil
	}

	elem := regexpCache.order.PushFront(&regexpCacheEntry{
		pattern: pattern,
		re:      compiled,
	})
	regexpCache.compiled[pattern] = elem
	if len(regexpCache.compiled) > regexpCacheMaxEntries {
		oldest := regexpCache.order.Back()
		if oldest != nil {
			regexpCache.order.Remove(oldest)
			delete(regexpCache.compiled, oldest.Value.(*regexpCacheEntry).pattern)
		}
	}

	return compiled, nil
}

func resetRegexpCacheForTest() {
	regexpCache.mu.Lock()
	defer regexpCache.mu.Unlock()

	regexpCache.compiled = make(map[string]*list.Element)
	regexpCache.order.Init()
}

func regexpCacheLenForTest() int {
	regexpCache.mu.Lock()
	defer regexpCache.mu.Unlock()

	return len(regexpCache.compiled)
}
