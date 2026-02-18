# Parsing Architecture

This document explains how Raven parses markdown files into structured data.

## Overview

Raven uses a **goldmark-first** parsing approach:

1. **YAML frontmatter** is parsed first (before markdown processing)
2. **Goldmark** parses the markdown body into an AST
3. **AST walking** extracts Raven-specific syntax (traits, refs, type declarations)
4. **Object building** creates the final document structure

This approach leverages goldmark (a CommonMark-compliant parser) for all standard markdown constructs, with custom parsing only for Raven-specific syntax.

## Parsing Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Raw File Content                          │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                     1. Frontmatter Parsing                       │
│                                                                  │
│  - Detect --- delimiters                                        │
│  - Parse YAML content                                           │
│  - Extract type, fields, references                             │
│  - Calculate body start line                                    │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    2. Goldmark AST Parsing                       │
│                                                                  │
│  - Parse body content with goldmark.New().Parser().Parse()      │
│  - Produces AST with nodes like:                                │
│    • *ast.Heading                                               │
│    • *ast.Paragraph                                             │
│    • *ast.ListItem                                              │
│    • *ast.FencedCodeBlock                                       │
│    • *ast.CodeBlock (indented)                                  │
│    • *ast.CodeSpan (inline)                                     │
│    • *ast.Text                                                  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                   3. AST Walking (ExtractFromAST)                │
│                                                                  │
│  First pass:                                                    │
│  - Extract headings from *ast.Heading nodes                     │
│  - Detect ::type() declarations in next sibling paragraphs     │
│                                                                  │
│  Second pass:                                                   │
│  - Skip *ast.FencedCodeBlock, *ast.CodeBlock, *ast.CodeSpan    │
│  - Collect text from *ast.Paragraph and *ast.ListItem          │
│  - Parse @traits and [[refs]] from collected text              │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    4. Object Building                            │
│                                                                  │
│  - Create file-level object from frontmatter                    │
│  - Create section/embedded objects from headings                │
│  - Associate traits and refs with parent objects                │
│  - Compute line ranges for each object                          │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                       ParsedDocument                             │
│                                                                  │
│  - FilePath: vault-relative path                                │
│  - RawContent: original markdown                                │
│  - Objects: []*ParsedObject                                     │
│  - Traits: []*ParsedTrait                                       │
│  - Refs: []*ParsedRef                                           │
└─────────────────────────────────────────────────────────────────┘
```

## Syntax Elements

### 1. YAML Frontmatter

**Location:** `internal/parser/frontmatter.go`

Frontmatter is parsed before any markdown processing:

```yaml
---
type: project
title: Website Redesign
owner: people/freya
tags: [web, frontend]
---
```

**Parsing steps:**
1. Detect opening `---` on first line
2. Find closing `---`
3. Parse content as YAML using `gopkg.in/yaml.v3`
4. Convert YAML values to `schema.FieldValue` types
5. Extract `type` field specially (defaults to `page`)
6. Record `EndLine` for body offset calculation

**Special handling:**
- Wikilinks in string values (e.g., `owner: "[[people/freya]]"`) are detected and converted to refs
- Dates are parsed from YAML's native date type
- Arrays are preserved as `schema.Array`

### 2. Headings → Objects

**Location:** `internal/parser/ast.go` (extraction), `internal/parser/document.go` (object building)

Every markdown heading creates an object:

```markdown
# Main Title          → file-level object (from frontmatter)
## Overview           → section object
## Tasks              
::section()           → section object (explicit type)
## Weekly Standup
::meeting(time=09:00) → meeting object (explicit type)
```

**Extraction (AST):**
1. Walk AST for `*ast.Heading` nodes
2. Concatenate text from child `*ast.Text` nodes
3. Calculate line number from byte offset
4. Check next sibling for type declaration

**Object building:**
- If heading has `::type()` on next line → create typed object
- Otherwise → create `section` object with `title` and `level` fields
- ID is `<file-id>#<slug>` where slug is derived from heading text
- Parent is determined by heading level hierarchy

### 3. Embedded Type Declarations

**Location:** `internal/parser/typedecl.go` (parsing), `internal/parser/ast.go` (detection)

