package query

import (
	"database/sql/driver"
	"fmt"
	"regexp"

	"modernc.org/sqlite"
)

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

	re, err := regexp.Compile(pattern)
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
