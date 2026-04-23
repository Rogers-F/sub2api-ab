package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func captureUsageFailoverSourceAccountIDFromID(sourceAccountID, selectedAccountID int64) *int64 {
	if sourceAccountID <= 0 || sourceAccountID == selectedAccountID {
		return nil
	}
	value := sourceAccountID
	return &value
}

func captureUsageFailoverSourceAccountID(ctx context.Context, selectedAccountID int64) *int64 {
	if accountID, ok := service.FailoverSourceAccountIDFromContext(ctx); ok {
		return captureUsageFailoverSourceAccountIDFromID(accountID, selectedAccountID)
	}
	return nil
}
