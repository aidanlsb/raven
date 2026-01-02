package lsp

import (
	"sync"

	"github.com/aidanlsb/raven/internal/parser"
)

// DocumentManager tracks open documents and their content.
type DocumentManager struct {
	mu        sync.RWMutex
	documents map[string]*Document
}

// Document represents an open document in the editor.
type Document struct {
	URI     string
	Content string
	Version int
	Parsed  *parser.ParsedDocument // Cached parse result (may be nil)
}

// NewDocumentManager creates a new document manager.
func NewDocumentManager() *DocumentManager {
	return &DocumentManager{
		documents: make(map[string]*Document),
	}
}

// Open registers a newly opened document.
func (dm *DocumentManager) Open(uri, content string, version int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.documents[uri] = &Document{
		URI:     uri,
		Content: content,
		Version: version,
	}
}

// Update applies changes to a document.
// For now, we only support full document sync (the entire content is replaced).
func (dm *DocumentManager) Update(uri, content string, version int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if doc, ok := dm.documents[uri]; ok {
		doc.Content = content
		doc.Version = version
		doc.Parsed = nil // Invalidate cached parse
	}
}

// Close removes a document from tracking.
func (dm *DocumentManager) Close(uri string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	delete(dm.documents, uri)
}

// Get retrieves a document by URI.
func (dm *DocumentManager) Get(uri string) *Document {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	return dm.documents[uri]
}

// GetContent retrieves just the content of a document.
func (dm *DocumentManager) GetContent(uri string) string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if doc, ok := dm.documents[uri]; ok {
		return doc.Content
	}
	return ""
}

// SetParsed caches the parsed document.
func (dm *DocumentManager) SetParsed(uri string, parsed *parser.ParsedDocument) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if doc, ok := dm.documents[uri]; ok {
		doc.Parsed = parsed
	}
}

// All returns all open documents.
func (dm *DocumentManager) All() []*Document {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	docs := make([]*Document, 0, len(dm.documents))
	for _, doc := range dm.documents {
		docs = append(docs, doc)
	}
	return docs
}
