package cli

// This file holds small, package-wide helpers for decoding values out of the
// generic canonical response envelope (map[string]interface{}) into concrete
// Go types for rendering.

func stringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func stringPointer(raw interface{}) *string {
	switch value := raw.(type) {
	case nil:
		return nil
	case string:
		return &value
	case *string:
		return value
	default:
		return nil
	}
}

func mapValue(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	default:
		return nil
	}
}

func intFromAny(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func intPointerFromAny(raw interface{}) *int {
	switch value := raw.(type) {
	case nil:
		return nil
	case int:
		return &value
	case int64:
		v := int(value)
		return &v
	case float64:
		v := int(value)
		return &v
	default:
		return nil
	}
}

func int64FromAny(raw interface{}) int64 {
	switch value := raw.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}
