package indexschema

import (
	"database/sql"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
)

// TryResolveExactID answers canonical object/section IDs with a targeted
// primary-key lookup instead of loading every object and section into the
// resolver. It falls back (ok=false) whenever another resolver rule could
// produce a different or ambiguous match: aliases, name fields, short names,
// or date-prefix aliases.
func TryResolveExactID(db ResolverQuerier, sch *schema.Schema, dailyDirectory, target string) (string, bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false, nil
	}
	normalized := normalizeExactIDRef(target)
	if normalized == "" {
		normalized = target
	}

	id, found, err := lookupExactID(db, normalized)
	if err != nil {
		return "", false, err
	}
	if !found && normalized != target {
		id, found, err = lookupExactID(db, target)
		if err != nil {
			return "", false, err
		}
	}
	if !found {
		return "", false, nil
	}

	if date, ok := bareDateFromRef(dailyDirectory, normalized); ok && date != id {
		return "", false, nil
	}
	if date, ok := bareDateFromRef(dailyDirectory, target); ok && date != id {
		return "", false, nil
	}

	collides, err := exactIDHasCollisions(db, sch, target, normalized, id)
	if err != nil || collides {
		return "", false, err
	}
	return id, true, nil
}

func lookupExactID(db ResolverQuerier, id string) (string, bool, error) {
	if id == "" {
		return "", false, nil
	}
	var found string
	err := db.QueryRow(`SELECT id FROM objects WHERE id = ?`, id).Scan(&found)
	if err == nil {
		return found, true, nil
	}
	if err != sql.ErrNoRows && !isMissingRelationError(err, "objects") {
		return "", false, err
	}

	err = db.QueryRow(`SELECT id FROM sections WHERE id = ?`, id).Scan(&found)
	if err == nil {
		return found, true, nil
	}
	if err == sql.ErrNoRows || isMissingRelationError(err, "sections") {
		return "", false, nil
	}
	return "", false, err
}

func exactIDHasCollisions(db ResolverQuerier, sch *schema.Schema, raw, normalized, id string) (bool, error) {
	if hit, err := aliasCollides(db, raw, normalized, id); err != nil || hit {
		return hit, err
	}
	if hit, err := nameFieldCollides(db, sch, raw, normalized, id); err != nil || hit {
		return hit, err
	}
	matchRef := normalized
	if matchRef == "" {
		matchRef = raw
	}
	if isPathLikeRef(matchRef) {
		return false, nil
	}
	return shortNameCollides(db, matchRef, id)
}

func isPathLikeRef(ref string) bool {
	return strings.Contains(ref, "/") || strings.Contains(ref, "#")
}

func normalizeExactIDRef(ref string) string {
	baseRef, fragment, isSection := paths.ParseSectionID(ref)
	baseRef = paths.TrimMDExtension(baseRef)
	if !isSection {
		return baseRef
	}
	return baseRef + "#" + fragment
}

func aliasCollides(db ResolverQuerier, raw, normalized, id string) (bool, error) {
	candidates := uniqueNonEmpty(raw, normalized, slugs.ComponentSlug(raw), slugs.ComponentSlug(normalized))
	if len(candidates) == 0 {
		return false, nil
	}
	placeholders := strings.Repeat("?,", len(candidates))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(candidates)+1)
	for _, c := range candidates {
		args = append(args, c)
	}
	args = append(args, id)

	var other string
	err := db.QueryRow(
		`SELECT id FROM objects WHERE alias IS NOT NULL AND alias != '' AND alias IN (`+placeholders+`) AND id != ? LIMIT 1`,
		args...,
	).Scan(&other)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows || isMissingRelationError(err, "objects") || isMissingColumnError(err, "alias") {
		return false, nil
	}
	return false, err
}

func nameFieldCollides(db ResolverQuerier, sch *schema.Schema, raw, normalized, id string) (bool, error) {
	if sch == nil {
		return false, nil
	}
	typeNameFields := BuildTypeNameFields(sch)
	if len(typeNameFields) == 0 {
		return false, nil
	}
	values := uniqueNonEmpty(raw, normalized, strings.ToLower(raw), strings.ToLower(normalized),
		slugs.ComponentSlug(raw), slugs.ComponentSlug(normalized))
	for objType, nameField := range typeNameFields {
		jsonPath := "$." + nameField
		for _, value := range values {
			var other string
			err := db.QueryRow(
				`SELECT id FROM objects WHERE type = ? AND json_type(fields, ?) = 'text' AND json_extract(fields, ?) = ? AND id != ? LIMIT 1`,
				objType, jsonPath, jsonPath, value, id,
			).Scan(&other)
			if err == nil {
				return true, nil
			}
			if err != sql.ErrNoRows && !isMissingRelationError(err, "objects") {
				return false, err
			}
		}
	}
	return false, nil
}

func shortNameCollides(db ResolverQuerier, ref, id string) (bool, error) {
	var other string
	err := db.QueryRow(
		`SELECT id FROM objects WHERE id != ? AND (id = ? OR id LIKE '%/' || ?) LIMIT 1`,
		id, ref, ref,
	).Scan(&other)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows && !isMissingRelationError(err, "objects") {
		return false, err
	}

	err = db.QueryRow(
		`SELECT id FROM sections WHERE id != ? AND (id = ? OR id LIKE '%/' || ? OR id LIKE '%/#' || ? OR id LIKE '%#' || ?) LIMIT 1`,
		id, ref, ref, ref, ref,
	).Scan(&other)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows || isMissingRelationError(err, "sections") {
		return false, nil
	}
	return false, err
}

func bareDateFromRef(dailyDirectory, ref string) (string, bool) {
	baseRef, _, _ := paths.ParseSectionID(ref)
	baseRef = paths.TrimMDExtension(baseRef)
	if dates.IsValidDate(baseRef) {
		return baseRef, true
	}
	dailyDir := DefaultDailyDirectory(dailyDirectory)
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
