package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/sectionsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleSectionCreate executes the canonical `section create` command.
func HandleSectionCreate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()

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
	createReq := sectionsvc.CreateRequest{
		FileReference:  fileReference,
		Title:          title,
		Level:          level,
		Placement:      sectionPlacementArg(req.Args),
		Preview:        req.Preview,
		FailOnIndexErr: true,
	}
	result, warnings, failure := sectionWrite(sess, req.Preview, func(rt *vaultruntime.Runtime) (*sectionsvc.CreateResult, []string, mutation.ChangeSet, error) {
		result, err := sectionsvc.Create(rt, createReq)
		if err != nil {
			return nil, nil, mutation.ChangeSet{}, err
		}
		return result, result.WarningMessages, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	data := sectionLifecycleData(result.SectionID, result.FileRelative, result.Placement, result.AnchorID, req.Preview)
	data.Level = result.Level
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

// HandleSectionMove executes the canonical `section move` command.
func HandleSectionMove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()

	sectionID := strings.TrimSpace(stringArg(req.Args, "section_id"))
	if sectionID == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires section ID argument", nil, "Usage: rvn section move <file#section>")
	}
	moveReq := sectionsvc.MoveRequest{
		Reference:      sectionID,
		Placement:      sectionPlacementArg(req.Args),
		Preview:        req.Preview,
		FailOnIndexErr: true,
	}
	result, warnings, failure := sectionWrite(sess, req.Preview, func(rt *vaultruntime.Runtime) (*sectionsvc.MoveResult, []string, mutation.ChangeSet, error) {
		result, err := sectionsvc.Move(rt, moveReq)
		if err != nil {
			return nil, nil, mutation.ChangeSet{}, err
		}
		return result, result.WarningMessages, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	return commandexec.SuccessWithWarnings(
		sectionLifecycleData(result.SectionID, result.FileRelative, result.Placement, result.AnchorID, req.Preview),
		warnings,
		nil,
	)
}

// HandleSectionDelete executes the canonical `section delete` command.
func HandleSectionDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure(
			"MISSING_ARGUMENT",
			"requires section reference argument",
			nil,
			"Usage: rvn section delete <file#section> [--confirm]",
		)
	}
	deleteReq := sectionsvc.DeleteRequest{
		Reference:      reference,
		Preview:        req.Preview,
		FailOnIndexErr: true,
	}
	result, postWarnings, failure := sectionWrite(sess, req.Preview, func(rt *vaultruntime.Runtime) (*sectionsvc.DeleteResult, []string, mutation.ChangeSet, error) {
		result, err := sectionsvc.Delete(rt, deleteReq)
		if err != nil {
			return nil, nil, mutation.ChangeSet{}, err
		}
		return result, result.WarningMessages, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	data := sectionsvc.DeletePayload{
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
	warnings = appendCommandWarnings(warnings, postWarnings)
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

// HandleSectionRename executes the canonical `section rename` command.
func HandleSectionRename(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()

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
	renameReq := sectionsvc.RenameRequest{
		Reference:      sectionID,
		NewHeadingText: newHeadingText,
		Preview:        req.Preview,
		FailOnIndexErr: true,
	}
	result, warnings, failure := sectionWrite(sess, req.Preview, func(rt *vaultruntime.Runtime) (*sectionsvc.RenameResult, []string, mutation.ChangeSet, error) {
		result, err := sectionsvc.Rename(rt, renameReq)
		if err != nil {
			return nil, nil, mutation.ChangeSet{}, err
		}
		return result, result.WarningMessages, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	data := sectionsvc.RenamePayload{
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

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func sectionWrite[T any](sess *vaultsession.Session, preview bool, fn func(*vaultruntime.Runtime) (T, []string, mutation.ChangeSet, error)) (T, []commandexec.Warning, commandexec.Result) {
	if preview {
		value, messages, _, err := fn(sess.Runtime())
		if err != nil {
			var zero T
			return zero, nil, commandexec.FromServiceError(err)
		}
		return value, warningMessagesToCommandWarnings(messages, indexUpdateFailedWarningCode), commandexec.Result{}
	}
	var messages []string
	value, proj, failure := mutateMapped(sess, func(rt *vaultruntime.Runtime) (T, mutation.ChangeSet, error) {
		v, msgs, cs, err := fn(rt)
		messages = msgs
		return v, cs, err
	}, commandexec.FromServiceError)
	return value, appendCommandWarnings(warningMessagesToCommandWarnings(messages, indexUpdateFailedWarningCode), proj.Warnings), failure
}

func sectionPlacementArg(args map[string]any) sectionsvc.Placement {
	return sectionsvc.Placement{
		After:  strings.TrimSpace(stringArg(args, "after")),
		Before: strings.TrimSpace(stringArg(args, "before")),
		Under:  strings.TrimSpace(stringArg(args, "under")),
	}
}

func sectionLifecycleData(sectionID, file, placement, anchor string, preview bool) sectionsvc.LifecycleResult {
	data := sectionsvc.LifecycleResult{
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
