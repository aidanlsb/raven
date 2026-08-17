package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// PostCommitReferenceResolutionError reports that the document's index rows
// were committed, but the follow-up reference-resolution pass did not finish.
// Callers must not treat this as a rolled-back index write.
type PostCommitReferenceResolutionError struct {
	FilePath  string
	VaultWide bool
	Err       error
}

func (e *PostCommitReferenceResolutionError) Error() string {
	if e == nil {
		return "index write committed, but reference resolution did not complete"
	}
	scope := "file"
	if e.VaultWide {
		scope = "vault-wide"
	}
	return fmt.Sprintf("index write for %s committed, but %s reference resolution did not complete: %v", e.FilePath, scope, e.Err)
}

func (e *PostCommitReferenceResolutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IndexDocument indexes a parsed document into the database.
// Deprecated: Use IndexDocumentWithMtime for proper staleness tracking.
func (d *Database) IndexDocument(doc *parser.ParsedDocument, sch *schema.Schema) error {
	return d.IndexDocumentWithMtime(doc, sch, 0)
}

// IndexDocumentWithMtime indexes a parsed document with file modification time tracking.
// fileMtime should be the file's modification time as Unix timestamp (seconds).
// Pass 0 if mtime is unknown (will use current time as fallback).
func (d *Database) IndexDocumentWithMtime(doc *parser.ParsedDocument, sch *schema.Schema, fileMtime int64) error {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	d.prepareReferenceResolverCacheLocked(sch)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := d.ensureReferenceResolverCacheCurrentLocked(tx); err != nil {
		return err
	}
	oldResolverState, err := d.writeResolverFileStateLocked(tx, doc.FilePath, sch)
	if err != nil {
		return err
	}

	// Delete existing data for this file
	if err := deleteByFilePath(tx, doc.FilePath); err != nil {
		return err
	}

	now := time.Now().Unix()

	// Use provided mtime or fall back to current time
	mtime := indexedMtime(now, fileMtime)

	if err := indexObjects(tx, doc, sch, mtime, now); err != nil {
		return err
	}
	if err := indexSections(tx, doc, now); err != nil {
		return err
	}
	if err := indexInlineTraits(tx, doc, sch, now); err != nil {
		return err
	}
	if err := indexRefs(tx, doc, sch); err != nil {
		return err
	}
	if err := indexLinks(tx, doc); err != nil {
		return err
	}
	if err := indexFieldRefs(tx, doc, sch); err != nil {
		return err
	}
	if err := indexDates(tx, doc, sch); err != nil {
		return err
	}
	if err := indexFTS(tx, doc, sch); err != nil {
		return err
	}

	newResolverState, err := d.writeResolverFileStateLocked(tx, doc.FilePath, sch)
	if err != nil {
		return err
	}

	resolverGeneration, err := bumpResolverGeneration(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if d.autoResolveRefs && d.dailyDirectory != "" {
		d.updateReferenceResolverCacheLocked(oldResolverState, newResolverState)
		d.setReferenceResolverGenerationLocked(resolverGeneration)
		resolveScope := &doc.FilePath
		if resolverStateAddsCandidates(oldResolverState, newResolverState) {
			// The write introduced new resolution candidates, so refs in
			// other files that previously failed to resolve may now succeed.
			// A vault-wide pass is a cheap existence probe when nothing is
			// unresolved.
			resolveScope = nil
		}
		if _, err := d.resolveReferencesWithSchemaLocked(resolveScope, d.dailyDirectory, sch); err != nil {
			return &PostCommitReferenceResolutionError{
				FilePath:  doc.FilePath,
				VaultWide: resolveScope == nil,
				Err:       err,
			}
		}
	} else {
		// Bulk indexing deliberately keeps the cache cold and builds one resolver
		// from the completed index during the final resolution pass.
		d.referenceResolverCache = nil
	}

	return nil
}

func indexedMtime(now, fileMtime int64) int64 {
	mtime := fileMtime
	if mtime == 0 {
		mtime = now
	}
	return mtime
}

func indexObjects(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema, mtime, indexedAt int64) error {
	objStmt, err := tx.Prepare(`
		INSERT INTO objects (id, file_path, type, fields, line_start, alias, file_mtime, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer objStmt.Close()

	for _, obj := range doc.Objects {
		fields := obj.Fields
		if sch != nil {
			if typeDef := sch.Types[obj.Type]; typeDef != nil {
				fields = schema.IndexableFields(obj.Fields, typeDef.Fields)
			}
		}
		fieldsJSON, err := json.Marshal(fields)
		if err != nil {
			return err
		}

		// Extract alias from fields if present
		var alias *string
		if aliasField, ok := obj.Fields["alias"]; ok {
			if s, ok := aliasField.AsString(); ok && s != "" {
				alias = &s
			}
		}

		_, err = objStmt.Exec(
			obj.ID,
			doc.FilePath,
			obj.Type,
			string(fieldsJSON),
			obj.LineStart,
			alias,
			mtime,
			indexedAt,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// UnknownFrontmatterWarnings returns non-fatal messages for unknown frontmatter
// keys found while indexing. Callers surface these; indexing still proceeds and
// stores only schema-known fields for typed objects.
func UnknownFrontmatterWarnings(doc *parser.ParsedDocument, sch *schema.Schema) []string {
	if doc == nil || sch == nil {
		return nil
	}

	var warnings []string
	for _, obj := range doc.Objects {
		if obj == nil {
			continue
		}
		typeDef := sch.Types[obj.Type]
		if typeDef == nil {
			continue
		}
		for _, key := range schema.UnknownFrontmatterKeys(obj.Fields, typeDef.Fields, nil) {
			warnings = append(warnings, fmt.Sprintf(
				"%s: unknown frontmatter key %q on type %q (not indexed; run rvn check)",
				doc.FilePath, key, obj.Type,
			))
		}
	}
	return warnings
}

func indexSections(tx *sql.Tx, doc *parser.ParsedDocument, indexedAt int64) error {
	stmt, err := tx.Prepare(`
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, section := range doc.Sections {
		filePath := section.FilePath
		if filePath == "" {
			filePath = doc.FilePath
		}
		_, err := stmt.Exec(
			section.ID,
			section.FileObjectID,
			filePath,
			section.Slug,
			section.Title,
			section.Level,
			section.LineStart,
			section.LineEnd,
			section.SubtreeLineEnd,
			section.ParentSectionID,
			indexedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func indexInlineTraits(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema, indexedAt int64) error {
	traitStmt, err := tx.Prepare(`
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer traitStmt.Close()

	for _, indexedTrait := range indexedTraits(doc, sch) {
		trait := indexedTrait.Trait

		// Get value as string, applying schema defaults for bare traits
		var valueStr interface{}
		if trait.Value != nil {
			if s := traitValueForIndex(*trait.Value); s != "" {
				valueStr = s
			}
		} else {
			// Bare trait with no value - check schema for default
			valueStr = getTraitDefault(sch, trait.TraitType)
		}

		_, execErr := traitStmt.Exec(
			indexedTrait.ID,
			doc.FilePath,
			trait.ParentScopeID,
			trait.TraitType,
			valueStr,
			trait.Content,
			trait.Line,
			indexedAt,
		)
		if execErr != nil {
			return execErr
		}
	}

	return nil
}

func traitValueForIndex(value fieldvalue.FieldValue) string {
	return fieldvalue.TraitIndexString(value)
}

type indexedTrait struct {
	ID    string
	Trait *model.Trait
}

func indexedTraits(doc *parser.ParsedDocument, sch *schema.Schema) []indexedTrait {
	indexed := make([]indexedTrait, 0, len(doc.Traits))
	for _, trait := range doc.Traits {
		if sch != nil {
			if _, defined := sch.Traits[trait.TraitType]; !defined {
				continue
			}
		}
		indexed = append(indexed, indexedTrait{
			ID:    fmt.Sprintf("%s:trait:%d", doc.FilePath, len(indexed)),
			Trait: trait,
		})
	}
	return indexed
}

func indexRefs(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	refStmt, err := tx.Prepare(`
		INSERT INTO refs (source_id, target_id, target_raw, display_text, file_path, line_number, position_start, position_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer refStmt.Close()

	// Collect all refs: parsed refs + refs from schema-typed ref fields
	allRefs := doc.Refs

	// Extract additional refs from ref-typed fields in frontmatter.
	// This allows `company: cursor` to work when the schema declares `company: ref`.
	if sch != nil {
		schemaRefs := parser.SchemaFieldRefsAsReferences(parser.ExtractSchemaFieldRefs(doc.Objects, sch))
		allRefs = mergeRefs(allRefs, schemaRefs)
	}

	for _, ref := range allRefs {
		_, err = refStmt.Exec(
			ref.SourceID,
			nil, // target_id resolved later
			ref.TargetRaw,
			ref.DisplayText,
			doc.FilePath,
			ref.LineOrZero(),
			ref.PositionStartOrZero(),
			ref.PositionEndOrZero(),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func indexLinks(tx *sql.Tx, doc *parser.ParsedDocument) error {
	stmt, err := tx.Prepare(`
		INSERT INTO links (
			source_id, source_type, file_path, line_number, position_start, position_end,
			raw_target, display, is_image, scheme, ext, normalized_key
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, link := range doc.Links {
		if link == nil {
			continue
		}
		_, err = stmt.Exec(
			link.SourceID,
			link.SourceType,
			doc.FilePath,
			link.Line,
			link.PositionStart,
			link.PositionEnd,
			link.RawTarget,
			link.Display,
			link.IsImage,
			link.Scheme,
			link.Ext,
			link.NormalizedKey,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func indexFieldRefs(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	if sch == nil {
		return nil
	}

	fieldRefs := parser.ExtractSchemaFieldRefs(doc.Objects, sch)
	if len(fieldRefs) == 0 {
		return nil
	}

	stmt, err := tx.Prepare(`
		INSERT INTO field_refs (source_id, field_name, target_raw, target_id, resolution_status, file_path, line_number)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range fieldRefs {
		if ref.TargetRaw == "" {
			continue
		}
		_, err = stmt.Exec(
			ref.SourceID,
			ref.FieldName,
			ref.TargetRaw,
			nil,
			"missing",
			doc.FilePath,
			ref.Line,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func indexDates(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	dateStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO date_index (date, source_type, source_id, field_name, file_path)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer dateStmt.Close()

	for _, obj := range doc.Objects {
		generatedDate := generatedDateObjectDate(obj)
		if generatedDate != "" {
			_, err = dateStmt.Exec(generatedDate, "object", obj.ID, "date", doc.FilePath)
			if err != nil {
				return err
			}
		}

		var fieldDefs map[string]*schema.FieldDefinition
		if sch != nil {
			if typeDef := sch.Types[obj.Type]; typeDef != nil {
				fieldDefs = typeDef.Fields
			}
		}
		for fieldName, fieldValue := range obj.Fields {
			if obj.Type == "date" && fieldName == "date" {
				continue
			}
			for _, dateStr := range extractDateStringsForField(fieldValue, fieldDefs[fieldName], sch == nil) {
				_, err = dateStmt.Exec(dateStr, "object", obj.ID, fieldName, doc.FilePath)
				if err != nil {
					return err
				}
			}
		}
	}

	for _, indexedTrait := range indexedTraits(doc, sch) {
		trait := indexedTrait.Trait
		var traitDef *schema.TraitDefinition
		if sch != nil {
			traitDef = sch.Traits[trait.TraitType]
		}
		if trait.Value != nil {
			for _, dateStr := range extractDateStringsForTrait(*trait.Value, traitDef, sch == nil) {
				_, err = dateStmt.Exec(dateStr, "trait", indexedTrait.ID, trait.TraitType, doc.FilePath)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func extractDateStringsForField(fv fieldvalue.FieldValue, def *schema.FieldDefinition, allowHeuristic bool) []string {
	if def == nil {
		if allowHeuristic {
			return oneDateString(extractDateString(fv))
		}
		return nil
	}

	switch def.Type {
	case schema.FieldTypeDate, schema.FieldTypeDatetime:
		return oneDateString(extractDateString(fv))
	case schema.FieldTypeDateArray, schema.FieldTypeDatetimeArray:
		return extractDateStringsFromArray(fv, extractDateString)
	case schema.FieldTypeRef:
		if def.Target == "date" {
			return oneDateString(extractDateRefString(fv))
		}
	case schema.FieldTypeRefArray:
		if def.Target == "date" {
			return extractDateStringsFromArray(fv, extractDateRefString)
		}
	}
	return nil
}

func extractDateStringsForTrait(fv fieldvalue.FieldValue, def *schema.TraitDefinition, allowHeuristic bool) []string {
	if def == nil {
		if allowHeuristic {
			return oneDateString(extractDateString(fv))
		}
		return nil
	}
	switch def.Type {
	case schema.FieldTypeDate, schema.FieldTypeDatetime:
		return oneDateString(extractDateString(fv))
	case schema.FieldTypeDateArray, schema.FieldTypeDatetimeArray:
		return extractDateStringsFromArray(fv, extractDateString)
	}
	return nil
}

func extractDateStringsFromArray(fv fieldvalue.FieldValue, extract func(fieldvalue.FieldValue) string) []string {
	arr, ok := fv.AsArray()
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if dateStr := extract(item); dateStr != "" {
			out = append(out, dateStr)
		}
	}
	return out
}

func oneDateString(dateStr string) []string {
	if dateStr == "" {
		return nil
	}
	return []string{dateStr}
}

func extractDateRefString(fv fieldvalue.FieldValue) string {
	raw, ok := fv.AsString()
	if !ok {
		return ""
	}
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(raw, "]]"), "[["))
	if dates.IsValidDate(raw) {
		return raw
	}
	if idx := strings.LastIndex(raw, "/"); idx >= 0 {
		candidate := raw[idx+1:]
		if dates.IsValidDate(candidate) {
			return candidate
		}
	}
	return ""
}

func indexFTS(tx *sql.Tx, doc *parser.ParsedDocument, sch *schema.Schema) error {
	ftsStmt, err := tx.Prepare(`
		INSERT INTO fts_content (object_id, title, content, file_path)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	// Pre-split content into lines for section extraction
	lines := strings.Split(doc.RawContent, "\n")

	for _, obj := range doc.Objects {
		// Get title: check schema name_field first, then "title" field, then object ID
		title := ""
		if sch != nil && obj.Type != "" {
			if typeDef, ok := sch.Types[obj.Type]; ok && typeDef.NameField != "" {
				if nameVal, ok := obj.Fields[typeDef.NameField]; ok {
					if s, ok := nameVal.AsString(); ok {
						title = s
					}
				}
			}
		}
		if title == "" {
			if titleField, ok := obj.Fields["title"]; ok {
				if s, ok := titleField.AsString(); ok {
					title = s
				}
			}
		}
		if title == "" {
			title = obj.ID
		}

		_, err = ftsStmt.Exec(obj.ID, title, doc.Body, doc.FilePath)
		if err != nil {
			return err
		}
	}

	for _, section := range doc.Sections {
		content := extractSectionContent(lines, section.LineStart, section.LineEnd)
		_, err = ftsStmt.Exec(section.ID, section.Title, content, doc.FilePath)
		if err != nil {
			return err
		}
	}

	return nil
}

// extractSectionContent extracts direct content for a section from the given line range.
// lineStart and lineEnd are 1-indexed. If lineEnd is nil, extracts to end of file.
func extractSectionContent(lines []string, lineStart int, lineEnd *int) string {
	if lineStart < 1 || lineStart > len(lines) {
		return ""
	}

	// Convert to 0-indexed
	start := lineStart - 1
	end := len(lines)
	if lineEnd != nil && *lineEnd <= len(lines) {
		end = *lineEnd
	}

	if start >= end {
		return ""
	}

	return strings.Join(lines[start:end], "\n")
}

func generatedDateObjectDate(obj *model.Object) string {
	if obj == nil || obj.Type != "date" {
		return ""
	}

	candidate := obj.ID
	if idx := strings.LastIndex(candidate, "/"); idx >= 0 {
		candidate = candidate[idx+1:]
	}
	if dates.IsValidDate(candidate) {
		return candidate
	}
	return ""
}

// extractDateString extracts a date string from a field value if it's a date type.
// Only extracts absolute dates in YYYY-MM-DD format.
// Relative keywords (today, tomorrow, etc.) are NOT resolved here because the
// resolved value would become stale on reindex. Instead, relative dates are
// handled at query time.
// Returns empty string if not a date.
func extractDateString(fv fieldvalue.FieldValue) string {
	if s, ok := fv.AsString(); ok {
		if len(s) >= 10 {
			candidate := s[:10] // Return just the date part (in case of datetime)
			if dates.IsValidDate(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// getTraitDefault returns the default value for a trait from the schema.
// For boolean traits with default: true, returns "true".
// For other traits, returns the default value as a string, or nil if no default.
func getTraitDefault(sch *schema.Schema, traitType string) interface{} {
	if sch == nil {
		return nil
	}

	traitDef, exists := sch.Traits[traitType]
	if !exists || traitDef == nil {
		return nil
	}

	// If no default is defined, return nil
	if traitDef.Default == nil {
		// For boolean traits without explicit default, the presence of the trait
		// implies "true" - this is the expected UX for bare boolean traits
		if traitDef.IsBoolean() {
			return "true"
		}
		return nil
	}

	defaultValue := parser.FieldValueFromYAML(traitDef.Default)
	if s := traitValueForIndex(defaultValue); s != "" {
		return s
	}
	return fmt.Sprintf("%v", traitDef.Default)
}

// mergeRefs merges two ref slices, deduplicating by (sourceID, targetRaw) pairs.
// This prevents double-indexing when a ref is both:
// 1. Found by raw YAML scanning (as [[target]])
// 2. Extracted from a ref-typed field
func mergeRefs(existing, additional []*model.Reference) []*model.Reference {
	// Build a set of existing (sourceID, targetRaw) pairs
	seen := make(map[string]bool)
	for _, ref := range existing {
		key := ref.SourceID + "\x00" + ref.TargetRaw
		seen[key] = true
	}

	// Add new refs that aren't duplicates
	result := existing
	for _, ref := range additional {
		key := ref.SourceID + "\x00" + ref.TargetRaw
		if !seen[key] {
			result = append(result, ref)
			seen[key] = true
		}
	}

	return result
}
