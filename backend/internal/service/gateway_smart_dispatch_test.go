//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeGatewaySmartDispatcher struct {
	calls    int
	onRefill func(req SmartDispatchRefillRequest)
}

func (f *fakeGatewaySmartDispatcher) Refill(_ context.Context, req SmartDispatchRefillRequest) (*SmartDispatchRefillResult, error) {
	f.calls++
	if f.onRefill != nil {
		f.onRefill(req)
	}
	return &SmartDispatchRefillResult{Attempted: true, MovedAccountIDs: []int64{9}}, nil
}

func TestGatewayService_SelectAccountForModelWithExclusions_SmartDispatchRetry(t *testing.T) {
	groupID := int64(10)
	sourceID := int64(20)
	accountRepo := &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{},
	}
	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{
			groupID: {
				ID:                         groupID,
				Platform:                   PlatformAnthropic,
				Status:                     StatusActive,
				Hydrated:                   true,
				SmartDispatchEnabled:       true,
				SmartDispatchSourceGroupID: &sourceID,
				SmartDispatchCount:         1,
			},
		},
	}
	dispatcher := &fakeGatewaySmartDispatcher{
		onRefill: func(req SmartDispatchRefillRequest) {
			require.Equal(t, groupID, req.TargetGroup.ID)
			require.Equal(t, PlatformAnthropic, req.Platform)
			acc := Account{
				ID:          9,
				Name:        "moved",
				Platform:    PlatformAnthropic,
				Status:      StatusActive,
				Schedulable: true,
				AccountGroups: []AccountGroup{
					{GroupID: groupID},
				},
			}
			accountRepo.accounts = []Account{acc}
			accountRepo.accountsByID[acc.ID] = &acc
		},
	}
	svc := &GatewayService{
		accountRepo:     accountRepo,
		groupRepo:       groupRepo,
		cfg:             testConfig(),
		smartDispatcher: dispatcher,
	}

	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "", "claude-sonnet-4-5", nil)

	require.NoError(t, err)
	require.Equal(t, int64(9), account.ID)
	require.Equal(t, 1, dispatcher.calls)
}
