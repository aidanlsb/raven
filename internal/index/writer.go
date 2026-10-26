package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type documentWriter struct {
	tx        *sql.Tx
	objStmt   *sql.Stmt
	secStmt   *sql.Stmt
	traitStmt *sql.Stmt
	refStmt   *sql.Stmt
	linkStmt  *sql.Stmt
	dateStmt  *sql.Stmt
	ftsStmt   *sql.Stmt
}

func newDocumentWriter(tx *sql.Tx) (*documentWriter, error) {
	w := &documentWriter{tx: tx}
	var err error
	w.objStmt, err = tx.Prepare(`
		INSERT INTO objects (id, file_path, type, fields, line_start, alias, file_mtime, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, err
	}
	w.secStmt, err = tx.Prepare(`
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		w.close()
		return nil, err
	}
	w.traitStmt, err = tx.Prepare(`
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, value, content, line_number, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		w.close()
		return nil, err
	}
	w.refStmt, err = tx.Prepare(`
		INSERT INTO refs (source_id, target_id, target_raw, display_text, field_name, resolution_status, file_path, line_number, position_start, position_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		w.close()
		return nil, err
	}
	w.linkStmt, err = tx.Prepare(`
		INSERT INTO links (
			source_id, source_type, file_path, line_number, position_start, position_end,
			raw_target, display, is_image, scheme, ext, normalized_key
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		w.close()
		return nil, err
	}
	w.dateStmt, err = tx.Prepare(`
		INSERT OR REPLACE INTO date_index (date, source_type, source_id, field_name, file_path)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		w.close()
		return nil, err
	}
	w.ftsStmt, err = tx.Prepare(`
		INSERT INTO fts_content (object_id, title, content, file_path)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		w.close()
		return nil, err
	}
	return w, nil
}

func (w *documentWriter) close() {
	if w == nil {
		return
	}
	for _, stmt := range []*sql.Stmt{w.objStmt, w.secStmt, w.traitStmt, w.refStmt, w.linkStmt, w.dateStmt, w.ftsStmt} {
		if stmt != nil {
			_ = stmt.Close()
		}
	}
}

func withSuspendedResolverTriggers(tx *sql.Tx, fn func() error) error {
	if _, err := tx.Exec(indexschema.DropResolverGenerationTriggersSQL); err != nil {
		return fmt.Errorf("suspend resolver generation triggers: %w", err)
	}
	if err := fn(); err != nil {
		return err
	}
	if _, err := tx.Exec(indexschema.ResolverGenerationTriggersSQL); err != nil {
		return fmt.Errorf("restore resolver generation triggers: %w", err)
	}
	return nil
}

func (w *documentWriter) write(doc *parser.ParsedDocument, sch *schema.Schema, mtime, now int64, replaceExisting bool) error {
	if replaceExisting {
		if err := deleteByFilePath(w.tx, doc.FilePath); err != nil {
			return err
		}
	}
	if err := indexObjectsWithStmt(w.objStmt, doc, sch, mtime, now); err != nil {
		return err
	}
	if err := indexSectionsWithStmt(w.secStmt, doc, now); err != nil {
		return err
	}
	traits := indexedTraits(doc, sch)
	if err := indexInlineTraitsWithStmt(w.traitStmt, doc, sch, traits, now); err != nil {
		return err
	}
	if err := indexRefsWithStmt(w.refStmt, doc, sch); err != nil {
		return err
	}
	if err := indexLinksWithStmt(w.linkStmt, doc); err != nil {
		return err
	}
	if err := indexDatesWithStmt(w.dateStmt, doc, sch, traits); err != nil {
		return err
	}
	if err := indexFTSWithStmt(w.ftsStmt, doc, sch); err != nil {
		return err
	}
	return upsertFileFingerprint(w.tx, doc.FilePath, mtime, FileContentHash([]byte(doc.RawContent)))
}

func indexObjectsWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument, sch *schema.Schema, mtime, indexedAt int64) error {
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

		var alias *string
		if aliasField, ok := obj.Fields["alias"]; ok {
			if s, ok := aliasField.AsString(); ok && s != "" {
				alias = &s
			}
		}

		_, err = stmt.Exec(
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

func indexSectionsWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument, indexedAt int64) error {
	for _, section := range doc.Sections {
		filePath := section.FilePath
		if filePath == "" {
			filePath = doc.FilePath
		}
		if _, err := stmt.Exec(
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
		); err != nil {
			return err
		}
	}
	return nil
}

func indexInlineTraitsWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument, sch *schema.Schema, traits []indexedTrait, indexedAt int64) error {
	for _, indexedTrait := range traits {
		trait := indexedTrait.Trait

		var valueStr interface{}
		if trait.Value != nil {
			if s := traitValueForIndex(*trait.Value); s != "" {
				valueStr = s
			}
		} else {
			valueStr = getTraitDefault(sch, trait.TraitType)
		}

		if _, execErr := stmt.Exec(
			indexedTrait.ID,
			doc.FilePath,
			trait.ParentScopeID,
			trait.TraitType,
			valueStr,
			trait.Content,
			trait.Line,
			indexedAt,
		); execErr != nil {
			return execErr
		}
	}
	return nil
}

func indexRefsWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument, sch *schema.Schema) error {
	for _, ref := range collectIndexedRefs(doc, sch) {
		if _, err := stmt.Exec(
			ref.sourceID,
			nil,
			ref.targetRaw,
			ref.displayText,
			ref.fieldName,
			resolutionStatusMissing,
			doc.FilePath,
			ref.line,
			ref.positionStart,
			ref.positionEnd,
		); err != nil {
			return err
		}
	}
	return nil
}

func indexLinksWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument) error {
	for _, link := range doc.Links {
		if link == nil {
			continue
		}
		if _, err := stmt.Exec(
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
		); err != nil {
			return err
		}
	}
	return nil
}

func indexDatesWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument, sch *schema.Schema, traits []indexedTrait) error {
	for _, obj := range doc.Objects {
		generatedDate := generatedDateObjectDate(obj)
		if generatedDate != "" {
			if _, err := stmt.Exec(generatedDate, "object", obj.ID, "date", doc.FilePath); err != nil {
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
				if _, err := stmt.Exec(dateStr, "object", obj.ID, fieldName, doc.FilePath); err != nil {
					return err
				}
			}
		}
	}

	for _, indexed := range traits {
		trait := indexed.Trait
		var traitDef *schema.TraitDefinition
		if sch != nil {
			traitDef = sch.Traits[trait.TraitType]
		}
		if trait.Value != nil {
			for _, dateStr := range extractDateStringsForTrait(*trait.Value, traitDef, sch == nil) {
				if _, err := stmt.Exec(dateStr, "trait", indexed.ID, trait.TraitType, doc.FilePath); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func indexFTSWithStmt(stmt *sql.Stmt, doc *parser.ParsedDocument, sch *schema.Schema) error {
	lines := strings.Split(doc.RawContent, "\n")

	for _, obj := range doc.Objects {
		title := objectFTSTitle(obj, sch)
		if _, err := stmt.Exec(obj.ID, title, doc.Body, doc.FilePath); err != nil {
			return err
		}
	}

	for _, section := range doc.Sections {
		content := extractSectionContent(lines, section.LineStart, section.LineEnd)
		if _, err := stmt.Exec(section.ID, section.Title, content, doc.FilePath); err != nil {
			return err
		}
	}
	return nil
}

func objectFTSTitle(obj *model.Object, sch *schema.Schema) string {
	if sch != nil && obj.Type != "" {
		if typeDef, ok := sch.Types[obj.Type]; ok && typeDef.NameField != "" {
			if nameVal, ok := obj.Fields[typeDef.NameField]; ok {
				if s, ok := nameVal.AsString(); ok {
					return s
				}
			}
		}
	}
	if titleField, ok := obj.Fields["title"]; ok {
		if s, ok := titleField.AsString(); ok && s != "" {
			return s
		}
	}
	return obj.ID
}
