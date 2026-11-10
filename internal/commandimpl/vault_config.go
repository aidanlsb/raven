package commandimpl

import (
	"context"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/vaultconfigsvc"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

func HandleVaultConfigShow(_ context.Context, req commandexec.Request) commandexec.Result {
	sess, failure := openSession(req.VaultPath, vaultsession.Lazy())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	result, err := vaultconfigsvc.Show(sess.Runtime())
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(*result, &commandexec.Meta{Count: result.ProtectedPrefixesCount + result.ExcludeCount})
}
