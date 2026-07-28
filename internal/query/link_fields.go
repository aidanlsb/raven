package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// linkFieldTypes is the single field vocabulary for predicates over link rows.
// It is shared by links(...) and the link query root.
var linkFieldTypes = map[string]schema.FieldType{
	"source_id":      schema.FieldTypeString,
	"source_type":    schema.FieldTypeString,
	"file_path":      schema.FieldTypeString,
	"line":           schema.FieldTypeNumber,
	"position_start": schema.FieldTypeNumber,
	"position_end":   schema.FieldTypeNumber,
	"raw_target":     schema.FieldTypeString,
	"display":        schema.FieldTypeString,
	"is_image":       schema.FieldTypeBool,
	"scheme":         schema.FieldTypeString,
	"ext":            schema.FieldTypeString,
	"normalized_key": schema.FieldTypeString,
}

func availableLinkFields() []string {
	fields := make([]string, 0, len(linkFieldTypes))
	for field := range linkFieldTypes {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func linkColumnExpr(alias, field string) (string, bool) {
	switch field {
	case "source_id":
		return alias + ".source_id", true
	case "source_type":
		return alias + ".source_type", true
	case "file_path":
		return alias + ".file_path", true
	case "line":
		return alias + ".line_number", true
	case "position_start":
		return alias + ".position_start", true
	case "position_end":
		return alias + ".position_end", true
	case "raw_target":
		return alias + ".raw_target", true
	case "display":
		return alias + ".display", true
	case "is_image":
		return alias + ".is_image", true
	case "scheme":
		return alias + ".scheme", true
	case "ext":
		return alias + ".ext", true
	case "normalized_key":
		return alias + ".normalized_key", true
	default:
		return "", false
	}
}

// validateLinkPredicate validates the shared predicate grammar over link rows.
// Link predicates support scalar field comparisons, oneof() (which parses to
// comparisons), and string functions over the string-valued link fields.
func validateLinkPredicate(pred Predicate) error {
	switch p := pred.(type) {
	case *OrPredicate:
		for _, child := range p.Predicates {
			if err := validateLinkPredicate(child); err != nil {
				return err
			}
		}
		return nil
	case *GroupPredicate:
		for _, child := range p.Predicates {
			if err := validateLinkPredicate(child); err != nil {
				return err
			}
		}
		return nil
	case *NotPredicate:
		return validateLinkPredicate(p.Inner)
	case *FieldPredicate:
		fieldType, ok := linkFieldTypes[p.Field]
		if !ok {
			return unknownLinkFieldError(p.Field)
		}
		if p.IsRefValue {
			return &ValidationError{
				Message:    fmt.Sprintf("link field '.%s' does not support Raven reference values", p.Field),
				Suggestion: "Use an identifier or quoted string value",
			}
		}
		if fieldType == schema.FieldTypeBool && !p.IsExists {
			if p.CompareOp != CompareEq && p.CompareOp != CompareNeq {
				return &ValidationError{
					Message:    "link field '.is_image' only supports == and !=",
					Suggestion: "Use .is_image==true or .is_image==false",
				}
			}
			if !strings.EqualFold(p.Value, "true") && !strings.EqualFold(p.Value, "false") {
				return &ValidationError{
					Message:    fmt.Sprintf("link field '.is_image' requires true or false, got '%s'", p.Value),
					Suggestion: "Use .is_image==true or .is_image==false",
				}
			}
		}
		return nil
	case *StringFuncPredicate:
		if p.IsElementRef {
			return &ValidationError{
				Message:    "string function placeholder '_' is not valid for link predicates",
				Suggestion: `Use a link field such as includes(.display, "...")`,
			}
		}
		fieldType, ok := linkFieldTypes[p.Field]
		if !ok {
			return unknownLinkFieldError(p.Field)
		}
		if fieldType != schema.FieldTypeString {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates are not valid for link field '.%s'", p.Field),
				Suggestion: "Use comparison predicates for non-string link fields",
			}
		}
		return validateRegexPattern(p)
	default:
		return &ValidationError{
			Message:    fmt.Sprintf("unsupported link predicate %T", pred),
			Suggestion: fmt.Sprintf("Filter links with one of: %s", strings.Join(availableLinkFields(), ", ")),
		}
	}
}

func unknownLinkFieldError(field string) error {
	return &ValidationError{
		Message:    fmt.Sprintf("link has no field '%s'", field),
		Suggestion: fmt.Sprintf("Available link fields: %s", strings.Join(availableLinkFields(), ", ")),
	}
}
