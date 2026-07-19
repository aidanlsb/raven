package index

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

type referenceResolverCache struct {
	resolver       *resolver.Resolver
	dailyDirectory string
	schemaKey      string
	schema         *schema.Schema
	generation     int64
}

type resolverFileState struct {
	objectIDs  map[string]struct{}
	assetIDs   map[string]struct{}
	aliases    map[string]map[string]struct{}
	nameFields map[string]map[string]struct{}
}

func newResolverFileState() *resolverFileState {
	return &resolverFileState{
		objectIDs:  make(map[string]struct{}),
		assetIDs:   make(map[string]struct{}),
		aliases:    make(map[string]map[string]struct{}),
		nameFields: make(map[string]map[string]struct{}),
	}
}

func resolverSchemaKey(sch *schema.Schema) string {
	if sch == nil {
		return "nil"
	}
	nameFields := buildTypeNameFields(sch)
	typeNames := make([]string, 0, len(nameFields))
	for typeName := range nameFields {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)

	var key strings.Builder
	key.WriteString("schema")
	for _, typeName := range typeNames {
		key.WriteByte('\x00')
		key.WriteString(typeName)
		key.WriteByte('=')
		key.WriteString(nameFields[typeName])
	}
	return key.String()
}

func (d *Database) prepareReferenceResolverCacheLocked(sch *schema.Schema) {
	if d.referenceResolverCache == nil {
		return
	}
	if d.referenceResolverCache.schemaKey != resolverSchemaKey(sch) {
		d.referenceResolverCache = nil
	}
}

func (d *Database) ensureReferenceResolverCacheCurrentLocked(db resolverQuerier) error {
	if d.referenceResolverCache == nil {
		return nil
	}
	generation, err := resolverGeneration(db)
	if err != nil {
		d.referenceResolverCache = nil
		return err
	}
	if d.referenceResolverCache.generation != generation {
		d.referenceResolverCache = nil
	}
	return nil
}

func (d *Database) setReferenceResolverGenerationLocked(generation int64) {
	if d.referenceResolverCache != nil {
		d.referenceResolverCache.generation = generation
	}
}

func (d *Database) cachedResolverFileStateLocked(tx *sql.Tx, filePath string) (*resolverFileState, error) {
	if d.referenceResolverCache == nil {
		return nil, nil
	}
	return loadResolverFileState(tx, filePath, d.referenceResolverCache.schema)
}

func (d *Database) updateReferenceResolverCacheLocked(oldState, newState *resolverFileState) {
	if d.referenceResolverCache == nil || (oldState == nil && newState == nil) {
		return
	}
	if oldState == nil {
		oldState = newResolverFileState()
	}
	if newState == nil {
		newState = newResolverFileState()
	}
	d.referenceResolverCache.resolver.ApplyUpdate(diffResolverFileStates(oldState, newState))
}

func (d *Database) getReferenceResolverLocked(dailyDirectory string, sch *schema.Schema) (*resolver.Resolver, error) {
	dailyDirectory = defaultDailyDir(dailyDirectory)
	schemaKey := resolverSchemaKey(sch)
	if cache := d.referenceResolverCache; cache != nil &&
		cache.dailyDirectory == dailyDirectory &&
		cache.schemaKey == schemaKey {
		if err := d.ensureReferenceResolverCacheCurrentLocked(d.db); err != nil {
			return nil, err
		}
		if d.referenceResolverCache != nil {
			return cache.resolver, nil
		}
	}

	res, generation, err := buildResolverSnapshot(d.db, ResolverOptions{
		DailyDirectory: dailyDirectory,
		Schema:         sch,
	})
	if err != nil {
		return nil, err
	}
	d.referenceResolverCache = &referenceResolverCache{
		resolver:       res,
		dailyDirectory: dailyDirectory,
		schemaKey:      schemaKey,
		schema:         sch,
		generation:     generation,
	}
	d.referenceResolverBuilds++
	return res, nil
}

const resolverGenerationMetaKey = "resolver_generation"

func resolverGeneration(db resolverQuerier) (int64, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, resolverGenerationMetaKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		// BuildResolver also supports the simplified legacy/test databases used
		// by query callers, which may not include Raven's meta table.
		if strings.Contains(err.Error(), "no such table: meta") {
			return 0, nil
		}
		return 0, fmt.Errorf("read resolver generation: %w", err)
	}
	generation, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse resolver generation %q: %w", raw, err)
	}
	return generation, nil
}

