package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleObjects executes the canonical `objects` command: a cheap index-backed
// listing of every file-backed object without bodies.
func HandleObjects(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	sess, failure := openSession(req.VaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	limit, _ := intArg(req.Args, "limit")
	offset, _ := intArg(req.Args, "offset")
	result, err := readsvc.ListObjects(rt, limit, offset)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	payload := result.ObjectsPayload()
	out := commandexec.Success(payload, &commandexec.Meta{Count: len(payload.Items), QueryTimeMs: time.Since(start).Milliseconds()})
	return withIndexRefreshWarnings(refreshWarnings, withSchemaLoadWarning(rt, out))
}
