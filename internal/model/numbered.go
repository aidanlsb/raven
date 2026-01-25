package model

// Result is the interface implemented by all result types.
// This allows uniform handling in display and numbered reference systems.
type Result interface {
	GetID() string
	GetKind() string // "trait", "object", "reference", "search"
	GetContent() string
	GetLocation() string
}

// Numbered wraps any Result with a 1-indexed number for user reference.
// This is used by commands that return lists (query, backlinks, search)
// to enable selection via `rvn last 1,3,5`.
type Numbered[T Result] struct {
	// Num is the 1-indexed result number for user reference.
	Num int `json:"num"`

	// Item is the underlying result.
	Item T `json:"item"`
}

// GetNum returns the result number.
func (n Numbered[T]) GetNum() int { return n.Num }

// GetID delegates to the underlying item.
func (n Numbered[T]) GetID() string { return n.Item.GetID() }

// GetKind delegates to the underlying item.
func (n Numbered[T]) GetKind() string { return n.Item.GetKind() }

// GetContent delegates to the underlying item.
func (n Numbered[T]) GetContent() string { return n.Item.GetContent() }

// GetLocation delegates to the underlying item.
func (n Numbered[T]) GetLocation() string { return n.Item.GetLocation() }

// NumberedList converts a slice of results to numbered results.
func NumberedList[T Result](items []T) []Numbered[T] {
	result := make([]Numbered[T], len(items))
	for i, item := range items {
		result[i] = Numbered[T]{
			Num:  i + 1, // 1-indexed
			Item: item,
		}
	}
	return result
}
