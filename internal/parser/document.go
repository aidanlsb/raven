package parser

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ravenscroftj/raven/internal/schema"
)

// ParsedDocument represents a fully parsed document.
type ParsedDocument struct {
	FilePath string          // File path relative to vault
	Objects  []*ParsedObject // All objects in this document
	Traits   []*ParsedTrait  // All traits in this document
	Refs     []*ParsedRef    // All references in this document
}

// ParsedObject represents a parsed object (file-level or embedded).
type ParsedObject struct {
	ID           string                       // Unique ID (path for file-level, path#id for embedded)
	ObjectType   string                       // Type name
	Fields       map[string]schema.FieldValue // Fields/metadata
	Tags         []string                     // Tags
	Heading      *string                      // Heading text (for embedded objects)
	HeadingLevel *int                         // Heading level (for embedded objects)
	ParentID     *string                      // Parent object ID (for embedded objects)
	LineStart    int                          // Line where this object starts
	LineEnd      *int                         // Line where this object ends (embedded only)
}

// ParsedTrait represents a parsed trait annotation.
type ParsedTrait struct {
	TraitType      string              // Trait type name (e.g., "due", "priority", "highlight")
	Value          *schema.FieldValue  // Trait value (nil for boolean traits)
	Content        string              // The content the trait annotates
	ParentObjectID string              // Parent object ID
	Line           int                 // Line number
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

// ParseDocument parses a markdown document.
func ParseDocument(content string, filePath string, vaultPath string) (*ParsedDocument, error) {
	relativePath := filePath
	if vaultPath != "" {
		if rel, err := filepath.Rel(vaultPath, filePath); err == nil {
			relativePath = rel
		}
	}

	// File ID is path without extension
	fileID := strings.TrimSuffix(relativePath, ".md")
	fileID = strings.TrimSuffix(fileID, filepath.Ext(relativePath))
	// Normalize path separators
	fileID = filepath.ToSlash(fileID)

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

	var fileTags []string
	if frontmatter != nil {
		fileTags = append(fileTags, frontmatter.Tags...)
	}

	// Add inline tags from body
	inlineTags := ExtractInlineTags(bodyContent)
	for _, tag := range inlineTags {
		found := false
		for _, existing := range fileTags {
			if existing == tag {
				found = true
				break
			}
		}
		if !found {
			fileTags = append(fileTags, tag)
		}
	}

	// Store tags in fields as well
	if len(fileTags) > 0 {
		tagValues := make([]schema.FieldValue, len(fileTags))
		for i, t := range fileTags {
			tagValues[i] = schema.String(t)
		}
		fileFields["tags"] = schema.Array(tagValues)
	}

	objects = append(objects, &ParsedObject{
		ID:         fileID,
		ObjectType: fileType,
		Fields:     fileFields,
		Tags:       fileTags,
		LineStart:  1,
	})

	// Extract all headings from the body
	headings := ExtractHeadings(bodyContent, contentStartLine)

	// Build a map of line -> type declaration for quick lookup
	bodyLines := strings.Split(bodyContent, "\n")
	typeDeclLines := make(map[int]*EmbeddedTypeInfo)

	for lineOffset, line := range bodyLines {
		lineNum := contentStartLine + lineOffset
		if embedded := ParseEmbeddedType(line, lineNum); embedded != nil {
			typeDeclLines[lineNum] = embedded
		}
	}

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
		// Check if the line after this heading has a type declaration
		nextLine := heading.Line + 1

		// Pop parents that are at same or deeper level
		for len(parentStack) > 1 && parentStack[len(parentStack)-1].level >= heading.Level {
			parentStack = parentStack[:len(parentStack)-1]
		}
		currentParent := parentStack[len(parentStack)-1].id

		if embedded, ok := typeDeclLines[nextLine]; ok {
			// Explicit type declaration
			embeddedID := fileID + "#" + embedded.ID
			headingText := heading.Text
			headingLevel := heading.Level

			objects = append(objects, &ParsedObject{
				ID:           embeddedID,
				ObjectType:   embedded.TypeName,
				Fields:       embedded.Fields,
				Tags:         embedded.Tags,
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
				Tags:         []string{},
				Heading:      &headingText,
				HeadingLevel: &headingLevel,
				ParentID:     &currentParent,
				LineStart:    heading.Line,
			})

			parentStack = append(parentStack, parentEntry{id: sectionID, level: heading.Level})
		}
	}

	// Process traits - assign to the correct parent based on line number
	for lineOffset, line := range bodyLines {
		lineNum := contentStartLine + lineOffset

		// Parse ALL traits on this line
		parsedTraits := ParseTraitAnnotations(line, lineNum)
		if len(parsedTraits) > 0 {
			parentID := findParentForLine(objects, lineNum)

			// Get the shared content (text after all traits)
			// Use the content from the last trait (everything after the last @)
			sharedContent := ExtractTraitContent(bodyLines, lineOffset)

			for _, parsedTrait := range parsedTraits {
				traits = append(traits, &ParsedTrait{
					TraitType:      parsedTrait.TraitName,
					Value:          parsedTrait.Value,
					Content:        sharedContent,
					ParentObjectID: parentID,
					Line:           lineNum,
				})
			}
		}
	}

	// Extract all references from body
	bodyRefs := ExtractRefs(bodyContent, contentStartLine)
	for _, refItem := range bodyRefs {
		parentID := findParentForLine(objects, refItem.Line)

		refs = append(refs, &ParsedRef{
			SourceID:    parentID,
			TargetRaw:   refItem.TargetRaw,
			DisplayText: refItem.DisplayText,
			Line:        refItem.Line,
			Start:       refItem.Start,
			End:         refItem.End,
		})
	}

	// Compute line_end for each object
	computeLineEnds(objects)

	return &ParsedDocument{
		FilePath: relativePath,
		Objects:  objects,
		Traits:   traits,
		Refs:     refs,
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
