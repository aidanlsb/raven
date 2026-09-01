package commandimpl

import (
	"context"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/vaultconfigsvc"
)

func HandleVaultConfigShow(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.Show(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigShowResult{
		ConfigPath:          result.ConfigPath,
		Exists:              result.Exists,
		AutoReindex:         result.AutoReindex,
		AutoReindexExplicit: result.AutoReindexExplicit,
		DailyTemplate:       result.DailyTemplate,
		Directories: commandpayload.VaultDirectories{
			Configured: result.Directories.Configured,
			Daily:      result.Directories.Daily,
			Type:       result.Directories.Object,
			Page:       result.Directories.Page,
			Template:   result.Directories.Template,
		},
		Capture: commandpayload.VaultCapture{
			Destination: result.Capture.Destination,
			Heading:     result.Capture.Heading,
		},
		Deletion: commandpayload.VaultDeletion{
			Behavior: result.Deletion.Behavior,
			TrashDir: result.Deletion.TrashDir,
		},
		QueriesCount:           result.QueriesCount,
		ProtectedPrefixes:      result.ProtectedPrefixes,
		ProtectedPrefixesCount: len(result.ProtectedPrefixes),
		Exclude:                result.Exclude,
		ExcludeCount:           len(result.Exclude),
	}, &commandexec.Meta{Count: len(result.ProtectedPrefixes) + len(result.Exclude)})
}
