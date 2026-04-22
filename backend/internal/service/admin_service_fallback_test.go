//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type adminFallbackAccountRepoStub struct {
	mockAccountRepoForPlatform
	createCalls int
	updateCalls int
	created     *Account
	updated     *Account
}

func (r *adminFallbackAccountRepoStub) Create(ctx context.Context, account *Account) error {
	r.createCalls++
	cloned := *account
	if cloned.ID == 0 {
		cloned.ID = int64(1000 + r.createCalls)
		account.ID = cloned.ID
	}
	r.created = &cloned
	if r.accountsByID == nil {
		r.accountsByID = map[int64]*Account{}
	}
	r.accountsByID[cloned.ID] = r.created
	return nil
}

func (r *adminFallbackAccountRepoStub) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	cloned := *account
	r.updated = &cloned
	if r.accountsByID == nil {
		r.accountsByID = map[int64]*Account{}
	}
	r.accountsByID[cloned.ID] = r.updated
	return nil
}

func newAdminFallbackTestAccount(id int64, platform string) *Account {
	return &Account{
		ID:          id,
		Name:        "account",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
		Credentials: map[string]any{},
	}
}

func TestAdminService_UpdateAccount_RejectsSelfFallbackAccount(t *testing.T) {
	current := newAdminFallbackTestAccount(1, PlatformOpenAI)
	repo := &adminFallbackAccountRepoStub{
		mockAccountRepoForPlatform: mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{1: current},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{
		FallbackAccountID: ptr(int64(1)),
	})

	require.ErrorContains(t, err, "cannot set self as fallback account")
	require.Zero(t, repo.updateCalls)
}

func TestAdminService_CreateAccount_RejectsFallbackPlatformMismatch(t *testing.T) {
	repo := &adminFallbackAccountRepoStub{
		mockAccountRepoForPlatform: mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{
				2: newAdminFallbackTestAccount(2, PlatformOpenAI),
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "new-account",
		Platform:             PlatformAnthropic,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{},
		Concurrency:          1,
		Priority:             1,
		FallbackAccountID:    ptr(int64(2)),
		SkipDefaultGroupBind: true,
	})

	require.ErrorContains(t, err, "fallback account platform mismatch")
	require.Zero(t, repo.createCalls)
}

func TestAdminService_UpdateAccount_RejectsFallbackCycle(t *testing.T) {
	current := newAdminFallbackTestAccount(1, PlatformAnthropic)
	next := newAdminFallbackTestAccount(2, PlatformAnthropic)
	next.FallbackAccountID = ptr(int64(1))

	repo := &adminFallbackAccountRepoStub{
		mockAccountRepoForPlatform: mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{
				1: current,
				2: next,
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{
		FallbackAccountID: ptr(int64(2)),
	})

	require.ErrorContains(t, err, "fallback account cycle detected")
	require.Zero(t, repo.updateCalls)
}

func TestAdminService_CreateAccount_AllowsValidFallbackChain(t *testing.T) {
	second := newAdminFallbackTestAccount(2, PlatformAnthropic)
	second.FallbackAccountID = ptr(int64(3))
	third := newAdminFallbackTestAccount(3, PlatformAnthropic)

	repo := &adminFallbackAccountRepoStub{
		mockAccountRepoForPlatform: mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{
				2: second,
				3: third,
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "new-account",
		Platform:             PlatformAnthropic,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{},
		Concurrency:          1,
		Priority:             1,
		FallbackAccountID:    ptr(int64(2)),
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.created)
	require.NotNil(t, repo.created.FallbackAccountID)
	require.Equal(t, int64(2), *repo.created.FallbackAccountID)
}

func TestAdminService_UpdateAccount_ClearsFallbackAccount(t *testing.T) {
	current := newAdminFallbackTestAccount(1, PlatformAnthropic)
	current.FallbackAccountID = ptr(int64(2))

	repo := &adminFallbackAccountRepoStub{
		mockAccountRepoForPlatform: mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{1: current},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	account, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{
		FallbackAccountID: ptr(int64(0)),
	})

	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, 1, repo.updateCalls)
	require.Nil(t, account.FallbackAccountID)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackAccountID)
}
