package bulkops

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
)

type TraitApplyPlan struct {
	Command  string
	NewValue string
	Items    []model.Trait
}

func PlanTraitApply(raw *RawApplyCommand, traits []model.Trait) (*TraitApplyPlan, error) {
	if raw == nil {
		return nil, applyErr(codes.ErrInvalidInput, "no apply command specified", "Use --apply <command> [args...]")
	}
	if raw.Command != "update" {
		return nil, applyErr(
			codes.ErrInvalidInput,
			fmt.Sprintf("'%s' is not supported for trait queries", raw.Command),
			"For trait queries, use: --apply \"update <new_value>\"",
		)
	}

	newValue := strings.TrimSpace(strings.Join(raw.Args, " "))
	if newValue == "" {
		return nil, applyErr(codes.ErrMissingArgument, "no value specified", "Usage: --apply \"update <new_value>\"")
	}

	return &TraitApplyPlan{
		Command:  raw.Command,
		NewValue: newValue,
		Items:    traits,
	}, nil
}
