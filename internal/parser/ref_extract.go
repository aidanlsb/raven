package parser

import (
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// RefExtractOptions controls how refs are extracted from a FieldValue.
type RefExtractOptions struct {
	// AllowBareStrings treats plain strings as ref targets.
	AllowBareStrings bool
	// AllowWikilinksInString scans string values for wikilink references.
	AllowWikilinksInString bool
	// AllowTripleBrackets passes allowTriple=true to the wikilink parser.
	AllowTripleBrackets bool
}

// ExtractedRef represents a resolved ref target and optional display text.
// This is a lightweight extraction intermediate; document-level refs use model.Reference.
type ExtractedRef struct {
	TargetRaw   string
	DisplayText *string
}

// SchemaFieldRef is a ref discovered from a schema-typed frontmatter field.
type SchemaFieldRef struct {
	SourceID  string
	FieldName string
	TargetRaw string
	Line      int
}

// ExtractRefsFromFieldValue extracts refs from a FieldValue using the provided options.
func ExtractRefsFromFieldValue(fv fieldvalue.FieldValue, opts RefExtractOptions) []ExtractedRef {
	var refs []ExtractedRef

	if target, ok := fv.AsRef(); ok {
		return []ExtractedRef{{TargetRaw: target}}
	}

	if arr, ok := fv.AsArray(); ok {
		for _, item := range arr {
			refs = append(refs, ExtractRefsFromFieldValue(item, opts)...)
		}
		return refs
	}

	if s, ok := fv.AsString(); ok {
		if opts.AllowWikilinksInString {
			matches := wikilink.FindAllInLine(s, opts.AllowTripleBrackets)
			for _, match := range matches {
				refs = append(refs, ExtractedRef{
					TargetRaw:   match.Target,
					DisplayText: match.DisplayText,
				})
			}
		}
		if opts.AllowBareStrings && s != "" && !strings.Contains(s, "[[") {
			refs = append(refs, ExtractedRef{TargetRaw: s})
		}
	}

	return refs
}

// ExtractSchemaFieldRefs extracts refs from ref / ref[] typed object fields.
//
// The parser is schema-blind at parse time, so bare strings like `company: cursor`
// stay strings until a schema-aware caller (index, check) runs this helper.
func ExtractSchemaFieldRefs(objects []*model.Object, sch *schema.Schema) []SchemaFieldRef {
	if sch == nil {
		return nil
	}

	var refs []SchemaFieldRef
	opts := RefExtractOptions{AllowBareStrings: true}

	for _, obj := range objects {
		if obj == nil {
			continue
		}
		typeDef := sch.Types[obj.Type]
		if typeDef == nil {
			continue
		}

		for fieldName, fieldValue := range obj.Fields {
			fieldDef := typeDef.Fields[fieldName]
			if fieldDef == nil {
				continue
			}

			switch fieldDef.Type {
			case schema.FieldTypeRef:
				if targets := ExtractRefsFromFieldValue(fieldValue, opts); len(targets) > 0 {
					if targets[0].TargetRaw == "" {
						continue
					}
					refs = append(refs, SchemaFieldRef{
						SourceID:  obj.ID,
						FieldName: fieldName,
						TargetRaw: targets[0].TargetRaw,
						Line:      obj.LineStart,
					})
				}

			case schema.FieldTypeRefArray:
				for _, target := range ExtractRefsFromFieldValue(fieldValue, opts) {
					if target.TargetRaw == "" {
						continue
					}
					refs = append(refs, SchemaFieldRef{
						SourceID:  obj.ID,
						FieldName: fieldName,
						TargetRaw: target.TargetRaw,
						Line:      obj.LineStart,
					})
				}
			}
		}
	}

	return refs
}

// SchemaFieldRefsAsReferences converts schema field refs into model.Reference values.
func SchemaFieldRefsAsReferences(schemaRefs []SchemaFieldRef) []*model.Reference {
	if len(schemaRefs) == 0 {
		return nil
	}
	refs := make([]*model.Reference, 0, len(schemaRefs))
	for _, schemaRef := range schemaRefs {
		refs = append(refs, model.NewInlineReference(
			schemaRef.SourceID,
			schemaRef.TargetRaw,
			nil,
			schemaRef.Line,
			0,
			0,
		))
	}
	return refs
}
