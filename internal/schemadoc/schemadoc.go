// Package schemadoc owns validated schema.yaml document edits.
//
// It deliberately keeps the editable YAML representation separate from the
// typed schema model: the document preserves the mutation semantics used by
// existing commands, while Schema exposes the typed view loaded from the exact
// same bytes. Every write is parsed through schema.Parse before it is committed.
package schemadoc

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

// ErrNoChange lets a mutation finish successfully without rewriting schema.yaml.
var ErrNoChange = errors.New("schema document unchanged")

// Operation identifies the stage of a failed document edit.
type Operation string

const (
	OperationRead     Operation = "read"
	OperationLoad     Operation = "load"
	OperationDecode   Operation = "decode"
	OperationMarshal  Operation = "marshal"
	OperationValidate Operation = "validate"
	OperationWrite    Operation = "write"
)

// Error reports which stage of a schema document edit failed.
type Error struct {
	Operation Operation
	Err       error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Document contains the typed and editable views of one schema.yaml snapshot.
type Document struct {
	vaultPath string
	schema    *schema.Schema
	root      map[string]interface{}
}

// Load reads schema.yaml once and builds typed and editable views from the same
// bytes. Unlike schema.Load, a missing file is an error because there is no
// document to edit.
func Load(vaultPath string) (*Document, error) {
	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, editError(OperationRead, err)
	}

	loaded, err := schema.Parse(data, schemaPath)
	if err != nil {
		return nil, editError(OperationLoad, err)
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, editError(OperationDecode, err)
	}
	if root == nil {
		return nil, editError(OperationDecode, fmt.Errorf("schema document must be a YAML mapping"))
	}

	return &Document{
		vaultPath: vaultPath,
		schema:    loaded.Schema,
		root:      root,
	}, nil
}

// Edit runs a mutation against typed and editable views of one schema.yaml
// snapshot, validates the staged YAML through the typed loader, and writes it
// atomically.
//
// This function does NOT record index invalidation. Callers must use
// EditWithInvalidation if the mutation requires index recovery tracking.
func Edit(vaultPath string, mutate func(*Document) error) error {
	doc, err := Load(vaultPath)
	if err != nil {
		return err
	}
	if err := mutate(doc); err != nil {
		if errors.Is(err, ErrNoChange) {
			return nil
		}
		return err
	}
	return doc.Write()
}

// EditResult reports the outcome of a schema mutation with invalidation tracking.
type EditResult struct {
	// OperationID is the index journal operation ID, if invalidation was recorded.
	// Empty when the mutation requires no index work.
	OperationID string
	// Changed is true if the schema.yaml file was rewritten.
	Changed bool
}

// EditWithInvalidation runs a schema mutation and records index invalidation
// before the durable schema write. It returns the operation ID for tracking
// and whether the schema changed.
//
// If the mutation requires index recovery (PolicyFullScan or
// PolicyResolverRefresh), this function:
//  1. Loads the before schema
//  2. Applies the mutation
//  3. Classifies the change
//  4. Records invalidation in the index journal
//  5. Writes schema.yaml atomically
//
// If recording invalidation fails, the mutation aborts before writing schema.yaml.
// The caller is responsible for running ApplyInvalidation afterward when
// auto-reindex is enabled.
func EditWithInvalidation(vaultPath string, mutate func(*Document) error) (*EditResult, error) {
	// Load before state
	beforeSchema, beforeErr := schema.Load(vaultPath)
	if beforeErr != nil {
		// If schema doesn't exist yet or is corrupt, treat as nil
		beforeSchema = nil
	}

	doc, err := Load(vaultPath)
	if err != nil {
		return nil, err
	}

	if err := mutate(doc); err != nil {
		if errors.Is(err, ErrNoChange) {
			return &EditResult{Changed: false}, nil
		}
		return nil, err
	}

	// Validate and parse the staged mutation to get after schema
	output, marshalErr := yaml.Marshal(doc.root)
	if marshalErr != nil {
		return nil, editError(OperationMarshal, marshalErr)
	}
	if err := validate(output, paths.SchemaPath(vaultPath)); err != nil {
		return nil, err
	}

	afterResult, parseErr := schema.Parse(output, paths.SchemaPath(vaultPath))
	if parseErr != nil {
		return nil, editError(OperationValidate, parseErr)
	}
	afterSchema := afterResult.Schema

	// Classify and record invalidation before writing
	operationID, classification, classifyErr := recordInvalidation(vaultPath, beforeSchema, afterSchema)
	if classifyErr != nil {
		return nil, classifyErr
	}
	classificationState.Lock()
	classificationState.value = classification
	classificationState.Unlock()

	// Write schema.yaml atomically
	if err := atomicfile.WriteFile(paths.SchemaPath(vaultPath), output, 0o644); err != nil {
		return nil, editError(OperationWrite, err)
	}

	return &EditResult{
		OperationID: operationID,
		Changed:     true,
	}, nil
}

