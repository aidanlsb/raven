package objectsvc

import (
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

type SetEmbeddedObjectRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	FilePath       string
	ObjectID       string
	Updates        map[string]string
	TypedUpdates   map[string]schema.FieldValue
	Schema         *schema.Schema
	AllowedFields  map[string]bool
	DocumentParser *parser.ParseOptions
}

type SetEmbeddedObjectResult struct {
	ObjectID        string
	ObjectType      string
	Slug            string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]schema.FieldValue
}

func SetEmbeddedObject(req SetEmbeddedObjectRequest) (*SetEmbeddedObjectResult, error) {
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}

	_, slug, isEmbedded := paths.ParseEmbeddedID(req.ObjectID)
	if !isEmbedded {
		return nil, newError(ErrorInvalidInput, "invalid embedded object ID", "Expected format: file-id#embedded-id", nil, nil)
	}

	contentBytes, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read file", "", nil, err)
	}
	content := string(contentBytes)

	doc, err := parser.ParseDocumentWithOptions(content, req.FilePath, req.VaultPath, req.DocumentParser)
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse document", "Failed to parse document", nil, err)
	}

	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == req.ObjectID {
			targetObj = obj
			break
		}
	}
	if targetObj == nil {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("embedded object '%s' not found in file", slug),
			"Check that the embedded ID exists in the file",
			nil,
			nil,
		)
	}
	if targetObj.ParentID == nil {
		return nil, newError(
			ErrorInvalidInput,
			"cannot use embedded set on file-level object",
			"Use 'rvn set <file-id> field=value' instead",
			nil,
			nil,
		)
	}

	typeDeclLine := targetObj.LineStart + 1
	lines := strings.Split(content, "\n")
	if typeDeclLine-1 >= len(lines) {
		return nil, newError(ErrorInvalidInput, "type declaration line not found", "", nil, nil)
	}

	declLine := lines[typeDeclLine-1]
	if !strings.HasPrefix(strings.TrimSpace(declLine), "::") {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("expected type declaration at line %d, found: %s", typeDeclLine, strings.TrimSpace(declLine)),
			"The embedded object may have been modified or is in an unexpected format",
			nil,
			nil,
		)
	}

	mergedUpdates := make(map[string]string, len(req.Updates)+len(req.TypedUpdates))
	for key, value := range req.Updates {
		mergedUpdates[key] = value
	}
	for key, value := range req.TypedUpdates {
		mergedUpdates[key] = fieldmutation.SerializeFieldValueLiteral(value)
	}

	fieldNames := make([]string, 0, len(mergedUpdates))
	for key := range mergedUpdates {
		fieldNames = append(fieldNames, key)
	}
	if unknownErr := fieldmutation.DetectUnknownFieldMutationByNames(targetObj.ObjectType, req.Schema, fieldNames, req.AllowedFields); unknownErr != nil {
		return nil, unknownErr
	}

	parsedUpdates, resolvedUpdates, warningMessages, err := fieldmutation.PrepareValidatedFieldMutation(
		targetObj.ObjectType,
		targetObj.Fields,
		mergedUpdates,
		req.Schema,
		req.AllowedFields,
		&fieldmutation.RefValidationContext{
			VaultPath:    req.VaultPath,
			VaultConfig:  req.VaultConfig,
			ParseOptions: req.DocumentParser,
		},
	)
	if err != nil {
		return nil, err
	}

	newFields := make(map[string]schema.FieldValue, len(targetObj.Fields)+len(parsedUpdates))
	for key, value := range targetObj.Fields {
		newFields[key] = value
	}
	for key, value := range parsedUpdates {
		newFields[key] = value
	}

	leadingSpace := ""
	for _, c := range declLine {
		if c == ' ' || c == '\t' {
			leadingSpace += string(c)
			continue
		}
		break
	}
	lines[typeDeclLine-1] = leadingSpace + parser.SerializeTypeDeclaration(targetObj.ObjectType, newFields)

	newContent := strings.Join(lines, "\n")
	if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
		return nil, newError(ErrorFileWrite, "failed to write file", "", nil, err)
	}

	previousFields := make(map[string]schema.FieldValue, len(targetObj.Fields))
	for key, value := range targetObj.Fields {
		previousFields[key] = value
	}

	return &SetEmbeddedObjectResult{
		ObjectID:        req.ObjectID,
		ObjectType:      targetObj.ObjectType,
		Slug:            slug,
		ResolvedUpdates: resolvedUpdates,
		WarningMessages: warningMessages,
		PreviousFields:  previousFields,
	}, nil
}
