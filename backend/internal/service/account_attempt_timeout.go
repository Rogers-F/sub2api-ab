package service

import (
	"context"
	"errors"
	"sync/atomic"
)

type accountAttemptTimeoutStateKey struct{}

// AccountAttemptTimeoutState tracks whether the current upstream attempt was
// canceled by the local account-level timeout instead of the parent request.
type AccountAttemptTimeoutState struct {
	timedOut atomic.Bool
}

// MarkTimedOut marks the current attempt as timed out by the local timer.
func (s *AccountAttemptTimeoutState) MarkTimedOut() {
	if s != nil {
		s.timedOut.Store(true)
	}
}

// TimedOut reports whether the local attempt timer fired.
func (s *AccountAttemptTimeoutState) TimedOut() bool {
	return s != nil && s.timedOut.Load()
}

// WithAccountAttemptTimeoutState attaches attempt-timeout state to context.
func WithAccountAttemptTimeoutState(ctx context.Context, state *AccountAttemptTimeoutState) context.Context {
	if ctx == nil || state == nil {
		return ctx
	}
	return context.WithValue(ctx, accountAttemptTimeoutStateKey{}, state)
}

// AccountAttemptTimedOut reports whether the current upstream attempt was
// canceled by the local account-level timeout.
func AccountAttemptTimedOut(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	state, _ := ctx.Value(accountAttemptTimeoutStateKey{}).(*AccountAttemptTimeoutState)
	return state != nil && state.TimedOut()
}

// ShouldFailoverOnAttemptTimeout reports whether a request error should be
// treated as a retryable failover caused by the local per-attempt timeout.
func ShouldFailoverOnAttemptTimeout(ctx context.Context, err error) bool {
	if err == nil || !AccountAttemptTimedOut(ctx) {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
