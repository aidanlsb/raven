package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/resolver"
)

// SetResolver injects a resolver for target resolution.
//
// This allows callers (CLI/LSP) to provide a canonical resolver that includes
// aliases and vault-specific settings like daily directory. If not set, the
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

	// Query all object IDs from the database
	rows, err := e.db.Query("SELECT id FROM objects")
	if err != nil {
		return nil, fmt.Errorf("failed to get object IDs: %w", err)
	}
	defer rows.Close()

	var objectIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		objectIDs = append(objectIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Query all aliases from the database
	aliasRows, err := e.db.Query("SELECT alias, id FROM objects WHERE alias IS NOT NULL AND alias != '' ORDER BY id")
	if err != nil {
		// Fall back to resolver without aliases
		e.resolver = resolver.New(objectIDs)
		return e.resolver, nil
	}
	defer aliasRows.Close()

	aliases := make(map[string]string)
	for aliasRows.Next() {
		var alias, id string
		if err := aliasRows.Scan(&alias, &id); err != nil {
			continue
		}
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = id
		}
	}
	if err := aliasRows.Err(); err != nil {
		// Fall back to resolver without aliases (avoid partial/incorrect alias maps)
		e.resolver = resolver.New(objectIDs)
		return e.resolver, nil
	}

	dailyDir := e.dailyDirectory
	if dailyDir == "" {
		dailyDir = "daily"
	}
	e.resolver = resolver.NewWithAliases(objectIDs, aliases, dailyDir)
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
