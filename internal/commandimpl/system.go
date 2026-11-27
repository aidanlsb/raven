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
	"github.com/aidanlsb/raven/internal/vaultsession"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

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
	return commandexec.SuccessWithWarnings(result.Data(), result.Warnings, nil)
}

// HandleReindex executes the canonical `reindex` command.
func HandleReindex(ctx context.Context, req commandexec.Request) commandexec.Result {
	sess, failure := openSession(strings.TrimSpace(req.VaultPath), vaultsession.SchemaFirst())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()

	start := time.Now()
	result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{
		Full:    boolArg(req.Args, "full"),
		DryRun:  boolArg(req.Args, "dry-run"),
		Context: ctx,
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
	sess, failure := openSession(strings.TrimSpace(req.VaultPath), vaultsession.Config())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	result, err := objectsvc.EnsureDaily(rt, stringArg(req.Args, "date"), stringArg(req.Args, "template"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(result.Payload(), nil)
}

// HandleDate executes the canonical `date` command.
func HandleDate(_ context.Context, req commandexec.Request) commandexec.Result {
	sess, failure := openSession(strings.TrimSpace(req.VaultPath), vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()
	result, err := readsvc.Date(rt, stringArg(req.Args, "date"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(result.Payload(), &commandexec.Meta{Count: len(result.Items)}))
}

// HandleVersion executes the canonical `version` command.
func HandleVersion(_ context.Context, req commandexec.Request) commandexec.Result {
	info := versioninfo.Current()
	if strings.TrimSpace(req.ExecutablePath) != "" {
		info = versioninfo.CurrentVersionInfoFromExecutable(req.ExecutablePath)
	}
	return commandexec.Success(info, nil)
}
