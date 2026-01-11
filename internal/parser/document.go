package parser

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

// ParsedDocument represents a fully parsed document.
type ParsedDocument struct {
	FilePath   string          // File path relative to vault
	RawContent string          // Raw markdown content (for full-text search indexing)
	Objects    []*ParsedObject // All objects in this document
	Traits     []*ParsedTrait  // All traits in this document
	Refs       []*ParsedRef    // All references in this document
}

// ParsedObject represents a parsed object (file-level or embedded).
type ParsedObject struct {
	ID           string                       // Unique ID (path for file-level, path#id for embedded)
	ObjectType   string                       // Type name
	Fields       map[string]schema.FieldValue // Fields/metadata
	Heading      *string                      // Heading text (for embedded objects)
	HeadingLevel *int                         // Heading level (for embedded objects)
	ParentID     *string                      // Parent object ID (for embedded objects)
	LineStart    int                          // Line where this object starts
	LineEnd      *int                         // Line where this object ends (embedded only)
}

// ParsedTrait represents a parsed trait annotation.
type ParsedTrait struct {
	TraitType      string             // Trait type name (e.g., "due", "priority", "highlight")
	Value          *schema.FieldValue // Trait value (nil for boolean traits)
	Content        string             // The content the trait annotates
	ParentObjectID string             // Parent object ID
	Line           int                // Line number
}

// HasValue returns true if this trait has a value.
func (t *ParsedTrait) HasValue() bool {
	return t.Value != nil && !t.Value.IsNull()
}

// ValueString returns the value as a string, or empty string if no value.
func (t *ParsedTrait) ValueString() string {
	if t.Value == nil {
		return ""
	}
	if s, ok := t.Value.AsString(); ok {
		return s
	}
	return ""
}

// ParsedRef represents a parsed reference.
type ParsedRef struct {
	SourceID    string  // Source object ID
	TargetRaw   string  // Raw target (as written)
	DisplayText *string // Display text
	Line        int     // Line number
	Start       int     // Start position
	End         int     // End position
}

// ParseOptions contains options for parsing documents.
type ParseOptions struct {
	// ObjectsRoot is the root directory for typed objects (e.g., "objects/").
	// If set, this prefix is stripped from file paths when computing object IDs.
	ObjectsRoot string

	// PagesRoot is the root directory for untyped pages (e.g., "pages/").
	// If set, this prefix is stripped from file paths when computing object IDs.
	PagesRoot string
}

// ParseDocument parses a markdown document.
func ParseDocument(content string, filePath string, vaultPath string) (*ParsedDocument, error) {
	return ParseDocumentWithOptions(content, filePath, vaultPath, nil)
}

