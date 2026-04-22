package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func captureUsageFailoverSourceAccountID(ctx context.Context, selectedAccountID int64) *int64 {
	if accountID, ok := service.FailoverSourceAccountIDFromContext(ctx); ok && accountID > 0 && accountID != selectedAccountID {
		value := accountID
		return &value
	}
	return nil
}
