package bulkops

import "strings"

type RawApplyCommand struct {
	Command string
	Args    []string
}

func ParseRawApply(applyArgs []string) (*RawApplyCommand, error) {
	applyStr := strings.Join(applyArgs, " ")
	parts := strings.Fields(applyStr)
	if len(parts) == 0 {
		return nil, newError(CodeInvalidInput, "no apply command specified", "Use --apply <command> [args...]")
	}

	return &RawApplyCommand{
		Command: parts[0],
		Args:    parts[1:],
	}, nil
}
