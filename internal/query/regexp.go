package query

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"sync"

	"modernc.org/sqlite"
)

var regexpCache = struct {
	mu       sync.RWMutex
	compiled map[string]*regexp.Regexp
}{
	compiled: map[string]*regexp.Regexp{},
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
	regexpCache.mu.RLock()
	re := regexpCache.compiled[pattern]
	regexpCache.mu.RUnlock()
	if re != nil {
		return re, nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexpCache.mu.Lock()
	defer regexpCache.mu.Unlock()
	if existing := regexpCache.compiled[pattern]; existing != nil {
		return existing, nil
	}
	regexpCache.compiled[pattern] = compiled
	return compiled, nil
}
