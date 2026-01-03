# Raven LSP Implementation Design

## Overview

A Language Server Protocol implementation for Raven that provides:
- Autocomplete for references (`[[`), traits (`@`), and types
- Go-to-definition for references
- Hover information
- Real-time diagnostics
- Automated index updates

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Editor (VS Code, Cursor, Neovim)     │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ LSP Protocol (JSON-RPC over stdio)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      LSP Server (rvn lsp)                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Document   │  │  Completion │  │    Diagnostics      │  │
│  │  Manager    │  │  Provider   │  │    Provider         │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│         │                │                    │             │
│         ▼                ▼                    ▼             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                   Index Manager                      │    │
│  │  - Live index (in-memory for open files)            │    │
│  │  - SQLite index (for closed files)                  │    │
│  │  - Incremental updates on file changes              │    │
│  └─────────────────────────────────────────────────────┘    │
│         │                                                   │
│         ▼                                                   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Existing Raven Infrastructure            │   │
│  │  - Parser (frontmatter, traits, refs)                │   │
│  │  - Schema (types, traits definitions)                │   │
│  │  - Resolver (reference resolution)                   │   │
│  │  - Validator (error detection)                       │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## LSP Methods to Implement

### Phase 1: Core Features

| Method | Purpose | Implementation |
|--------|---------|----------------|
| `initialize` | Handshake, declare capabilities | Return supported features |
| `textDocument/didOpen` | Track open documents | Store in document manager |
| `textDocument/didChange` | Handle edits | Update document buffer |
| `textDocument/didSave` | Trigger reindex | Update index for file |
| `textDocument/didClose` | Cleanup | Remove from document manager |
| `textDocument/completion` | Autocomplete | Query index/schema |
| `textDocument/definition` | Go-to-definition | Use resolver |
| `textDocument/publishDiagnostics` | Show errors | Run validator |

### Phase 2: Enhanced Features

| Method | Purpose |
|--------|---------|
| `textDocument/hover` | Show info on hover |
| `textDocument/references` | Find all references |
| `textDocument/rename` | Rename with updates |
| `textDocument/codeAction` | Quick fixes |

## Key Components

### 1. Document Manager (`internal/lsp/documents.go`)

```go
type DocumentManager struct {
    mu        sync.RWMutex
    documents map[string]*Document  // URI -> Document
}

type Document struct {
    URI     string
    Content string
    Version int
    Parsed  *parser.ParsedDocument  // Cached parse result
}

// Methods:
// - Open(uri, content, version)
// - Update(uri, changes, version)
// - Close(uri)
// - Get(uri) *Document
// - GetContent(uri) string
```

### 2. Index Manager (`internal/lsp/indexer.go`)

```go
type IndexManager struct {
    db        *index.Database
    schema    *schema.Schema
    vaultPath string
    
    // In-memory overlay for unsaved changes
    overlay   map[string]*parser.ParsedDocument
}

// Methods:
// - Initialize(vaultPath) - Load existing index
// - UpdateDocument(path, doc) - Update single document
// - GetAllObjectIDs() []string - For completion
// - Resolve(ref) *resolver.Result
// - Reindex() - Full reindex
```

### 3. Completion Provider (`internal/lsp/completion.go`)

Trigger characters: `[`, `@`, `:`

```go
func (s *Server) Completion(params *protocol.CompletionParams) (*protocol.CompletionList, error) {
    doc := s.documents.Get(params.TextDocument.URI)
    line := getLineAt(doc.Content, params.Position.Line)
    
    // Detect context
    switch {
    case isInReference(line, params.Position.Character):
        // Complete with object IDs from index
        return s.completeReferences(params)
        
    case isInTrait(line, params.Position.Character):
        // Complete with trait names from schema
        return s.completeTraits(params)
        
    case isInFrontmatterType(doc, params.Position):
        // Complete with type names from schema
        return s.completeTypes(params)
    }
    
    return nil, nil
}
```

### 4. Definition Provider (`internal/lsp/definition.go`)

