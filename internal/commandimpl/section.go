package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/sectionsvc"
)

// HandleSectionRename executes the canonical `section rename` command.
func HandleSectionRename(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	sectionID := strings.TrimSpace(stringArg(req.Args, "section_id"))
	newHeadingText := strings.TrimSpace(stringArg(req.Args, "new_heading_text"))
	if sectionID == "" || newHeadingText == "" {
		return commandexec.Failure(
			"MISSING_ARGUMENT",
			"requires section ID and new heading text arguments",
			nil,
			`Usage: rvn section rename <file#section> "<new heading text>"`,
		)
	}

	result, err := sectionsvc.Rename(sectionsvc.RenameRequest{
		VaultPath:      vaultPath,
		VaultConfig:    rt.VaultCfg,
		Schema:         rt.Schema,
		Reference:      sectionID,
		NewHeadingText: newHeadingText,
		Preview:        req.Preview,
		ParseOptions:   buildParseOptions(rt.VaultCfg),
		FailOnIndexErr: true,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := map[string]interface{}{
		"source":      result.SourceID,
		"destination": result.DestinationID,
	}
	if req.Preview {
		data["preview"] = true
		data["status"] = "preview"
	}
	if len(result.UpdatedRefs) > 0 {
		data["updated_refs"] = result.UpdatedRefs
	}

	return commandexec.SuccessWithWarnings(
		data,
		warningMessagesToCommandWarnings(result.WarningMessages, indexUpdateFailedWarningCode),
		nil,
	)
}
