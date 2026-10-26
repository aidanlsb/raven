package index

import (
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
	return d.indexDocumentsLocked([]DocumentIndexInput{{Document: doc, FileMtime: fileMtime}}, sch, IndexDocumentsOptions{})
}

// DocumentIndexInput is one parsed vault file ready to write into the index.
type DocumentIndexInput struct {
	Document  *parser.ParsedDocument
	FileMtime int64
}

// IndexDocumentsOptions controls a batched index write.
type IndexDocumentsOptions struct {
	// SkipExistingDelete skips DELETE-by-file-path before insert. Full reindex
	// uses this after ClearAllData so empty deletes are not issued per file.
	SkipExistingDelete bool
}

// IndexDocuments writes many parsed documents in one SQLite transaction with
// prepared statements reused across files. Resolver-generation triggers are
// suspended for the transaction and generation is bumped once at the end.
func (d *Database) IndexDocuments(docs []DocumentIndexInput, sch *schema.Schema, opts IndexDocumentsOptions) error {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()
	return d.indexDocumentsLocked(docs, sch, opts)
}

func (d *Database) indexDocumentsLocked(docs []DocumentIndexInput, sch *schema.Schema, opts IndexDocumentsOptions) error {
	if len(docs) == 0 {
		return nil
	}

	d.prepareReferenceResolverCacheLocked(sch)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := d.ensureReferenceResolverCacheCurrentLocked(tx); err != nil {
		return err
	}

	singleFile := len(docs) == 1
	var oldResolverState *resolverFileState
	if singleFile {
		oldResolverState, err = d.writeResolverFileStateLocked(tx, docs[0].Document.FilePath, sch)
		if err != nil {
			return err
		}
	}

	now := time.Now().Unix()
	replaceExisting := !opts.SkipExistingDelete
	if err := withSuspendedResolverTriggers(tx, func() error {
		writer, err := newDocumentWriter(tx)
		if err != nil {
			return err
		}
		defer writer.close()
		for _, input := range docs {
			if input.Document == nil {
				continue
			}
			mtime := indexedMtime(now, input.FileMtime)
			if err := writer.write(input.Document, sch, mtime, now, replaceExisting); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := bumpRevision(tx); err != nil {
		return err
	}

	var newResolverState *resolverFileState
	if singleFile {
		newResolverState, err = d.writeResolverFileStateLocked(tx, docs[0].Document.FilePath, sch)
		if err != nil {
			return err
		}
	}

	resolverGeneration, err := bumpResolverGeneration(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if d.autoResolveRefs && d.dailyDirectory != "" {
		if singleFile {
			d.updateReferenceResolverCacheLocked(oldResolverState, newResolverState)
			d.setReferenceResolverGenerationLocked(resolverGeneration)
			resolveScope := &docs[0].Document.FilePath
			if resolverStateAddsCandidates(oldResolverState, newResolverState) {
				resolveScope = nil
			}
			if _, err := d.resolveReferencesWithSchemaLocked(resolveScope, d.dailyDirectory, sch); err != nil {
				return &PostCommitReferenceResolutionError{
					FilePath:  docs[0].Document.FilePath,
					VaultWide: resolveScope == nil,
					Err:       err,
				}
			}
			return nil
		}
		d.referenceResolverCache = nil
		if _, err := d.resolveReferencesWithSchemaLocked(nil, d.dailyDirectory, sch); err != nil {
			return &PostCommitReferenceResolutionError{
				FilePath:  docs[0].Document.FilePath,
				VaultWide: true,
				Err:       err,
			}
		}
		return nil
	}

	// Bulk indexing deliberately keeps the cache cold and builds one resolver
	// from the completed index during the final resolution pass.
	d.referenceResolverCache = nil
	return nil
}

func indexedMtime(now, fileMtime int64) int64 {
	mtime := fileMtime
	if mtime == 0 {
		mtime = now
	}
	return mtime
}

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

type indexedRef struct {
	sourceID      string
	targetRaw     string
	displayText   *string
	fieldName     *string
	line          int
	positionStart int
	positionEnd   int
}

func collectIndexedRefs(doc *parser.ParsedDocument, sch *schema.Schema) []indexedRef {
	schemaRefs := parser.ExtractSchemaFieldRefs(doc.Objects, sch)
	remainingByKey := make(map[string][]int, len(schemaRefs))
	used := make([]bool, len(schemaRefs))
	for i, sr := range schemaRefs {
		if sr.TargetRaw == "" {
			used[i] = true
			continue
		}
		key := sr.SourceID + "\x00" + sr.TargetRaw
		remainingByKey[key] = append(remainingByKey[key], i)
	}

	fmStart, fmEnd, hasFrontmatter := frontmatterContentLineRangeFromDoc(doc)
	out := make([]indexedRef, 0, len(doc.Refs)+len(schemaRefs))

	for _, ref := range doc.Refs {
		if ref == nil {
			continue
		}
		var fieldName *string
		key := ref.SourceID + "\x00" + ref.TargetRaw
		line := ref.LineOrZero()
		if hasFrontmatter && line >= fmStart && line <= fmEnd {
			if idxs := remainingByKey[key]; len(idxs) > 0 {
				i := idxs[0]
				remainingByKey[key] = idxs[1:]
				used[i] = true
				name := schemaRefs[i].FieldName
				fieldName = &name
			}
		}
		out = append(out, indexedRef{
			sourceID:      ref.SourceID,
			targetRaw:     ref.TargetRaw,
			displayText:   ref.DisplayText,
			fieldName:     fieldName,
			line:          line,
			positionStart: ref.PositionStartOrZero(),
			positionEnd:   ref.PositionEndOrZero(),
		})
	}

	for i, sr := range schemaRefs {
		if used[i] {
			continue
		}
		name := sr.FieldName
		out = append(out, indexedRef{
			sourceID:  sr.SourceID,
			targetRaw: sr.TargetRaw,
			fieldName: &name,
			line:      sr.Line,
		})
	}

	return out
}

func frontmatterContentLineRangeFromDoc(doc *parser.ParsedDocument) (start, end int, ok bool) {
	if doc == nil || !doc.HasFrontmatter {
		return frontmatterContentLineRange(docRawContent(doc))
	}
	start = 2
	end = doc.FrontmatterEndLine - 1
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

func docRawContent(doc *parser.ParsedDocument) string {
	if doc == nil {
		return ""
	}
	return doc.RawContent
}

func frontmatterContentLineRange(raw string) (start, end int, ok bool) {
	if raw == "" {
		return 0, 0, false
	}
	fm, err := parser.ParseFrontmatter(raw)
	if err != nil || fm == nil || fm.Raw == "" {
		return 0, 0, false
	}
	start = 2
	end = fm.EndLine - 1
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

func extractDateStringsForField(fv fieldvalue.FieldValue, def *schema.FieldDefinition, allowHeuristic bool) []string {
	if def == nil {
		if allowHeuristic {
			return oneDateString(extractDateString(fv))
		}
		return nil
	}

	switch {
	case def.Type == schema.FieldTypeDate || def.Type == schema.FieldTypeDatetime:
		return oneDateString(extractDateString(fv))
	case def.Type.IsArray():
		elem, _ := def.Type.ElementType()
		if elem == schema.FieldTypeDate || elem == schema.FieldTypeDatetime {
			return extractDateStringsFromArray(fv, extractDateString)
		}
		if def.Type.IsRef() && def.Target == "date" {
			return extractDateStringsFromArray(fv, extractDateRefString)
		}
	case def.Type.IsRef() && def.Target == "date":
		return oneDateString(extractDateRefString(fv))
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
	switch {
	case def.Type == schema.FieldTypeDate || def.Type == schema.FieldTypeDatetime:
		return oneDateString(extractDateString(fv))
	case def.Type.IsArray():
		elem, _ := def.Type.ElementType()
		if elem == schema.FieldTypeDate || elem == schema.FieldTypeDatetime {
			return extractDateStringsFromArray(fv, extractDateString)
		}
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
