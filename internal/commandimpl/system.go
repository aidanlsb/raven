package commandimpl

import (
	"context"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/initsvc"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// DateAssociation remains an alias for CLI rendering compatibility.
type DateAssociation = readsvc.DateAssociation

// HandleInit executes the canonical `init` command.
func HandleInit(_ context.Context, req commandexec.Request) commandexec.Result {
	path := strings.TrimSpace(stringArg(req.Args, "path"))
	if path == "" {
		return commandexec.Failure(codes.ErrMissingArgument, "path is required", nil, "Usage: rvn init <path>")
	}
	result, err := initsvc.Initialize(
		path,
		req.ConfigPath,
		req.StatePath,
		versioninfo.CurrentVersionInfo().Version,
	)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	warnings := make([]commandexec.Warning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, commandexec.Warning{Code: warning.Code, Message: warning.Message})
	}
	return commandexec.SuccessWithWarnings(result.Data(), warnings, nil)
}

// HandleReindex executes the canonical `reindex` command.
func HandleReindex(ctx context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newSchemaFirstCommandVaultRuntime(strings.TrimSpace(req.VaultPath))
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	start := time.Now()
	result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{
		VaultPath: rt.VaultPath,
		Full:      boolArg(req.Args, "full"),
		DryRun:    boolArg(req.Args, "dry-run"),
		Context:   ctx,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	warnings := make([]commandexec.Warning, 0, len(result.WarningMessages))
	for _, warning := range result.WarningMessages {
		code := indexUpdateFailedWarningCode
		if strings.Contains(warning, "unknown frontmatter key") {
			code = codes.WarnUnknownField
		}
		warnings = append(warnings, commandexec.Warning{Code: code, Message: warning})
	}
	return commandexec.SuccessWithWarnings(result.Data(), warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleDaily executes the canonical `daily` command.
func HandleDaily(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newConfigCommandVaultRuntime(strings.TrimSpace(req.VaultPath))
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := objectsvc.EnsureDaily(rt, stringArg(req.Args, "date"), stringArg(req.Args, "template"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(map[string]interface{}{
		"file": result.RelativePath, "id": result.Date, "date": result.Date,
		"created": result.Created, "opened": false,
	}, nil)
}

// HandleDate executes the canonical `date` command.
func HandleDate(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newConfigOnlyCommandVaultRuntime(strings.TrimSpace(req.VaultPath))
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := readsvc.Date(rt, stringArg(req.Args, "date"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := map[string]interface{}{
		"date": result.Date, "day_of_week": result.DayOfWeek,
		"daily_note_id": result.DailyNoteID, "daily_path": result.DailyPath,
		"daily_exists": result.DailyExists, "items": result.Items, "backlinks": result.Backlinks,
	}
	if result.DailyNote != nil {
		data["daily_note"] = result.DailyNote
	}
	return commandexec.Success(data, &commandexec.Meta{Count: len(result.Items)})
}

// HandleVersion executes the canonical `version` command.
func HandleVersion(_ context.Context, req commandexec.Request) commandexec.Result {
	info := versioninfo.Current()
	if strings.TrimSpace(req.ExecutablePath) != "" {
		info = versioninfo.CurrentVersionInfoFromExecutable(req.ExecutablePath)
	}
	return commandexec.Success(map[string]interface{}{
		"version": info.Version, "module_path": info.ModulePath, "commit": info.Commit,
		"commit_time": info.CommitTime, "modified": info.Modified, "go_version": info.GoVersion,
		"goos": info.GOOS, "goarch": info.GOARCH,
	}, nil)
}
