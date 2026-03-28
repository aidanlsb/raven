package model

// Numbered wraps any item with a 1-indexed number for user reference.
// This is used by commands that return lists (query, backlinks, search)
// to preserve numbered result output in CLI and MCP responses.
type Numbered[T any] struct {
	// Num is the 1-indexed result number for user reference.
	Num int `json:"num"`

	// Item is the underlying result.
	Item T `json:"item"`
}

// NumberedList converts a slice of results to numbered results.
func NumberedList[T any](items []T) []Numbered[T] {
	result := make([]Numbered[T], len(items))
	for i, item := range items {
		result[i] = Numbered[T]{
			Num:  i + 1, // 1-indexed
			Item: item,
		}
	}
	return result
}
