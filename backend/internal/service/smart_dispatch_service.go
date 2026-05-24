package service

import (
	"context"
	"fmt"
)

type SmartDispatchAccountLister interface {
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error)
}

type SmartDispatchMover interface {
	MoveAccountsForSmartDispatch(ctx context.Context, targetGroupID, sourceGroupID int64, accountIDs []int64) (movedIDs []int64, targetAlreadyNormal bool, err error)
}

type SmartDispatcher interface {
	Refill(ctx context.Context, req SmartDispatchRefillRequest) (*SmartDispatchRefillResult, error)
}

type SmartDispatchService struct {
	accountRepo SmartDispatchAccountLister
	mover       SmartDispatchMover
}

type SmartDispatchRefillRequest struct {
	TargetGroup *Group
	ExcludedIDs map[int64]struct{}
}

type SmartDispatchRefillResult struct {
	Attempted           bool
	TargetAlreadyNormal bool
	MovedAccountIDs     []int64
}

func NewSmartDispatchService(accountRepo SmartDispatchAccountLister, mover SmartDispatchMover) *SmartDispatchService {
	return &SmartDispatchService{accountRepo: accountRepo, mover: mover}
}

func (s *SmartDispatchService) Refill(ctx context.Context, req SmartDispatchRefillRequest) (*SmartDispatchRefillResult, error) {
	result := &SmartDispatchRefillResult{}
	group := req.TargetGroup
	if s == nil || s.accountRepo == nil || s.mover == nil || group == nil {
		return result, nil
	}
	if !group.SmartDispatchEnabled || group.SmartDispatchSourceGroupID == nil || *group.SmartDispatchSourceGroupID <= 0 {
		return result, nil
	}

	count := group.SmartDispatchCount
	if count <= 0 {
		count = 1
	}
	sourceGroupID := *group.SmartDispatchSourceGroupID

	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, sourceGroupID)
	if err != nil {
		return result, fmt.Errorf("list smart dispatch source accounts: %w", err)
	}
	if len(accounts) == 0 {
		result.Attempted = true
		return result, nil
	}

	selected := make([]int64, 0, count)
	for i := range accounts {
		acc := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[acc.ID]; excluded {
				continue
			}
		}
		if !acc.IsSchedulable() {
			continue
		}
		selected = append(selected, acc.ID)
		if len(selected) >= count {
			break
		}
	}
	if len(selected) == 0 {
		result.Attempted = true
		return result, nil
	}

	movedIDs, targetAlreadyNormal, err := s.mover.MoveAccountsForSmartDispatch(ctx, group.ID, sourceGroupID, selected)
	if err != nil {
		return result, fmt.Errorf("move smart dispatch accounts: %w", err)
	}
	result.Attempted = true
	result.TargetAlreadyNormal = targetAlreadyNormal
	result.MovedAccountIDs = movedIDs
	return result, nil
}
