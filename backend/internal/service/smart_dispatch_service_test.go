package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSmartDispatchAccountRepo struct {
	byPlatform  []Account
	byPlatforms []Account
}

func (f *fakeSmartDispatchAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, _ string) ([]Account, error) {
	return f.byPlatform, nil
}

func (f *fakeSmartDispatchAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, _ int64, _ []string) ([]Account, error) {
	return f.byPlatforms, nil
}

type fakeSmartDispatchMover struct {
	movedTarget int64
	movedSource int64
	movedIDs    []int64
}

func (f *fakeSmartDispatchMover) MoveAccountsForSmartDispatch(_ context.Context, targetGroupID, sourceGroupID int64, accountIDs []int64) ([]int64, bool, error) {
	f.movedTarget = targetGroupID
	f.movedSource = sourceGroupID
	f.movedIDs = append([]int64(nil), accountIDs...)
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
		byPlatforms: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
			{ID: 3, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true},
		},
	}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup:    target,
		Platform:       PlatformAnthropic,
		UseMixed:       true,
		ExcludedIDs:    map[int64]struct{}{2: {}},
		CandidateAllow: func(account *Account) bool { return account != nil && account.ID != 3 },
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1}, result.MovedAccountIDs)
	require.Equal(t, target.ID, mover.movedTarget)
	require.Equal(t, sourceID, mover.movedSource)
	require.Equal(t, []int64{1}, mover.movedIDs)
}

func TestSmartDispatchService_RefillNoopsWhenDisabled(t *testing.T) {
	accountRepo := &fakeSmartDispatchAccountRepo{}
	mover := &fakeSmartDispatchMover{}
	svc := NewSmartDispatchService(accountRepo, mover)

	result, err := svc.Refill(context.Background(), SmartDispatchRefillRequest{
		TargetGroup: &Group{ID: 10, Platform: PlatformAnthropic},
		Platform:    PlatformAnthropic,
	})

	require.NoError(t, err)
	require.False(t, result.Attempted)
	require.Empty(t, mover.movedIDs)
}
