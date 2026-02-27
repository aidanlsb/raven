package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// StepOutputEntry represents one key/value entry from an object output page.
type StepOutputEntry struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// StepOutputPage is a paged view over a selected part of a step output.
type StepOutputPage struct {
	Path       string            `json:"path,omitempty"`
	Kind       string            `json:"kind"`
	Offset     int               `json:"offset"`
	Limit      int               `json:"limit"`
	Total      int               `json:"total"`
	Returned   int               `json:"returned"`
	HasMore    bool              `json:"has_more"`
	NextOffset int               `json:"next_offset,omitempty"`
	Items      []interface{}     `json:"items,omitempty"`
	Entries    []StepOutputEntry `json:"entries,omitempty"`
	Text       string            `json:"text,omitempty"`
}

// PaginateStepOutput returns a deterministic page from step output content.
//
// Supported target types:
// - arrays/slices (returned via Items)
// - objects with string keys (returned via Entries, key-sorted)
// - strings (returned via Text, rune-safe slicing)
//
// When path is non-empty, it is interpreted as dot-separated object keys
// (e.g. "data.results") relative to the root step output object.
func PaginateStepOutput(root interface{}, path string, offset, limit int) (*StepOutputPage, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	target, err := resolveStepOutputPath(root, path)
	if err != nil {
		return nil, err
	}

	page := &StepOutputPage{
		Path:   path,
		Offset: offset,
		Limit:  limit,
	}

	switch v := target.(type) {
	case []interface{}:
		page.Kind = "array"
		page.Total = len(v)
		start, end := pageBounds(page.Total, offset, limit)
		page.Items = append([]interface{}(nil), v[start:end]...)
		page.Returned = len(page.Items)
	case map[string]interface{}:
		page.Kind = "object"
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		page.Total = len(keys)
		start, end := pageBounds(page.Total, offset, limit)
		page.Entries = make([]StepOutputEntry, 0, end-start)
		for _, k := range keys[start:end] {
			page.Entries = append(page.Entries, StepOutputEntry{
				Key:   k,
				Value: v[k],
			})
		}
		page.Returned = len(page.Entries)
	case string:
		page.Kind = "string"
		runes := []rune(v)
		page.Total = len(runes)
		start, end := pageBounds(page.Total, offset, limit)
		page.Text = string(runes[start:end])
		page.Returned = end - start
	default:
		return nil, fmt.Errorf("path %q points to unsupported type %T; expected array, object, or string", path, target)
	}

	page.HasMore = offset+page.Returned < page.Total
	if page.HasMore {
		page.NextOffset = offset + page.Returned
	}

	return page, nil
}

func resolveStepOutputPath(root interface{}, path string) (interface{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return root, nil
	}

	segments := strings.Split(path, ".")
	cur := root
	traversed := make([]string, 0, len(segments))

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, fmt.Errorf("invalid path %q: empty path segment", path)
		}

		obj, ok := cur.(map[string]interface{})
		if !ok {
			at := strings.Join(traversed, ".")
			if at == "" {
				at = "<root>"
			}
			return nil, fmt.Errorf("invalid path %q at %q: value is %T, expected object", path, at, cur)
		}

		next, ok := obj[segment]
		if !ok {
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return nil, fmt.Errorf("path %q not found: key %q missing (available: %s)", path, segment, strings.Join(keys, ", "))
		}

		traversed = append(traversed, segment)
		cur = next
	}

	return cur, nil
}

func pageBounds(total, offset, limit int) (int, int) {
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return offset, end
}
