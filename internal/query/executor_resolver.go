package query

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/slugs"
)

// SetDailyDirectory sets the daily directory used when building a fallback resolver.
//
// Date shorthand references like [[2026-01-01]] resolve to the bare-date object ID
// (2026-01-01). The daily directory is retained so legacy prefixed references
// (e.g. [[daily/2026-01-01]]) still resolve to the same bare-date object.
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

	res, err := indexschema.BuildResolver(e.db, indexschema.ResolverOptions{
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
	if id, ok, err := e.tryResolveExactID(target); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}

	res, err := e.getResolver()
	if err != nil {
		return "", err
	}

	result := res.Resolve(target)
	if result.Ambiguous {
		return "", newExecutionError(
			fmt.Sprintf("ambiguous reference '%s' - matches: %s", target, strings.Join(result.Matches, ", ")),
			"Use a full object ID/path to disambiguate",
			nil,
		)
	}
	if result.TargetID == "" {
		// Not found - return the original target (will match nothing)
		return target, nil
	}
	return result.TargetID, nil
}

// tryResolveExactID answers canonical object/section IDs with a targeted
// primary-key lookup instead of loading every object and section into the
// resolver. It falls back (ok=false) whenever another resolver rule could
// produce a different or ambiguous match: aliases, name fields, short names,
// or date-prefix aliases.
func (e *Executor) tryResolveExactID(target string) (string, bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false, nil
	}
	normalized := normalizeQueryRef(target)
	if normalized == "" {
		normalized = target
	}

	id, found, err := e.lookupExactID(normalized)
	if err != nil {
		return "", false, err
	}
	if !found && normalized != target {
		id, found, err = e.lookupExactID(target)
		if err != nil {
			return "", false, err
		}
	}
	if !found {
		return "", false, nil
	}

	if date, ok := e.bareDateFromRef(normalized); ok && date != id {
		return "", false, nil
	}
	if date, ok := e.bareDateFromRef(target); ok && date != id {
		return "", false, nil
	}

	collides, err := e.exactIDHasCollisions(target, normalized, id)
	if err != nil || collides {
		return "", false, err
	}
	return id, true, nil
}

func (e *Executor) lookupExactID(id string) (string, bool, error) {
	if id == "" {
		return "", false, nil
	}
	var found string
	err := e.db.QueryRow(`SELECT id FROM objects WHERE id = ?`, id).Scan(&found)
	if err == nil {
		return found, true, nil
	}
	if err != sql.ErrNoRows && !isMissingTableError(err, "objects") {
		return "", false, err
	}

	err = e.db.QueryRow(`SELECT id FROM sections WHERE id = ?`, id).Scan(&found)
	if err == nil {
		return found, true, nil
	}
	if err == sql.ErrNoRows || isMissingTableError(err, "sections") {
		return "", false, nil
	}
	return "", false, err
}

func (e *Executor) exactIDHasCollisions(raw, normalized, id string) (bool, error) {
	if hit, err := e.aliasCollides(raw, normalized, id); err != nil || hit {
		return hit, err
	}
	if hit, err := e.nameFieldCollides(raw, normalized, id); err != nil || hit {
		return hit, err
	}
	matchRef := normalized
	if matchRef == "" {
		matchRef = raw
	}
	if isPathLikeQueryRef(matchRef) {
		return false, nil
	}
	return e.shortNameCollides(matchRef, id)
}

func isPathLikeQueryRef(ref string) bool {
	return strings.Contains(ref, "/") || strings.Contains(ref, "#")
}

func normalizeQueryRef(ref string) string {
	baseRef, fragment, isSection := paths.ParseSectionID(ref)
	baseRef = paths.TrimMDExtension(baseRef)
	if !isSection {
		return baseRef
	}
	return baseRef + "#" + fragment
}

func (e *Executor) aliasCollides(raw, normalized, id string) (bool, error) {
	candidates := uniqueNonEmpty(raw, normalized, slugs.ComponentSlug(raw), slugs.ComponentSlug(normalized))
	if len(candidates) == 0 {
		return false, nil
	}
	placeholders := strings.Repeat("?,", len(candidates))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, 0, len(candidates)+1)
	for _, c := range candidates {
		args = append(args, c)
	}
	args = append(args, id)

	var other string
	err := e.db.QueryRow(
		`SELECT id FROM objects WHERE alias IS NOT NULL AND alias != '' AND alias IN (`+placeholders+`) AND id != ? LIMIT 1`,
		args...,
	).Scan(&other)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows || isMissingTableError(err, "objects") || isMissingColumnError(err, "alias") {
		return false, nil
	}
	return false, err
}

func (e *Executor) nameFieldCollides(raw, normalized, id string) (bool, error) {
	if e.schema == nil {
		return false, nil
	}
	typeNameFields := indexschema.BuildTypeNameFields(e.schema)
	if len(typeNameFields) == 0 {
		return false, nil
	}
	values := uniqueNonEmpty(raw, normalized, strings.ToLower(raw), strings.ToLower(normalized),
		slugs.ComponentSlug(raw), slugs.ComponentSlug(normalized))
	for objType, nameField := range typeNameFields {
		jsonPath := jsonFieldPath(nameField)
		for _, value := range values {
			var other string
			err := e.db.QueryRow(
				`SELECT id FROM objects WHERE type = ? AND json_type(fields, ?) = 'text' AND json_extract(fields, ?) = ? AND id != ? LIMIT 1`,
				objType, jsonPath, jsonPath, value, id,
			).Scan(&other)
			if err == nil {
				return true, nil
			}
			if err != sql.ErrNoRows && !isMissingTableError(err, "objects") {
				return false, err
			}
		}
	}
	return false, nil
}

func (e *Executor) shortNameCollides(ref, id string) (bool, error) {
	var other string
	err := e.db.QueryRow(
		`SELECT id FROM objects WHERE id != ? AND (id = ? OR id LIKE '%/' || ?) LIMIT 1`,
		id, ref, ref,
	).Scan(&other)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows && !isMissingTableError(err, "objects") {
		return false, err
	}

	err = e.db.QueryRow(
		`SELECT id FROM sections WHERE id != ? AND (id = ? OR id LIKE '%/' || ? OR id LIKE '%/#' || ? OR id LIKE '%#' || ?) LIMIT 1`,
		id, ref, ref, ref, ref,
	).Scan(&other)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows || isMissingTableError(err, "sections") {
		return false, nil
	}
	return false, err
}

func (e *Executor) bareDateFromRef(ref string) (string, bool) {
	baseRef, _, _ := paths.ParseSectionID(ref)
	baseRef = paths.TrimMDExtension(baseRef)
	if dates.IsValidDate(baseRef) {
		return baseRef, true
	}
	dailyDir := e.dailyDirectory
	if dailyDir == "" {
		dailyDir = "daily"
	}
	for _, dir := range []string{dailyDir, "daily"} {
		prefix := dir + "/"
		if strings.HasPrefix(baseRef, prefix) {
			if rest := strings.TrimPrefix(baseRef, prefix); dates.IsValidDate(rest) {
				return rest, true
			}
		}
	}
	return "", false
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func isMissingTableError(err error, table string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") && strings.Contains(msg, strings.ToLower(table))
}

func isMissingColumnError(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such column") && strings.Contains(msg, strings.ToLower(column))
}