```go
func (s *Server) Definition(params *protocol.DefinitionParams) ([]protocol.Location, error) {
    doc := s.documents.Get(params.TextDocument.URI)
    
    // Find reference at position
    ref := findRefAtPosition(doc.Content, params.Position)
    if ref == "" {
        return nil, nil
    }
    
    // Resolve using existing resolver
    result := s.indexer.Resolve(ref)
    if result.NotFound || result.Ambiguous {
        return nil, nil
    }
    
    // Return location
    targetPath := filepath.Join(s.vaultPath, result.TargetID+".md")
    return []protocol.Location{{
        URI:   "file://" + targetPath,
        Range: protocol.Range{}, // Start of file
    }}, nil
}
```

### 5. Diagnostics Provider (`internal/lsp/diagnostics.go`)

```go
func (s *Server) publishDiagnostics(uri string) {
    doc := s.documents.Get(uri)
    if doc == nil {
        return
    }
    
    // Parse and validate
    parsed, _ := parser.ParseDocument(doc.Content, uriToPath(uri), s.vaultPath)
    
    // Run validator
    objectIDs := s.indexer.GetAllObjectIDs()
    validator := check.NewValidator(s.schema, objectIDs)
    issues := validator.Validate(parsed)
    
    // Convert to LSP diagnostics
    diagnostics := make([]protocol.Diagnostic, 0, len(issues))
    for _, issue := range issues {
        diagnostics = append(diagnostics, protocol.Diagnostic{
            Range:    lineToRange(issue.Line),
            Severity: severityFromLevel(issue.Level),
            Message:  issue.Message,
            Source:   "raven",
        })
    }
    
    s.client.PublishDiagnostics(&protocol.PublishDiagnosticsParams{
        URI:         uri,
        Diagnostics: diagnostics,
    })
}
```

## Automated Reindexing Strategy

### Approach: Hybrid (LSP events + file watching)

1. **On `didSave`**: Update index for the saved file
2. **On `didOpen`**: Ensure file is in index
3. **Background watcher**: Catch external file changes (git pull, etc.)

### Index Update Flow

```
File saved in editor
        │
        ▼
    didSave event
        │
        ▼
   Parse document
        │
        ▼
   Update SQLite index
   (single document)
        │
        ▼
   Broadcast to other
   open documents
   (for ref validation)
```

### File Watcher (for external changes)

```go
type FileWatcher struct {
    watcher   *fsnotify.Watcher
    debouncer *time.Timer
    pending   map[string]bool
}

// Watch vault directory
// On change: debounce 500ms, then reindex changed files
// Ignore: .raven/, .git/, .trash/
```

## CLI Command

```bash
# Start LSP server (stdio mode)
rvn lsp

# Start with specific vault
rvn lsp --vault-path /path/to/vault

# Debug mode (logs to stderr)
rvn lsp --debug
```

## Editor Configuration

### VS Code / Cursor

Create extension or use generic LSP client:

```json
// settings.json
{
  "raven.serverPath": "/path/to/rvn",
  "raven.vaultPath": "/path/to/vault"
}
```

### Neovim (with nvim-lspconfig)

```lua
require('lspconfig.configs').raven = {
  default_config = {
    cmd = { 'rvn', 'lsp' },
    filetypes = { 'markdown' },
    root_dir = function(fname)
      return vim.fn.findfile('raven.yaml', '.;')
    end,
  },
}
require('lspconfig').raven.setup{}
```

## Dependencies

```go
require (
    go.lsp.dev/protocol v0.12.0
    go.lsp.dev/jsonrpc2 v0.10.0
    github.com/fsnotify/fsnotify v1.7.0
)
```

## Implementation Status

### Foundation (Done)
- [x] LSP server skeleton (`rvn lsp` command)
- [x] Document manager (open/change/close)
- [x] Basic completion for `[[` references
- [x] Trait completion (`@`)
- [x] Go-to-definition for references
- [x] Hover information
- [x] Diagnostics on save
- [x] Auto-reindex on save

### Remaining Work
- [ ] Editor extension (VS Code/Cursor)
- [x] File watcher for external changes (shared `internal/watcher/`)
- [ ] Type completion in frontmatter
- [ ] Find all references
- [ ] Rename support
- [ ] Code actions (create missing page)
- [ ] Real-world testing and polish

## Next Steps

1. Create minimal VS Code/Cursor extension
2. Test with real vault
3. Add file watcher for external changes (git pull, etc.)
4. Polish completion (better filtering, snippets)
