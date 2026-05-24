package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type recordingTempUnschedulableSmartDispatchRefiller struct {
	accountIDs []int64
	err        error
}

func (r *recordingTempUnschedulableSmartDispatchRefiller) RefillForTempUnschedulableAccount(_ context.Context, accountID int64) error {
	r.accountIDs = append(r.accountIDs, accountID)
	return r.err
}

func TestSetTempUnschedulableInvokesSmartDispatchRefill(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	const accountID int64 = 42
	until := time.Now().Add(10 * time.Minute).UTC()
	refiller := &recordingTempUnschedulableSmartDispatchRefiller{}
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	repo.tempUnschedulableSmartDispatchRefiller = refiller

	mock.ExpectExec("UPDATE accounts").
		WithArgs(until, "temporary", accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.SetTempUnschedulable(context.Background(), accountID, until, "temporary")
	require.NoError(t, err)
	require.Equal(t, []int64{accountID}, refiller.accountIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetTempUnschedulableIgnoresSmartDispatchRefillError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	const accountID int64 = 43
	until := time.Now().Add(10 * time.Minute).UTC()
	refiller := &recordingTempUnschedulableSmartDispatchRefiller{err: errors.New("source group unavailable")}
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	repo.tempUnschedulableSmartDispatchRefiller = refiller

	mock.ExpectExec("UPDATE accounts").
		WithArgs(until, "temporary", accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.SetTempUnschedulable(context.Background(), accountID, until, "temporary")
	require.NoError(t, err)
	require.Equal(t, []int64{accountID}, refiller.accountIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
