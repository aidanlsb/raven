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
