package commandexec

// CancelledResult is emitted by interactive CLI flows that the user cancels.
type CancelledResult struct {
	Cancelled bool `json:"cancelled"`
}
