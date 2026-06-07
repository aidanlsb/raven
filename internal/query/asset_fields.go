package query

import (
	"sort"

	"github.com/aidanlsb/raven/internal/schema"
)

var assetFieldTypes = map[string]schema.FieldType{
	"id":         schema.FieldTypeString,
	"file_path":  schema.FieldTypeString,
	"filename":   schema.FieldTypeString,
	"extension":  schema.FieldTypeString,
	"media_type": schema.FieldTypeString,
	"size_bytes": schema.FieldTypeNumber,
}

func availableAssetFields() []string {
	fields := make([]string, 0, len(assetFieldTypes))
	for field := range assetFieldTypes {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func assetColumnExpr(alias, field string) (string, bool) {
	switch field {
	case "id":
		return alias + ".id", true
	case "file_path":
		return alias + ".file_path", true
	case "filename":
		return alias + ".filename", true
	case "extension":
		return alias + ".extension", true
	case "media_type":
		return alias + ".media_type", true
	case "size_bytes":
		return alias + ".size_bytes", true
	default:
		return "", false
	}
}
