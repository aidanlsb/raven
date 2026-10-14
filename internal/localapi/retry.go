package localapi

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func (s *Server) executeWithIndexLockRetry(ctx context.Context, req commandexec.Request) commandexec.Result {
	result := s.invoker.Execute(ctx, req)
	if !isIndexLockedResult(result) {
		return result
	}

	for _, wait := range s.retryBackoff {
		if ctx.Err() != nil {
			return result
		}
		if err := s.sleep(ctx, wait); err != nil {
			return result
		}
		result = s.invoker.Execute(ctx, req)
		if !isIndexLockedResult(result) {
			return result
		}
	}
	return result
}

func isIndexLockedResult(result commandexec.Result) bool {
	if result.OK || result.Error == nil {
		return false
	}
	if result.Error.Code == codes.ErrIndexLocked {
		return true
	}
	if result.Error.Code == codes.ErrDatabase &&
		strings.Contains(strings.ToLower(result.Error.Message), "index is locked") {
		return true
	}
	return false
}