func bumpResolverGeneration(tx *sql.Tx) (int64, error) {
	if _, err := tx.Exec(`
		INSERT INTO meta (key, value) VALUES (?, '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1
	`, resolverGenerationMetaKey); err != nil {
		return 0, fmt.Errorf("bump resolver generation: %w", err)
	}
	return resolverGeneration(tx)
}

func loadResolverFileState(tx *sql.Tx, filePath string, sch *schema.Schema) (*resolverFileState, error) {
	state := newResolverFileState()
	typeNameFields := buildTypeNameFields(sch)

	rows, err := tx.Query(`
		SELECT id, type, fields, alias
		FROM objects
		WHERE file_path = ?
		ORDER BY id
	`, filePath)
	if err != nil {
		return nil, fmt.Errorf("load resolver objects for %s: %w", filePath, err)
	}
	for rows.Next() {
		var (
			id, objectType, fieldsJSON string
			alias                      sql.NullString
		)
		if err := rows.Scan(&id, &objectType, &fieldsJSON, &alias); err != nil {
			rows.Close()
			return nil, err
		}
		state.objectIDs[id] = struct{}{}
		if alias.Valid && alias.String != "" {
			addResolverStateMatch(state.aliases, alias.String, id)
		}
		if nameValue, ok := extractNameFieldValue(typeNameFields, objectType, fieldsJSON); ok {
			addResolverStateMatch(state.nameFields, nameValue, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if err := loadResolverIDsForFile(tx, "sections", filePath, state.objectIDs); err != nil {
		return nil, err
	}
	if err := loadResolverIDsForFile(tx, "assets", filePath, state.assetIDs); err != nil {
		return nil, err
	}
	return state, nil
}

func loadResolverIDsForFile(tx *sql.Tx, table, filePath string, ids map[string]struct{}) error {
	var query string
	switch table {
	case "sections":
		query = "SELECT id FROM sections WHERE file_path = ? ORDER BY id"
	case "assets":
		query = "SELECT id FROM assets WHERE file_path = ? ORDER BY id"
	default:
		return fmt.Errorf("unsupported resolver table %q", table)
	}
	rows, err := tx.Query(query, filePath)
	if err != nil {
		return fmt.Errorf("load resolver %s for %s: %w", table, filePath, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids[id] = struct{}{}
	}
	return rows.Err()
}

func addResolverStateMatch(matches map[string]map[string]struct{}, value, id string) {
	ids := matches[value]
	if ids == nil {
		ids = make(map[string]struct{})
		matches[value] = ids
	}
	ids[id] = struct{}{}
}

func diffResolverFileStates(oldState, newState *resolverFileState) resolver.Update {
	return resolver.Update{
		AddedObjectIDs:    addedSetValues(oldState.objectIDs, newState.objectIDs),
		RemovedObjectIDs:  addedSetValues(newState.objectIDs, oldState.objectIDs),
		AddedAssetIDs:     addedSetValues(oldState.assetIDs, newState.assetIDs),
		RemovedAssetIDs:   addedSetValues(newState.assetIDs, oldState.assetIDs),
		AddedAliases:      addedMatchValues(oldState.aliases, newState.aliases),
		RemovedAliases:    addedMatchValues(newState.aliases, oldState.aliases),
		AddedNameFields:   addedMatchValues(oldState.nameFields, newState.nameFields),
		RemovedNameFields: addedMatchValues(newState.nameFields, oldState.nameFields),
	}
}

func addedSetValues(existing, current map[string]struct{}) []string {
	values := make([]string, 0)
	for value := range current {
		if _, ok := existing[value]; !ok {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

func addedMatchValues(existing, current map[string]map[string]struct{}) map[string][]string {
	added := make(map[string][]string)
	for value, currentIDs := range current {
		for id := range currentIDs {
			if existingIDs := existing[value]; existingIDs != nil {
				if _, ok := existingIDs[id]; ok {
					continue
				}
			}
			added[value] = append(added[value], id)
		}
		sort.Strings(added[value])
	}
	return added
}

func (d *Database) hasUnresolvedReferencesLocked(filePath *string) (bool, error) {
	var query string
	var args []any
	if filePath == nil {
		query = `
			SELECT EXISTS(
				SELECT 1 FROM refs WHERE target_id IS NULL
				UNION ALL
				SELECT 1 FROM field_refs WHERE target_id IS NULL
			)
		`
	} else {
		query = `
			SELECT EXISTS(
				SELECT 1 FROM refs WHERE target_id IS NULL AND file_path = ?
				UNION ALL
				SELECT 1 FROM field_refs WHERE target_id IS NULL AND file_path = ?
			)
		`
		args = []any{*filePath, *filePath}
	}
	var exists bool
	if err := d.db.QueryRow(query, args...).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
