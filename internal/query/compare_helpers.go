package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
)

func toNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case string:
		n = strings.TrimSpace(n)
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

type cmpKind int

const (
	cmpNil cmpKind = iota
	cmpNumber
	cmpTemporal // date or datetime
	cmpString
)

type cmpVal struct {
	kind cmpKind
	num  float64
	t    time.Time
	s    string
}

func normalizeForCompare(v interface{}) cmpVal {
	// nil
	if v == nil {
		return cmpVal{kind: cmpNil}
	}

	// Common pointer shapes (avoid comparing pointer addresses).
	switch vv := v.(type) {
	case *string:
		if vv == nil {
			return cmpVal{kind: cmpNil}
		}
		v = *vv
	}

	// Numbers
	if n, ok := toNumber(v); ok {
		return cmpVal{kind: cmpNumber, num: n}
	}

	// Strings / temporal detection
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if dates.IsValidDatetime(s) {
			if t, err := dates.ParseDatetime(s); err == nil {
				return cmpVal{kind: cmpTemporal, t: t, s: s}
			}
		}
		if dates.IsValidDate(s) {
			if t, err := dates.ParseDate(s); err == nil {
				return cmpVal{kind: cmpTemporal, t: t, s: s}
			}
		}
		return cmpVal{kind: cmpString, s: s}
	}

	// Fallback: stringify
	return cmpVal{kind: cmpString, s: fmt.Sprint(v)}
}

func compareValues(a, b interface{}) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	av := normalizeForCompare(a)
	bv := normalizeForCompare(b)

	// Normalized nil handling
	if av.kind == cmpNil && bv.kind == cmpNil {
		return 0
	}
	if av.kind == cmpNil {
		return -1
	}
	if bv.kind == cmpNil {
		return 1
	}

	// If both are numbers, compare numerically.
	if av.kind == cmpNumber && bv.kind == cmpNumber {
		switch {
		case av.num < bv.num:
			return -1
		case av.num > bv.num:
			return 1
		default:
			return 0
		}
	}

	// If both are temporal, compare by parsed time.
	if av.kind == cmpTemporal && bv.kind == cmpTemporal {
		switch {
		case av.t.Before(bv.t):
			return -1
		case av.t.After(bv.t):
			return 1
		default:
			return 0
		}
	}

	// Fallback: string comparison (preserves prior behavior for mixed types).
	if av.s < bv.s {
		return -1
	}
	if av.s > bv.s {
		return 1
	}
	return 0
}
