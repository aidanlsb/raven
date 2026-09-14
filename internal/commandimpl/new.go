package commandimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
)

// HandleNew executes the canonical `new` command.
func HandleNew(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	typeName := strings.TrimSpace(stringArg(req.Args, "type"))
	title := strings.TrimSpace(stringArg(req.Args, "title"))
	// Leave targetPath empty when no explicit --object-path is given so the service
	// derives the filename/slug from the title (which may contain "/").
	targetPath := strings.TrimSpace(stringArg(req.Args, "object-path"))

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --field payload", nil, err.Error())
	}

	typedFieldValues, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --fields-json payload", nil, "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	allFieldValues := mergeFieldInputs(fieldValues, typedFieldValues)

	result, err := objectsvc.Write(objectsvc.WriteRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		Title:       title,
		TargetPath:  targetPath,
		FieldValues: allFieldValues,
		VaultConfig: vaultCfg,
		Schema:      sch,
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		TemplateID:  stringArg(req.Args, "template"),
		CreateOnly:  true,
		Runtime:     rt,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	missingRefs, postWarnings := applyChangeSet(rt, result.ChangeSet, req.IndexJournalOperation)
	data := commandpayload.NewResult{
		ObjectMutation: commandpayload.ObjectMutation{
			File:  result.RelativePath,
			ID:    vaultCfg.FilePathToObjectID(result.RelativePath),
			Title: title,
			Type:  typeName,
		},
		MissingReferences: missingRefs,
	}

	return commandexec.SuccessWithWarnings(data, postWarnings, nil)
}

func mapContentMutationError(err error) commandexec.Result {
	if _, ok := svcerr.AsError(err); ok {
		return commandexec.FromServiceError(err)
	}

	var unknownErr *fieldmutation.UnknownFieldMutationError
	if errors.As(err, &unknownErr) {
		return commandexec.Failure("UNKNOWN_FIELD", unknownErr.Error(), unknownErr.Details(), unknownErr.Suggestion())
	}

	var validationErr *fieldmutation.ValidationError
	if errors.As(err, &validationErr) {
		return commandexec.Failure("VALIDATION_FAILED", validationErr.Error(), nil, validationErr.Suggestion())
	}

	return commandexec.FromServiceError(err)
}

func warningMessagesToCommandWarnings(messages []string, code codes.WarningCode) []commandexec.Warning {
	if len(messages) == 0 {
		return nil
	}

	warnings := make([]commandexec.Warning, 0, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		warnings = append(warnings, commandexec.Warning{
			Code:    code,
			Message: message,
		})
	}
	return warnings
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func commaStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue
			}
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ",")
	default:
		return stringArg(args, key)
	}
}

func boolArg(args map[string]any, key string) bool {
	if args == nil {
		return false
	}
	value, ok := args[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func intArg(args map[string]any, key string) (int, bool) {
	if args == nil {
		return 0, false
	}
	value, ok := args[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	default:
		return 0, false
	}
}

func parseKeyValueArgs(raw any) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}

	out := make(map[string]string)
	for _, pair := range keyValuePairs(raw) {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("use --field key=value")
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func keyValuePairs(v any) []string {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for key := range val {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, fmt.Sprintf("%s=%v", key, val[key]))
		}
		return values
	case map[string]string:
		keys := make([]string, 0, len(val))
		for key := range val {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, key+"="+val[key])
		}
		return values
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		return []string{s}
	case []string:
		values := make([]string, 0, len(val))
		for _, item := range val {
			item = strings.TrimSpace(item)
			if item != "" {
				values = append(values, item)
			}
		}
		return values
	case []interface{}:
		values := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				values = append(values, s)
			}
		}
		return values
	default:
		return nil
	}
}

func parseTypedFieldValues(raw any) (map[string]fieldvalue.FieldValue, error) {
	if raw == nil {
		return map[string]fieldvalue.FieldValue{}, nil
	}

	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return map[string]fieldvalue.FieldValue{}, nil
		}
		return fieldmutation.ParseFieldValuesJSON(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return fieldmutation.ParseFieldValuesJSON(string(b))
	}
}
