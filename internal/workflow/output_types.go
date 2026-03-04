package workflow

import (
	"fmt"
	"strings"
)

type outputTypeSpec struct {
	Base    string
	IsArray bool
}

func parseOutputType(raw string) (outputTypeSpec, error) {
	typ := strings.TrimSpace(raw)
	if typ == "" {
		return outputTypeSpec{}, fmt.Errorf("type is required")
	}

	spec := outputTypeSpec{Base: typ}
	if strings.HasSuffix(typ, "[]") {
		spec.IsArray = true
		spec.Base = strings.TrimSpace(strings.TrimSuffix(typ, "[]"))
		if spec.Base == "" {
			return outputTypeSpec{}, fmt.Errorf("type is required before []")
		}
	}

	switch spec.Base {
	case "markdown", "string", "number", "bool", "object", "array":
	default:
		return outputTypeSpec{}, fmt.Errorf("unknown type %q", typ)
	}

	if spec.IsArray && spec.Base == "array" {
		return outputTypeSpec{}, fmt.Errorf("array[] is not supported")
	}

	return spec, nil
}
