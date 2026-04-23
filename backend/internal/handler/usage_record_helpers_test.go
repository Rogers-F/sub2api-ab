package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

func TestCaptureUsageFailoverSourceAccountIDFromID(t *testing.T) {
	t.Run("returns source account when different from selected account", func(t *testing.T) {
		got := captureUsageFailoverSourceAccountIDFromID(16, 18)
		require.NotNil(t, got)
		require.Equal(t, int64(16), *got)
	})

	t.Run("returns nil when source account is zero", func(t *testing.T) {
		require.Nil(t, captureUsageFailoverSourceAccountIDFromID(0, 18))
	})

	t.Run("returns nil when source account equals selected account", func(t *testing.T) {
		require.Nil(t, captureUsageFailoverSourceAccountIDFromID(18, 18))
	})
}

func TestCaptureUsageFailoverSourceAccountID(t *testing.T) {
	t.Run("reads failover source account from context", func(t *testing.T) {
		ctx := service.WithFailoverSourceAccountID(context.Background(), 16, false)
		got := captureUsageFailoverSourceAccountID(ctx, 18)
		require.NotNil(t, got)
		require.Equal(t, int64(16), *got)
	})

	t.Run("returns nil when selected account matches failover source account", func(t *testing.T) {
		ctx := service.WithFailoverSourceAccountID(context.Background(), 16, false)
		require.Nil(t, captureUsageFailoverSourceAccountID(ctx, 16))
	})
}