// RecordInvalidationFunc is the signature for the schema invalidation hook.
type RecordInvalidationFunc func(vaultPath string, beforeSchema, afterSchema *schema.Schema) (operationID string, classification interface{}, err error)

// recordInvalidation is the internal hook for schema change classification.
// It is defined as a variable so tests can override it and so we can avoid
// import cycles (schemasvc sets this via SetRecordInvalidationHook).
var recordInvalidation RecordInvalidationFunc = func(vaultPath string, beforeSchema, afterSchema *schema.Schema) (string, interface{}, error) {
	// Default no-op: no invalidation recorded
	return "", nil, nil
}

// SetRecordInvalidationHook wires up the schema change invalidation logic.
// Called by schemasvc.init() to avoid import cycles.
func SetRecordInvalidationHook(fn RecordInvalidationFunc) {
	recordInvalidation = fn
}

// classificationState stores the classification from the most recent edit.
// Protected by a mutex for concurrent access.
var classificationState struct {
	sync.RWMutex
	value interface{}
}

// GetLastClassification returns the classification from the most recent
// EditWithInvalidation call. This is a workaround for the hook interface.
func GetLastClassification() interface{} {
	classificationState.RLock()
	defer classificationState.RUnlock()
	return classificationState.value
}

// Schema returns the typed model loaded from the document's original bytes.
func (d *Document) Schema() *schema.Schema {
	return d.schema
}

// Root returns the editable YAML mapping.
func (d *Document) Root() map[string]interface{} {
	return d.root
}

// Marshal returns the edited YAML after validating it through the typed loader.
// It does not write the document.
func (d *Document) Marshal() ([]byte, error) {
	output, err := yaml.Marshal(d.root)
	if err != nil {
		return nil, editError(OperationMarshal, err)
	}
	if err := validate(output, paths.SchemaPath(d.vaultPath)); err != nil {
		return nil, err
	}
	return output, nil
}

// Write validates and atomically writes the edited document.
func (d *Document) Write() error {
	output, err := yaml.Marshal(d.root)
	if err != nil {
		return editError(OperationMarshal, err)
	}
	return Write(d.vaultPath, output)
}

// Write validates staged schema YAML and atomically writes it to schema.yaml.
// Rename plans use this to commit bytes prepared during preview planning.
func Write(vaultPath string, data []byte) error {
	schemaPath := paths.SchemaPath(vaultPath)
	if err := validate(data, schemaPath); err != nil {
		return err
	}
	if err := atomicfile.WriteFile(schemaPath, data, 0o644); err != nil {
		return editError(OperationWrite, err)
	}
	return nil
}

// EnsureMap returns key's mapping value, replacing a missing or non-mapping
// value with an empty mapping.
func EnsureMap(parent map[string]interface{}, key string) map[string]interface{} {
	node, ok := parent[key].(map[string]interface{})
	if ok {
		return node
	}
	node = make(map[string]interface{})
	parent[key] = node
	return node
}

func validate(data []byte, source string) error {
	if _, err := schema.Parse(data, source); err != nil {
		return editError(OperationValidate, err)
	}
	return nil
}

func editError(operation Operation, err error) *Error {
	return &Error{
		Operation: operation,
		Err:       err,
	}
}
