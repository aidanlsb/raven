package bulkops

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
)

type TraitApplyPlan struct {
	Command  string
	NewValue string
	Items    []model.Trait
}

func PlanTraitApply(raw *RawApplyCommand, traits []model.Trait) (*TraitApplyPlan, error) {
	if raw == nil {
		return nil, newError(CodeInvalidInput, "no apply command specified", "Use --apply <command> [args...]")
	}
	if raw.Command != "update" {
		return nil, newError(
			CodeInvalidInput,
			fmt.Sprintf("'%s' is not supported for trait queries", raw.Command),
			"For trait queries, use: --apply \"update <new_value>\"",
		)
	}

	newValue := strings.TrimSpace(strings.Join(raw.Args, " "))
	if newValue == "" {
		return nil, newError(CodeMissingArgument, "no value specified", "Usage: --apply \"update <new_value>\"")
	}

	return &TraitApplyPlan{
		Command:  raw.Command,
		NewValue: newValue,
		Items:    traits,
	}, nil
}
