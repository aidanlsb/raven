package check

// MissingReferences is embedded in mutation payloads that can create dangling
// references. Both fields are absent when the mutation did not introduce any
// missing targets, preserving the historical conditional map fields.
type MissingReferences struct {
	MissingRefs     int           `json:"missing_refs,omitempty"`
	MissingRefItems []*MissingRef `json:"missing_ref_items,omitempty"`
}

// MissingReferenceItems lets CLI renderers consume the shared missing-reference
// block without decoding a command-specific payload back through JSON.
func (m MissingReferences) MissingReferenceItems() []*MissingRef {
	return m.MissingRefItems
}

// MissingReferencePayload is implemented by mutation payloads that embed
// MissingReferences.
type MissingReferencePayload interface {
	MissingReferenceItems() []*MissingRef
}

// PackMissingRefs builds the omitempty-friendly missing-reference block.
func PackMissingRefs(refs []*MissingRef) MissingReferences {
	if len(refs) == 0 {
		return MissingReferences{}
	}
	return MissingReferences{MissingRefs: len(refs), MissingRefItems: refs}
}