// ParseDocumentWithOptions parses a markdown document with custom options.
func ParseDocumentWithOptions(content string, filePath string, vaultPath string, opts *ParseOptions) (*ParsedDocument, error) {
	relativePath := filePath
	if vaultPath != "" {
		if rel, err := filepath.Rel(vaultPath, filePath); err == nil {
			relativePath = rel
		}
	}

	// File ID is derived from the vault-relative file path.
	// This is the canonical path->ID mapping, including directory roots.
	objectsRoot := ""
	pagesRoot := ""
	if opts != nil {
		objectsRoot = opts.ObjectsRoot
		pagesRoot = opts.PagesRoot
	}
	fileID := paths.FilePathToObjectID(relativePath, objectsRoot, pagesRoot)

	var objects []*ParsedObject
	var traits []*ParsedTrait
	var refs []*ParsedRef

	// Parse frontmatter
	frontmatter, err := ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	contentStartLine := 1
	bodyContent := content
	if frontmatter != nil {
		contentStartLine = frontmatter.EndLine + 1
		lines := strings.Split(content, "\n")
		if frontmatter.EndLine < len(lines) {
			bodyContent = strings.Join(lines[frontmatter.EndLine:], "\n")
		} else {
			bodyContent = ""
		}
	}

	// Create file-level object
	fileFields := make(map[string]schema.FieldValue)
	if frontmatter != nil {
		for k, v := range frontmatter.Fields {
			fileFields[k] = v
		}
	}

	fileType := "page"
	if frontmatter != nil && frontmatter.ObjectType != "" {
		fileType = frontmatter.ObjectType
	}

	objects = append(objects, &ParsedObject{
		ID:         fileID,
		ObjectType: fileType,
		Fields:     fileFields,
		LineStart:  1,
	})

	// Extract references from frontmatter, if present.
	//
	// Historically, Raven only extracted refs from the markdown body, which meant
	// wikilinks inside YAML frontmatter were not indexed and therefore missing
	// from `rvn backlinks`. Frontmatter content starts on line 2 (the line after
	// the opening '---') and ends at frontmatter.EndLine-1.
	if frontmatter != nil && frontmatter.Raw != "" {
		fmRefs := ExtractRefs(frontmatter.Raw, 2)
		for _, refItem := range fmRefs {
			refs = append(refs, &ParsedRef{
				SourceID:    fileID,
				TargetRaw:   refItem.TargetRaw,
				DisplayText: refItem.DisplayText,
				Line:        refItem.Line,
				Start:       refItem.Start,
				End:         refItem.End,
			})
		}
	}

	// Use goldmark AST to extract all content from the body.
	// This automatically skips code blocks (fenced, indented, inline).
	astContent, err := ExtractFromAST([]byte(bodyContent), contentStartLine)
	if err != nil {
		return nil, err
	}

	// Use headings and type declarations from AST extraction
	headings := astContent.Headings
	typeDeclLines := astContent.TypeDecls

	// Track used IDs to ensure uniqueness
	usedIDs := make(map[string]int)

	// Parent stack for tracking hierarchy
	type parentEntry struct {
		id    string
		level int
	}
	parentStack := []parentEntry{{id: fileID, level: 0}}

	// Process each heading
	for _, heading := range headings {
		// Pop parents that are at same or deeper level
		for len(parentStack) > 1 && parentStack[len(parentStack)-1].level >= heading.Level {
			parentStack = parentStack[:len(parentStack)-1]
		}
		currentParent := parentStack[len(parentStack)-1].id

		// Check if this heading has an associated type declaration.
		// The AST extraction stores type decls keyed by the heading line number.
		if embedded, ok := typeDeclLines[heading.Line]; ok {
			// Explicit type declaration
			// Use explicit ID if provided, otherwise derive from slugified heading
			var slug string
			if embedded.ID != "" {
				slug = embedded.ID
			} else {
				baseSlug := Slugify(heading.Text)
				if baseSlug == "" {
					baseSlug = embedded.TypeName
				}

				// Ensure unique ID using counter approach
				slug = baseSlug
				usedIDs[baseSlug]++
				if usedIDs[baseSlug] > 1 {
					slug = baseSlug + "-" + strconv.Itoa(usedIDs[baseSlug])
				}
			}

			embeddedID := fileID + "#" + slug
			headingText := heading.Text
			headingLevel := heading.Level

			objects = append(objects, &ParsedObject{
				ID:           embeddedID,
				ObjectType:   embedded.TypeName,
				Fields:       embedded.Fields,
				Heading:      &headingText,
				HeadingLevel: &headingLevel,
				ParentID:     &currentParent,
				LineStart:    heading.Line,
			})

			parentStack = append(parentStack, parentEntry{id: embeddedID, level: heading.Level})
		} else {
			// No type declaration - create a "section" object
			baseSlug := Slugify(heading.Text)
			if baseSlug == "" {
				baseSlug = "section"
			}

			// Ensure unique ID
			slug := baseSlug
			usedIDs[baseSlug]++
			if usedIDs[baseSlug] > 1 {
				slug = baseSlug + "-" + strconv.Itoa(usedIDs[baseSlug])
			}

			sectionID := fileID + "#" + slug
			headingText := heading.Text
			headingLevel := heading.Level

			// Add title and level fields
			fields := map[string]schema.FieldValue{
				"title": schema.String(heading.Text),
				"level": schema.Number(float64(heading.Level)),
			}

			objects = append(objects, &ParsedObject{
				ID:           sectionID,
				ObjectType:   "section",
				Fields:       fields,
				Heading:      &headingText,
				HeadingLevel: &headingLevel,
				ParentID:     &currentParent,
				LineStart:    heading.Line,
			})

			parentStack = append(parentStack, parentEntry{id: sectionID, level: heading.Level})
		}
	}

	// Process traits from AST extraction - assign to the correct parent based on line number
	// Code blocks are already filtered out by the AST walker.
	for _, astTrait := range astContent.Traits {
		parentID := findParentForLine(objects, astTrait.Line)

		traits = append(traits, &ParsedTrait{
			TraitType:      astTrait.TraitName,
			Value:          astTrait.Value,
			Content:        astTrait.Content,
			ParentObjectID: parentID,
			Line:           astTrait.Line,
		})
	}

	// Process references from AST extraction
	// Code blocks are already filtered out by the AST walker.
	for _, astRef := range astContent.Refs {
		parentID := findParentForLine(objects, astRef.Line)

		refs = append(refs, &ParsedRef{
			SourceID:    parentID,
			TargetRaw:   astRef.TargetRaw,
			DisplayText: astRef.DisplayText,
			Line:        astRef.Line,
			Start:       astRef.Start,
			End:         astRef.End,
		})
	}

	// Compute line_end for each object
	computeLineEnds(objects)

	return &ParsedDocument{
		FilePath:   relativePath,
		RawContent: content,
		Objects:    objects,
		Traits:     traits,
		Refs:       refs,
	}, nil
}

// findParentForLine finds the parent object ID for a given line number.
func findParentForLine(objects []*ParsedObject, line int) string {
	var bestMatch *ParsedObject
	for _, obj := range objects {
		if obj.LineStart <= line {
			if bestMatch == nil || obj.LineStart > bestMatch.LineStart {
				bestMatch = obj
			}
		}
	}
	if bestMatch != nil {
		return bestMatch.ID
	}
	if len(objects) > 0 {
		return objects[0].ID
	}
	return ""
}

// computeLineEnds computes LineEnd for each object based on the next object's LineStart.
func computeLineEnds(objects []*ParsedObject) {
	if len(objects) == 0 {
		return
	}

	// Sort by line_start
	indices := make([]int, len(objects))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return objects[indices[i]].LineStart < objects[indices[j]].LineStart
	})

	for i := 0; i < len(indices); i++ {
		currentIdx := indices[i]
		if i+1 < len(indices) {
			nextLineEnd := objects[indices[i+1]].LineStart - 1
			objects[currentIdx].LineEnd = &nextLineEnd
		}
		// Last object extends to end of file (nil)
	}
}

// (directory root stripping is handled by internal/paths)
