package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type recordingSmartDispatchRefiller struct {
	accountIDs []int64
	err        error
}

func (r *recordingSmartDispatchRefiller) RefillForUnavailableAccount(_ context.Context, accountID int64) error {
	r.accountIDs = append(r.accountIDs, accountID)
	return r.err
}

func newAccountRepoSQLite(t *testing.T) (*accountRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return newAccountRepositoryWithSQL(client, nil, nil), client
}

func TestSetTempUnschedulableInvokesSmartDispatchRefill(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	const accountID int64 = 42
	until := time.Now().Add(10 * time.Minute).UTC()
	refiller := &recordingSmartDispatchRefiller{}
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	repo.smartDispatchRefiller = refiller

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

func TestSetRateLimitedInvokesSmartDispatchRefill(t *testing.T) {
	repo, client := newAccountRepoSQLite(t)
	ctx := context.Background()
	account, err := client.Account.Create().
		SetName("rate-limited").
		SetPlatform(service.PlatformAnthropic).
		SetType(service.AccountTypeOAuth).
		SetCredentials(map[string]any{}).
		SetExtra(map[string]any{}).
		SetConcurrency(3).
		SetPriority(50).
		SetStatus(service.StatusActive).
		SetSchedulable(true).
		SetErrorMessage("").
		Save(ctx)
	require.NoError(t, err)

	refiller := &recordingSmartDispatchRefiller{}
	repo.smartDispatchRefiller = refiller

	resetAt := time.Now().Add(10 * time.Minute).UTC()
	err = repo.SetRateLimited(ctx, account.ID, resetAt)
	require.NoError(t, err)
	require.Equal(t, []int64{account.ID}, refiller.accountIDs)
}

func TestSetTempUnschedulableIgnoresSmartDispatchRefillError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	const accountID int64 = 43
	until := time.Now().Add(10 * time.Minute).UTC()
	refiller := &recordingSmartDispatchRefiller{err: errors.New("source group unavailable")}
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	repo.smartDispatchRefiller = refiller

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
