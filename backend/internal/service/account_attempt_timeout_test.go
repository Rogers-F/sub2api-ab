//go:build unit

package service

import (
	"context"
	"testing"
)

func TestShouldFailoverOnAttemptTimeout(t *testing.T) {
	state := &AccountAttemptTimeoutState{}
	ctx := WithAccountAttemptTimeoutState(context.Background(), state)

	if ShouldFailoverOnAttemptTimeout(ctx, context.Canceled) {
		t.Fatal("did not expect failover before timeout is marked")
	}

	state.MarkTimedOut()

	if !AccountAttemptTimedOut(ctx) {
		t.Fatal("expected timeout state to be visible through context")
	}
	if !ShouldFailoverOnAttemptTimeout(ctx, context.Canceled) {
		t.Fatal("expected context canceled to trigger failover after timeout")
	}
	if !ShouldFailoverOnAttemptTimeout(ctx, context.DeadlineExceeded) {
		t.Fatal("expected deadline exceeded to trigger failover after timeout")
	}
	if ShouldFailoverOnAttemptTimeout(context.Background(), context.Canceled) {
		t.Fatal("did not expect failover without timeout state")
	}
}
