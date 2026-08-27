package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/sectionsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// HandleSectionCreate executes the canonical `section create` command.
func HandleSectionCreate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

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
		ParseOptions:   rt.ParseOptions,
		FailOnIndexErr: true,
		Runtime:        rt,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := sectionLifecycleData(result.SectionID, result.FileRelative, result.Placement, result.AnchorID, req.Preview)
	data.Level = result.Level
	return commandexec.SuccessWithWarnings(
		data,
		sectionCommandWarnings(rt, result.WarningMessages, result.IndexWarnings),
		nil,
	)
}

// HandleSectionMove executes the canonical `section move` command.
func HandleSectionMove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

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
		ParseOptions:   rt.ParseOptions,
		FailOnIndexErr: true,
		Runtime:        rt,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.SuccessWithWarnings(
		sectionLifecycleData(result.SectionID, result.FileRelative, result.Placement, result.AnchorID, req.Preview),
		sectionCommandWarnings(rt, result.WarningMessages, result.IndexWarnings),
		nil,
	)
}

// HandleSectionDelete executes the canonical `section delete` command.
func HandleSectionDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure(
			"MISSING_ARGUMENT",
			"requires section reference argument",
			nil,
			"Usage: rvn section delete <file#section> [--confirm]",
		)
	}
	result, err := sectionsvc.Delete(sectionsvc.DeleteRequest{
		VaultPath:      vaultPath,
		VaultConfig:    rt.VaultCfg,
		Schema:         rt.Schema,
		Reference:      reference,
		Preview:        req.Preview,
		ParseOptions:   rt.ParseOptions,
		FailOnIndexErr: true,
		Runtime:        rt,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := commandpayload.SectionDeleteResult{
		Section:         result.SectionID,
		File:            result.FileRelative,
		LineStart:       result.LineStart,
		LineEnd:         result.LineEnd,
		RemovedContent:  result.RemovedContent,
		DeletedSections: result.DeletedSections,
		Backlinks:       result.Backlinks,
	}
	if req.Preview {
		data.Preview = true
		data.Status = "preview"
	} else {
		data.Status = "deleted"
	}

	warnings := deleteBacklinkCommandWarnings(result.Backlinks)
	warnings = appendCommandWarnings(warnings, sectionCommandWarnings(rt, result.WarningMessages, result.IndexWarnings))
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

// HandleSectionRename executes the canonical `section rename` command.
func HandleSectionRename(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

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
		ParseOptions:   rt.ParseOptions,
		FailOnIndexErr: true,
		Runtime:        rt,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := commandpayload.SectionRenameResult{
		Source:      result.SourceID,
		Destination: result.DestinationID,
	}
	if req.Preview {
		data.Preview = true
		data.Status = "preview"
	}
	if len(result.UpdatedRefs) > 0 {
		data.UpdatedRefs = result.UpdatedRefs
	}

	return commandexec.SuccessWithWarnings(
		data,
		sectionCommandWarnings(rt, result.WarningMessages, result.IndexWarnings),
		nil,
	)
}

func sectionCommandWarnings(rt *vaultruntime.Runtime, warningMessages []string, indexWarnings []sectionsvc.IndexWarning) []commandexec.Warning {
	warnings := warningMessagesToCommandWarnings(warningMessages, indexUpdateFailedWarningCode)
	for _, indexWarning := range indexWarnings {
		warnings = append(warnings, projectionCommandWarnings(reindexsvc.RecordProjectionFailure(
			rt,
			indexWarning.FilePath,
			indexWarning.Stage,
			indexWarning.Err,
		))...)
	}
	return warnings
}

func sectionPlacementArg(args map[string]any) sectionsvc.Placement {
	return sectionsvc.Placement{
		After:  strings.TrimSpace(stringArg(args, "after")),
		Before: strings.TrimSpace(stringArg(args, "before")),
		Under:  strings.TrimSpace(stringArg(args, "under")),
	}
}

func sectionLifecycleData(sectionID, file, placement, anchor string, preview bool) commandpayload.SectionLifecycleResult {
	data := commandpayload.SectionLifecycleResult{
		Section:   sectionID,
		File:      file,
		Placement: placement,
	}
	if anchor != "" {
		data.Anchor = anchor
	}
	if preview {
		data.Preview = true
		data.Status = "preview"
	}
	return data
}
