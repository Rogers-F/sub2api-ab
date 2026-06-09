package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestMoveAccountsForSmartDispatchMovesOnlyMissingMinimumNormalAccounts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	defer func() {
		_ = client.Close()
	}()
	repo := newGroupRepositoryWithSQL(client, db)

	const (
		targetGroupID int64 = 10
		sourceGroupID int64 = 20
	)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs(smartDispatchLockID(targetGroupID)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("(?s)SELECT COUNT\\(\\*\\).*FROM account_groups").
		WithArgs(targetGroupID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery("(?s)SELECT a\\.id.*FROM unnest\\(\\$1::bigint\\[\\]\\).*LIMIT \\$3").
		WithArgs(sqlmock.AnyArg(), sourceGroupID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)))
	mock.ExpectExec("DELETE FROM account_groups").
		WithArgs(sourceGroupID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_groups").
		WithArgs(sqlmock.AnyArg(), targetGroupID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountGroupsChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventGroupChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventGroupChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	movedIDs, targetAlreadyNormal, err := repo.MoveAccountsForSmartDispatch(
		context.Background(),
		targetGroupID,
		sourceGroupID,
		[]int64{101, 102},
		3,
	)

	require.NoError(t, err)
	require.False(t, targetAlreadyNormal)
	require.Equal(t, []int64{101}, movedIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMoveAccountsForSmartDispatchPreservesMoveCountWhenTargetEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	defer func() {
		_ = client.Close()
	}()
	repo := newGroupRepositoryWithSQL(client, db)

	const (
		targetGroupID int64 = 10
		sourceGroupID int64 = 20
	)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs(smartDispatchLockID(targetGroupID)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("(?s)SELECT COUNT\\(\\*\\).*FROM account_groups").
		WithArgs(targetGroupID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("(?s)SELECT a\\.id.*FROM unnest\\(\\$1::bigint\\[\\]\\).*LIMIT \\$3").
		WithArgs(sqlmock.AnyArg(), sourceGroupID, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)).AddRow(int64(102)))
	mock.ExpectExec("DELETE FROM account_groups").
		WithArgs(sourceGroupID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO account_groups").
		WithArgs(sqlmock.AnyArg(), targetGroupID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountGroupsChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountGroupsChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventGroupChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventGroupChanged, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	movedIDs, targetAlreadyNormal, err := repo.MoveAccountsForSmartDispatch(
		context.Background(),
		targetGroupID,
		sourceGroupID,
		[]int64{101, 102},
		1,
	)

	require.NoError(t, err)
	require.False(t, targetAlreadyNormal)
	require.Equal(t, []int64{101, 102}, movedIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}
