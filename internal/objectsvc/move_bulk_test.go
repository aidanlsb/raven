package objectsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestMoveBulkRejectsSectionSources(t *testing.T) {
	t.Parallel()

	request := MoveBulkRequest{
		VaultPath:      t.TempDir(),
		VaultConfig:    config.DefaultVaultConfig(),
		ObjectIDs:      []string{"projects/site", "projects/site#tasks"},
		DestinationDir: "archive/",
	}

	if _, err := PreviewMoveBulk(request); err == nil || !strings.Contains(err.Error(), "does not accept section sources") {
		t.Fatalf("PreviewMoveBulk() error = %v, want section-source rejection", err)
	}
	if _, err := ApplyMoveBulk(request); err == nil || !strings.Contains(err.Error(), "does not accept section sources") {
		t.Fatalf("ApplyMoveBulk() error = %v, want section-source rejection", err)
	}
}
