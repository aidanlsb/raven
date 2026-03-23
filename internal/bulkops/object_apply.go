package bulkops

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
)

type ObjectApplyCommand string

const (
	ObjectApplySet    ObjectApplyCommand = "set"
	ObjectApplyDelete ObjectApplyCommand = "delete"
	ObjectApplyAdd    ObjectApplyCommand = "add"
	ObjectApplyMove   ObjectApplyCommand = "move"
)

type ObjectApplyPlan struct {
	Command         ObjectApplyCommand
	IDs             []string
	FileIDs         []string
	EmbeddedIDs     []string
	SetUpdates      map[string]string
	AddText         string
	MoveDestination string
}

func PlanObjectApply(raw *RawApplyCommand, ids []string) (*ObjectApplyPlan, error) {
	if raw == nil {
		return nil, newError(CodeInvalidInput, "no apply command specified", "Use --apply <command> [args...]")
	}

	plan := &ObjectApplyPlan{
		Command: ObjectApplyCommand(raw.Command),
		IDs:     dedupeIDs(ids),
	}
	plan.FileIDs, plan.EmbeddedIDs = partitionObjectIDs(plan.IDs)

	switch plan.Command {
	case ObjectApplySet:
		updates := make(map[string]string)
		for _, arg := range raw.Args {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return nil, newError(CodeInvalidInput, fmt.Sprintf("invalid field format: %s", arg), "Use format: field=value")
			}
			updates[parts[0]] = parts[1]
		}
		if len(updates) == 0 {
			return nil, newError(CodeMissingArgument, "no fields to set", "Usage: --apply set field=value...")
		}
		plan.SetUpdates = updates
		return plan, nil
	case ObjectApplyDelete:
		return plan, nil
	case ObjectApplyAdd:
		if len(raw.Args) == 0 {
			return nil, newError(CodeMissingArgument, "no text to add", "Usage: --apply add <text>")
		}
		plan.AddText = strings.Join(raw.Args, " ")
		return plan, nil
	case ObjectApplyMove:
		if len(raw.Args) == 0 {
			return nil, newError(CodeMissingArgument, "no destination provided", "Usage: --apply move <destination-directory/>")
		}
		destination := raw.Args[0]
		if !strings.HasSuffix(destination, "/") {
			return nil, newError(CodeInvalidInput, "destination must be a directory (end with /)", "Example: --apply move archive/projects/")
		}
		plan.MoveDestination = destination
		return plan, nil
	default:
		return nil, newError(CodeInvalidInput, fmt.Sprintf("unknown apply command: %s", raw.Command), "Supported commands: set, delete, add, move")
	}
}

func dedupeIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func partitionObjectIDs(ids []string) ([]string, []string) {
	fileIDs := make([]string, 0, len(ids))
	embeddedIDs := make([]string, 0)
	for _, id := range ids {
		if _, _, ok := paths.ParseEmbeddedID(id); ok {
			embeddedIDs = append(embeddedIDs, id)
			continue
		}
		fileIDs = append(fileIDs, id)
	}
	return fileIDs, embeddedIDs
}
