package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/resolver"
)

// SetResolver injects a resolver for target resolution.
//
// This allows callers (CLI/MCP) to provide a canonical resolver that includes
// aliases and keep-specific settings like daily directory. If not set, the
// executor will fall back to building a resolver from the objects table.
func (e *Executor) SetResolver(r *resolver.Resolver) {
	e.resolver = r
}

// SetDailyDirectory sets the daily directory used when building a fallback resolver.
//
// This matters for date shorthand references like [[2026-01-01]], which resolve to
// <dailyDirectory>/2026-01-01.
func (e *Executor) SetDailyDirectory(dir string) {
	if dir == "" {
		dir = "daily"
	}
	e.dailyDirectory = dir
}

// getResolver returns a resolver for target resolution, creating it if needed.
func (e *Executor) getResolver() (*resolver.Resolver, error) {
	if e.resolver != nil {
		return e.resolver, nil
	}

	res, err := index.BuildResolver(e.db, index.ResolverOptions{
		DailyDirectory: e.dailyDirectory,
		Schema:         e.schema,
	})
	if err != nil {
		return nil, fmt.Errorf("build resolver: %w", err)
	}
	e.resolver = res
	return e.resolver, nil
}

// resolveTarget resolves a reference to an object ID.
// Returns the resolved ID or an error if ambiguous.
func (e *Executor) resolveTarget(target string) (string, error) {
	res, err := e.getResolver()
	if err != nil {
		return "", err
	}

	result := res.Resolve(target)
	if result.Ambiguous {
		return "", fmt.Errorf("ambiguous reference '%s' - matches: %s",
			target, strings.Join(result.Matches, ", "))
	}
	if result.TargetID == "" {
		// Not found - return the original target (will match nothing)
		return target, nil
	}
	return result.TargetID, nil
}
