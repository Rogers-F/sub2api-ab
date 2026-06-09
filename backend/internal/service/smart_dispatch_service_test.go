package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSmartDispatchAccountRepo struct {
	byGroup     []Account
	byPlatform  []Account
	byPlatforms []Account
}

func (f *fakeSmartDispatchAccountRepo) ListSchedulableByGroupID(_ context.Context, _ int64) ([]Account, error) {
	return f.byGroup, nil
}

func (f *fakeSmartDispatchAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, _ string) ([]Account, error) {
	return f.byPlatform, nil
}

func (f *fakeSmartDispatchAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, _ int64, _ []string) ([]Account, error) {
	return f.byPlatforms, nil
}

type fakeSmartDispatchMover struct {
	movedTarget      int64
	movedSource      int64
	movedIDs         []int64
	movedMinNormal   int
	targetNormalFunc func(minNormal int) bool
}

func (f *fakeSmartDispatchMover) MoveAccountsForSmartDispatch(_ context.Context, targetGroupID, sourceGroupID int64, accountIDs []int64, minNormalAccounts int) ([]int64, bool, error) {
	f.movedTarget = targetGroupID
	f.movedSource = sourceGroupID
	f.movedIDs = append([]int64(nil), accountIDs...)
	f.movedMinNormal = minNormalAccounts
	if f.targetNormalFunc != nil {
		return nil, f.targetNormalFunc(minNormalAccounts), nil
	}
	return accountIDs, false, nil
}

func TestSmartDispatchService_RefillMovesConfiguredCountAndSkipsExcluded(t *testing.T) {
	sourceID := int64(20)
	target := &Group{
		ID:                         10,
		Platform:                   PlatformAnthropic,
		SmartDispatchEnabled:       true,
		SmartDispatchSourceGroupID: &sourceID,
		SmartDispatchCount:         2,
	}
	accountRepo := &fakeSmartDispatchAccountRepo{
		byGroup: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 3, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
		},
	}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: target,
		ExcludedIDs: map[int64]struct{}{2: {}},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1, 3}, result.MovedAccountIDs)
	require.Equal(t, target.ID, mover.movedTarget)
	require.Equal(t, sourceID, mover.movedSource)
	require.Equal(t, []int64{1, 3}, mover.movedIDs)
	require.Equal(t, 1, mover.movedMinNormal)
}

func TestSmartDispatchService_RefillDoesNotFilterByRequestPlatformOrModel(t *testing.T) {
	sourceID := int64(20)
	target := &Group{
		ID:                         10,
		Platform:                   PlatformAnthropic,
		SmartDispatchEnabled:       true,
		SmartDispatchSourceGroupID: &sourceID,
		SmartDispatchCount:         1,
	}
	accountRepo := &fakeSmartDispatchAccountRepo{
		byGroup: []Account{
			{ID: 8, Platform: PlatformGemini, Status: StatusActive, Schedulable: true},
		},
	}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: target,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{8}, result.MovedAccountIDs)
}

func TestSmartDispatchService_RefillPassesConfiguredMinimumNormalAccounts(t *testing.T) {
	sourceID := int64(20)
	target := &Group{
		ID:                             10,
		Platform:                       PlatformAnthropic,
		SmartDispatchEnabled:           true,
		SmartDispatchSourceGroupID:     &sourceID,
		SmartDispatchCount:             2,
		SmartDispatchMinNormalAccounts: 3,
	}
	accountRepo := &fakeSmartDispatchAccountRepo{
		byGroup: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
		},
	}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: target,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, result.MovedAccountIDs)
	require.Equal(t, 3, mover.movedMinNormal)
}

func TestSmartDispatchService_RefillSelectsMinimumNormalAccountsWhenLargerThanMoveCount(t *testing.T) {
	sourceID := int64(20)
	target := &Group{
		ID:                             10,
		Platform:                       PlatformAnthropic,
		SmartDispatchEnabled:           true,
		SmartDispatchSourceGroupID:     &sourceID,
		SmartDispatchCount:             1,
		SmartDispatchMinNormalAccounts: 3,
	}
	accountRepo := &fakeSmartDispatchAccountRepo{
		byGroup: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 3, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
		},
	}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: target,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, result.MovedAccountIDs)
	require.Equal(t, []int64{1, 2, 3}, mover.movedIDs)
	require.Equal(t, 3, mover.movedMinNormal)
}

func TestSmartDispatchService_RefillTreatsZeroMinimumAsDefaultOne(t *testing.T) {
	sourceID := int64(20)
	target := &Group{
		ID:                         10,
		Platform:                   PlatformAnthropic,
		SmartDispatchEnabled:       true,
		SmartDispatchSourceGroupID: &sourceID,
		SmartDispatchCount:         1,
	}
	accountRepo := &fakeSmartDispatchAccountRepo{
		byGroup: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
		},
	}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: target,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1}, result.MovedAccountIDs)
	require.Equal(t, 1, mover.movedMinNormal)
}

func TestSmartDispatchService_RefillNoopsWhenDisabled(t *testing.T) {
	accountRepo := &fakeSmartDispatchAccountRepo{}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: &Group{ID: 10, Platform: PlatformAnthropic},
	})

	require.NoError(t, err)
	require.False(t, result.Attempted)
	require.Empty(t, mover.movedIDs)
}
