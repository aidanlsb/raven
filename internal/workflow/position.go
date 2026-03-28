package workflow

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
)

// PositionHint controls insertion placement for step mutations.
type PositionHint struct {
	BeforeStepID string
	AfterStepID  string
	Index        *int
}

// ResolveInsertIndex determines a deterministic insertion index for a new step.
func ResolveInsertIndex(steps []*config.WorkflowStep, hint PositionHint) (int, error) {
	beforeID := strings.TrimSpace(hint.BeforeStepID)
	afterID := strings.TrimSpace(hint.AfterStepID)
	if beforeID != "" && afterID != "" {
		return 0, newDomainError(CodeInvalidInput, "use either before_step_id or after_step_id, not both", nil)
	}
	if hint.Index != nil && (beforeID != "" || afterID != "") {
		return 0, newDomainError(CodeInvalidInput, "use index alone or before/after alone", nil)
	}

	if hint.Index != nil {
		idx := *hint.Index
		if idx < 0 || idx > len(steps) {
			return 0, newDomainError(
				CodeInvalidInput,
				fmt.Sprintf("index %d out of bounds for %d steps", idx, len(steps)),
				nil,
			)
		}
		return idx, nil
	}

	if beforeID != "" {
		targetIdx := FindStepIndexInSteps(steps, beforeID)
		if targetIdx < 0 {
			err := newDomainError(CodeRefNotFound, fmt.Sprintf("step '%s' not found", beforeID), nil)
			err.StepID = beforeID
			return 0, err
		}
		return targetIdx, nil
	}

	if afterID != "" {
		targetIdx := FindStepIndexInSteps(steps, afterID)
		if targetIdx < 0 {
			err := newDomainError(CodeRefNotFound, fmt.Sprintf("step '%s' not found", afterID), nil)
			err.StepID = afterID
			return 0, err
		}
		return targetIdx + 1, nil
	}

	return len(steps), nil
}
