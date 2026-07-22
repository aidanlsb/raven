package checksvc

import (
	"errors"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestValidationErrorUsesSharedContract(t *testing.T) {
	t.Parallel()

	cause := errors.New("walk failed")
	err := validationError(cause)
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("validationError() = %T, want *svcerr.Error", err)
	}
	if svcErr.Code != codes.ErrValidationFailed || svcErr.Message != cause.Error() {
		t.Fatalf("validationError() = %#v", svcErr)
	}
	if !errors.Is(err, cause) {
		t.Fatal("validationError() should preserve its cause")
	}
}
