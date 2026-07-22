package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/sectionsvc"
)

// HandleSectionCreate executes the canonical `section create` command.
func HandleSectionCreate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	fileReference := strings.TrimSpace(stringArg(req.Args, "file"))
	title := strings.TrimSpace(stringArg(req.Args, "title"))
	level, hasLevel := intArg(req.Args, "level")
	if fileReference == "" || title == "" {
		return commandexec.Failure(
			"MISSING_ARGUMENT",
			"requires file and title arguments",
			nil,
			`Usage: rvn section create <file> "<title>" --level N`,
		)
	}
	if !hasLevel {
		return commandexec.Failure("MISSING_ARGUMENT", "--level is required", nil, `Usage: rvn section create <file> "<title>" --level N`)
	}

	result, err := sectionsvc.Create(sectionsvc.CreateRequest{
		VaultPath:      vaultPath,
		VaultConfig:    rt.VaultCfg,
		Schema:         rt.Schema,
		FileReference:  fileReference,
		Title:          title,
		Level:          level,
		Placement:      sectionPlacementArg(req.Args),
		Preview:        req.Preview,
		ParseOptions:   parser.OptionsFromVaultConfig(rt.VaultCfg),
		FailOnIndexErr: true,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := sectionLifecycleData(result.SectionID, result.FileRelative, result.Placement, result.AnchorID, req.Preview)
	data["level"] = result.Level
	return commandexec.SuccessWithWarnings(
		data,
		warningMessagesToCommandWarnings(result.WarningMessages, indexUpdateFailedWarningCode),
		nil,
	)
}

// HandleSectionMove executes the canonical `section move` command.
func HandleSectionMove(_ context.Context, req commandexec.Request) commandexec.Result {
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
	if sectionID == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires section ID argument", nil, "Usage: rvn section move <file#section>")
	}

	result, err := sectionsvc.Move(sectionsvc.MoveRequest{
		VaultPath:      vaultPath,
		VaultConfig:    rt.VaultCfg,
		Schema:         rt.Schema,
		Reference:      sectionID,
		Placement:      sectionPlacementArg(req.Args),
		Preview:        req.Preview,
		ParseOptions:   parser.OptionsFromVaultConfig(rt.VaultCfg),
		FailOnIndexErr: true,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.SuccessWithWarnings(
		sectionLifecycleData(result.SectionID, result.FileRelative, result.Placement, result.AnchorID, req.Preview),
		warningMessagesToCommandWarnings(result.WarningMessages, indexUpdateFailedWarningCode),
		nil,
	)
}

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
		ParseOptions:   parser.OptionsFromVaultConfig(rt.VaultCfg),
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

func sectionPlacementArg(args map[string]any) sectionsvc.Placement {
	return sectionsvc.Placement{
		After:  strings.TrimSpace(stringArg(args, "after")),
		Before: strings.TrimSpace(stringArg(args, "before")),
		Under:  strings.TrimSpace(stringArg(args, "under")),
	}
}

func sectionLifecycleData(sectionID, file, placement, anchor string, preview bool) map[string]interface{} {
	data := map[string]interface{}{
		"section":   sectionID,
		"file":      file,
		"placement": placement,
	}
	if anchor != "" {
		data["anchor"] = anchor
	}
	if preview {
		data["preview"] = true
		data["status"] = "preview"
	}
	return data
}
