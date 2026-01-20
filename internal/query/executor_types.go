package query

// ObjectResult represents an object returned from a query.
type ObjectResult struct {
	ID        string
	Type      string
	Fields    map[string]interface{}
	FilePath  string
	LineStart int
	ParentID  *string
}

// TraitResult represents a trait returned from a query.
type TraitResult struct {
	ID             string
	TraitType      string
	Value          *string
	Content        string
	FilePath       string
	Line           int
	ParentObjectID string
}
