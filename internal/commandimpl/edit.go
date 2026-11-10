package commandimpl

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/editsvc"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleEdit executes the canonical `edit` command.
func HandleEdit(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	// Edit resolves the target reference (schema-aware) before mutating and
	// then reports missing-reference warnings, so require a valid schema.
	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn edit <reference> <old_str> <new_str> or --edits-json")
	}

	edits, batchMode, err := parseCanonicalEditInput(req.Args)
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	runReq := editsvc.RunRequest{
		Reference: reference,
		Edits:     edits,
		Preview:   req.Preview,
	}
	if req.Preview {
		result, err := editsvc.Run(sess.Runtime(), runReq)
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		if batchMode {
			editsPreview := make([]editsvc.EditPreviewItem, 0, len(result.Edits))
			for _, editResult := range result.Edits {
				editsPreview = append(editsPreview, editsvc.EditPreviewItem{
					Index:  editResult.Index,
					Line:   editResult.Line,
					OldStr: editResult.OldStr,
					NewStr: editResult.NewStr,
					Preview: editsvc.EditPreview{
						Before: editResult.Before,
						After:  editResult.After,
					},
				})
			}
			return commandexec.Success(editsvc.EditBatchPreviewResult{
				Status: "preview",
				Path:   result.Path,
				Count:  len(editsPreview),
				Edits:  editsPreview,
			}, nil)
		}

		editResult := result.Edits[0]
		return commandexec.Success(editsvc.EditSinglePreviewResult{
			Status: "preview",
			Path:   result.Path,
			Line:   editResult.Line,
			Preview: editsvc.EditPreview{
				Before: editResult.Before,
				After:  editResult.After,
			},
		}, nil)
	}

	runReq.Preview = false
	result, proj, failure := mutateMapped(sess, func(rt *vaultruntime.Runtime) (*editsvc.RunResult, mutation.ChangeSet, error) {
		result, err := editsvc.Run(rt, runReq)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	}, commandexec.FromServiceError)
	if failure.Error != nil {
		return failure
	}
	missingRefs := missingRefsOf(proj)
	warnings := proj.Warnings

	if batchMode {
		applied := make([]editsvc.EditAppliedItem, 0, len(result.Edits))
		for _, editResult := range result.Edits {
			applied = append(applied, editsvc.EditAppliedItem{
				Index:   editResult.Index,
				Line:    editResult.Line,
				OldStr:  editResult.OldStr,
				NewStr:  editResult.NewStr,
				Context: editResult.Context,
			})
		}
		data := editsvc.EditBatchResult{
			Status:            "applied",
			Path:              result.Path,
			Count:             len(applied),
			Edits:             applied,
			MissingReferences: missingRefs,
		}
		return commandexec.SuccessWithWarnings(data, warnings, nil)
	}

	editResult := result.Edits[0]
	data := editsvc.EditSingleResult{
		Status:            "applied",
		Path:              result.Path,
		Line:              editResult.Line,
		OldStr:            editResult.OldStr,
		NewStr:            editResult.NewStr,
		Context:           editResult.Context,
		MissingReferences: missingRefs,
	}
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func parseCanonicalEditInput(args map[string]any) ([]editsvc.EditSpec, bool, error) {
	if raw, ok := args["edits-json"]; ok {
		var payload string
		switch v := raw.(type) {
		case string:
			payload = v
		default:
			encoded, err := json.Marshal(v)
			if err != nil {
				return nil, false, &svcerr.Error{
					Code:       codes.ErrInvalidInput,
					Message:    "invalid --edits-json payload",
					Suggestion: `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`,
					Details:    map[string]any{"error": err.Error()},
					Err:        err,
				}
			}
			payload = string(encoded)
		}

		edits, err := editsvc.ParseEditsJSON(strings.TrimSpace(payload))
		if err != nil {
			return nil, false, err
		}
		return edits, true, nil
	}

	oldStr := stringArg(args, "old_str")
	newStr, hasNew := args["new_str"]
	if oldStr == "" || !hasNew {
		return nil, false, &svcerr.Error{
			Code:       codes.ErrInvalidInput,
			Message:    "requires old_str and new_str when --edits-json is not provided",
			Suggestion: "Usage: rvn edit <reference> <old_str> <new_str> or --edits-json",
		}
	}

	return []editsvc.EditSpec{{
		OldStr: oldStr,
		NewStr: toAnyString(newStr),
	}}, false, nil
}

func toAnyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
