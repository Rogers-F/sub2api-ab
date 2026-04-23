package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestWithAccountAttemptContext_DisabledReturnsOriginalContext(t *testing.T) {
	baseCtx := context.Background()
	account := &service.Account{}

	attemptCtx, cleanup := withAccountAttemptContext(baseCtx, account, false, 0, false)
	defer cleanup()

	if attemptCtx != baseCtx {
		t.Fatal("expected disabled helper to reuse original context")
	}
	if service.AccountAttemptTimedOut(attemptCtx) {
		t.Fatal("did not expect timeout state on original context")
	}
}

func TestWithAccountAttemptContext_TimeoutCancelsAttemptOnly(t *testing.T) {
	originalTimeout := nonStreamForceFailoverTimeout
	nonStreamForceFailoverTimeout = 20 * time.Millisecond
	defer func() {
		nonStreamForceFailoverTimeout = originalTimeout
	}()

	fallbackID := int64(2)
	baseCtx := context.Background()
	account := &service.Account{
		FallbackAccountID: &fallbackID,
		Extra: map[string]any{
			"non_stream_force_failover_enabled": true,
		},
	}

	attemptCtx, cleanup := withAccountAttemptContext(baseCtx, account, false, 0, false)
	defer cleanup()

	if attemptCtx == baseCtx {
		t.Fatal("expected enabled helper to wrap original context")
	}

	select {
	case <-attemptCtx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected attempt context to be canceled by timeout")
	}

	if !service.AccountAttemptTimedOut(attemptCtx) {
		t.Fatal("expected attempt timeout state to be marked")
	}
	if baseCtx.Err() != nil {
		t.Fatalf("expected parent context to remain active, got %v", baseCtx.Err())
	}
}
