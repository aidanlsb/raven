package cli

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
)

func readSchemaDoc(vaultPath string) (map[string]interface{}, map[string]interface{}, error) {
	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, nil, handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return nil, nil, handleError(ErrSchemaInvalid, err, "")
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return nil, nil, handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}

	return schemaDoc, types, nil
}

func writeSchemaDoc(vaultPath string, schemaDoc map[string]interface{}) error {
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	schemaPath := paths.SchemaPath(vaultPath)
	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}
	return nil
}
