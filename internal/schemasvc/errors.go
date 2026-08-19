package schemasvc

import (
	"github.com/aidanlsb/raven/internal/svcerr"
)

// Error is kept as a compatibility alias for schema migration callers that
// previously matched schemasvc's package-local error type.
type Error = svcerr.Error