Type declarations explicitly set an object's type:

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]], [[people/thor]]])
```

**Detection (AST-based):**
1. When processing a heading, check `heading.NextSibling()`
2. If sibling is `*ast.Paragraph`, get first `*ast.Text` child
3. If text starts with `::`, parse as type declaration
4. Mark paragraph as "consumed" (skip for trait/ref extraction)

**Parsing:**
- Regex: `^::(\w+)\s*\(([^)]*)\)\s*$`
- Arguments parsed with state machine handling nested brackets and quotes
- Values converted to appropriate types (ref, array, date, etc.)

**Field value syntax:**
| Type | Syntax | Example |
|------|--------|---------|
| String | bare or quoted | `title=Hello`, `title="Hello, World"` |
| Number | bare | `priority=3` |
| Boolean | `true`/`false` | `active=true` |
| Date | YYYY-MM-DD | `due=2026-02-15` |
| Reference | `[[id]]` | `owner=[[people/freya]]` |
| Array | `[item, item]` | `tags=[web, frontend]` |
| Ref Array | `[[[id]], [[id]]]` | `attendees=[[[alice]], [[bob]]]` |

### 4. Traits

**Location:** `internal/parser/traits.go` (parsing), `internal/parser/ast.go` (extraction)

Traits are inline annotations:

```markdown
- @todo Buy groceries
- @due(2026-02-15) Finish report
- @priority(high) @highlight Important task
```

**Extraction (AST-based):**
1. Walk AST, skip code nodes (`FencedCodeBlock`, `CodeBlock`, `CodeSpan`)
2. For `Paragraph` and `ListItem` nodes, collect text from child `Text` nodes
3. Apply trait regex to collected text

**Regex:** `(?:^|[\s\-\*])@(\w+)(?:\s*\(([^)]*)\))?`

This matches:
- `@name` - boolean trait (no value)
- `@name(value)` - trait with value
- Must be preceded by start of line, whitespace, or list markers

**Value parsing:**
- `[[ref]]` → reference
- `YYYY-MM-DD` → date
- `YYYY-MM-DDTHH:MM` → datetime
- Everything else → string (includes enum values)

### 5. References (Wikilinks)

**Location:** `internal/wikilink/wikilink.go` (parsing), `internal/parser/ast.go` (extraction)

References link to other objects:

```markdown
See [[people/freya]] for details.
Contact [[people/freya|Freya]] about this.
```

**Extraction (AST-based):**
1. Same AST walk as traits (skip code nodes)
2. Collect text at paragraph/list-item level
3. Apply wikilink regex to find `[[target]]` or `[[target|display]]`

**Why paragraph-level collection?**

Goldmark splits text at `[` characters (potential link syntax), so `[[target]]` becomes multiple `Text` nodes:
- `"See ["`
- `"["`
- `"target]"`
- `"] for details."`

By collecting all text from a paragraph first, we reconstruct the full wikilink for regex matching.

**Regex:** `\[\[([^\]\[|]+)(?:\|([^\]]+))?\]\]`

## Code Block Handling

One of the key benefits of goldmark-first parsing is automatic code block handling.

### Fenced Code Blocks

````markdown
```python
@decorator  # NOT parsed as trait
def foo(): pass
```
````

Goldmark creates `*ast.FencedCodeBlock` nodes which we skip entirely.

### Indented Code Blocks

```markdown
Regular text with @trait

    @indented  # NOT parsed as trait
    code here

More text with @another
```

Goldmark creates `*ast.CodeBlock` nodes for 4-space indented content.

### Inline Code

```markdown
Use `@decorator` for Python decorators. @todo Real trait
```

Goldmark creates `*ast.CodeSpan` nodes. When collecting text from paragraphs, we skip these nodes, so only `@todo` is found.

### Code in List Items

```markdown
- @todo Real task
- ```
  @fake  # NOT parsed
  ```
- @done Another task
```

Since we process `*ast.ListItem` nodes and skip code blocks within them, this works correctly.

## Line Number Tracking

Precise line numbers are maintained throughout parsing for:
- Error messages
- Query results
- Trait/ref association with parent objects

**Implementation:**
1. Compute line start offsets: `computeLineStarts(content)`
2. Convert byte offset to line: `offsetToLine(lineStarts, offset)`
3. Add `startLine` offset for frontmatter

## Parent Object Association

Traits and refs are associated with the nearest containing object:

```markdown
# Project           ← file-level object

## Tasks            ← section object

- @todo Task 1      ← trait parented to "Tasks" section

## Notes            ← section object

See [[ref]]         ← ref parented to "Notes" section
```

**Algorithm:**
1. Objects are sorted by `LineStart`
2. For each trait/ref, find the object with the largest `LineStart` that is ≤ the trait/ref's line
3. That object is the parent

## File Structure

```
internal/parser/
├── ast.go           # Goldmark AST extraction (ExtractFromAST)
├── ast_test.go      # AST extraction tests
├── document.go      # Main ParseDocument function, object building
├── document_test.go # Integration tests
├── frontmatter.go   # YAML frontmatter parsing
├── traits.go        # Trait regex and parsing
├── refs.go          # Reference extraction (also used for frontmatter)
├── typedecl.go      # ::type() declaration parsing
├── markdown.go      # Helper functions (Slugify, line utilities)
├── codeblock.go     # Legacy code block utilities (used by refs.go)
└── wikilink/        # Wikilink parsing package
```

## Design Decisions

### Why goldmark-first?

1. **Correctness**: Goldmark is CommonMark-compliant and handles edge cases
2. **Code block handling**: Fenced, indented, and inline code all handled uniformly
3. **Maintainability**: Standard library for standard syntax, custom code only for Raven syntax
4. **Future-proof**: As markdown evolves, goldmark updates handle it

### Why not full AST for everything?

Some Raven syntax doesn't map cleanly to AST:

1. **Type declarations** (`::type()`) - Not valid markdown, parsed from text
2. **Traits** (`@name`) - Not valid markdown, parsed from text
3. **Wikilinks** (`[[ref]]`) - Goldmark splits at `[`, requires text reconstruction

The hybrid approach uses AST for structure and code detection, with regex for Raven-specific syntax.

### Why collect text at paragraph level?

Goldmark breaks text at special characters. For wikilinks:
- Input: `See [[target]] here`
- AST Text nodes: `"See ["`, `"["`, `"target]"`, `"] here"`

Collecting at paragraph level reconstructs the original text for regex matching.
