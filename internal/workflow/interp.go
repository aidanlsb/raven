package workflow

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Interpolate replaces {{inputs.*}} and {{steps.*}} references inside s.
//
// Rules:
// - Escaping: \{{ and \}} produce literal braces.
// - Unknown variables are errors (to avoid silent typos).
func Interpolate(s string, inputs map[string]string, steps map[string]interface{}) (string, error) {
	return interpolateWithTypedInputs(s, stringInputsToAny(inputs), steps)
}

// InterpolateObject applies interpolation recursively across a JSON-like object.
//
// If a string value is exactly a single interpolation expression like "{{steps.x}}",
// the resolved value is preserved as its native type (object/array/bool/number/string).
func InterpolateObject(obj map[string]interface{}, inputs map[string]string, steps map[string]interface{}) (map[string]interface{}, error) {
	return interpolateObjectWithTypedInputs(obj, stringInputsToAny(inputs), steps)
}

func stringInputsToAny(inputs map[string]string) map[string]interface{} {
	if inputs == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}
	return out
}

// interpolate replaces {{inputs.*}} and {{steps.*}} references inside s.
//
// Rules:
// - Escaping: \{{ and \}} produce literal braces.
// - Unknown variables are errors (to avoid silent typos).
func interpolateWithTypedInputs(s string, inputs map[string]interface{}, steps map[string]interface{}) (string, error) {
	var out strings.Builder
	out.Grow(len(s))

	var errs []string

	for i := 0; i < len(s); {
		// Escapes
		if s[i] == '\\' && i+2 < len(s) && s[i+1] == '{' && s[i+2] == '{' {
			out.WriteString("{{")
			i += 3
			continue
		}
		if s[i] == '\\' && i+2 < len(s) && s[i+1] == '}' && s[i+2] == '}' {
			out.WriteString("}}")
			i += 3
			continue
		}

		// Interpolation
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			end := strings.Index(s[i+2:], "}}")
			if end < 0 {
				// No closing braces; treat literally.
				out.WriteByte(s[i])
				i++
				continue
			}
			end = i + 2 + end
			expr := strings.TrimSpace(s[i+2 : end])
			i = end + 2

			val, ok, err := resolveExprTyped(expr, inputs, steps)
			if err != nil {
				errs = append(errs, err.Error())
				out.WriteString("{{" + expr + "}}")
				continue
			}
			if !ok {
				errs = append(errs, fmt.Sprintf("unknown variable: %s", expr))
				out.WriteString("{{" + expr + "}}")
				continue
			}
			out.WriteString(val)
			continue
		}

		out.WriteByte(s[i])
		i++
	}

	if len(errs) > 0 {
		return out.String(), errors.New(strings.Join(errs, "; "))
	}
	return out.String(), nil
}

func resolveExpr(expr string, inputs map[string]string, steps map[string]interface{}) (string, bool, error) {
	typed := stringInputsToAny(inputs)
	raw, ok, err := resolveExprRaw(expr, typed, steps)
	if err != nil || !ok {
		return "", ok, err
	}
	return formatForPrompt(raw), true, nil
}

func resolveExprTyped(expr string, inputs map[string]interface{}, steps map[string]interface{}) (string, bool, error) {
	raw, ok, err := resolveExprRaw(expr, inputs, steps)
	if err != nil || !ok {
		return "", ok, err
	}
	return formatForPrompt(raw), true, nil
}

func resolveExprRaw(expr string, inputs map[string]interface{}, steps map[string]interface{}) (interface{}, bool, error) {
	if strings.HasPrefix(expr, "inputs.") {
		key := strings.TrimPrefix(expr, "inputs.")
		if key == "" {
			return nil, false, fmt.Errorf("invalid inputs reference: %s", expr)
		}
		v, ok := inputs[key]
		if !ok {
			return nil, false, nil
		}
		return v, true, nil
	}

	if strings.HasPrefix(expr, "steps.") {
		path := strings.TrimPrefix(expr, "steps.")
		val, ok := resolveStepPath(steps, path)
		if !ok {
			return nil, false, nil
		}
		return val, true, nil
	}

	return nil, false, nil
}

func resolveStepPath(steps map[string]interface{}, path string) (interface{}, bool) {
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	cur, ok := steps[parts[0]]
	if !ok {
		return nil, false
	}
	for _, part := range parts[1:] {
		switch v := cur.(type) {
		case map[string]interface{}:
			next, ok := v[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func interpolateObjectWithTypedInputs(obj map[string]interface{}, inputs map[string]interface{}, steps map[string]interface{}) (map[string]interface{}, error) {
	if obj == nil {
		return nil, nil
	}
	out := make(map[string]interface{}, len(obj))
	for k, v := range obj {
		nv, err := interpolateValue(v, inputs, steps)
		if err != nil {
			return nil, err
		}
		out[k] = nv
	}
	return out, nil
}

func interpolateValue(v interface{}, inputs map[string]interface{}, steps map[string]interface{}) (interface{}, error) {
	switch t := v.(type) {
	case string:
		if expr, ok := extractExactInterpolationExpr(t); ok {
			raw, exists, err := resolveExprRaw(expr, inputs, steps)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("unknown variable: %s", expr)
			}
			return raw, nil
		}
		return interpolateWithTypedInputs(t, inputs, steps)
	case map[string]interface{}:
		return interpolateObjectWithTypedInputs(t, inputs, steps)
	case []interface{}:
		arr := make([]interface{}, len(t))
		for i, item := range t {
			nv, err := interpolateValue(item, inputs, steps)
			if err != nil {
				return nil, err
			}
			arr[i] = nv
		}
		return arr, nil
	default:
		return v, nil
	}
}

func extractExactInterpolationExpr(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return "", false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
	if inner == "" {
		return "", false
	}
	return inner, true
}
