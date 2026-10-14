package localapi

import (
	"context"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/index"
)

const (
	defaultIndexLockRetry   = 2 * time.Second
	defaultIndexLockBackoff = 50 * time.Millisecond
	maxIndexLockBackoff     = 500 * time.Millisecond
)

func executeWithIndexLockRetry(ctx context.Context, budget, initialBackoff time.Duration, execute func() commandexec.Result) commandexec.Result {
	if budget <= 0 {
		budget = defaultIndexLockRetry
	}
	if initialBackoff <= 0 {
		initialBackoff = defaultIndexLockBackoff
	}

	start := time.Now()
	delay := initialBackoff
	for {
		result := execute()
		if !resultIndicatesIndexLocked(result) {
			return result
		}
		remaining := budget - time.Since(start)
		if remaining <= 0 {
			return result
		}
		if delay > remaining {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result
		case <-timer.C:
		}
		delay *= 2
		if delay > maxIndexLockBackoff {
			delay = maxIndexLockBackoff
		}
	}
}

func resultIndicatesIndexLocked(result commandexec.Result) bool {
	if result.OK || result.Error == nil {
		return false
	}
	if result.Error.Code != codes.ErrDatabase {
		return false
	}
	msg := strings.ToLower(result.Error.Message)
	return strings.Contains(msg, strings.ToLower(index.ErrIndexLocked.Error()))
}
