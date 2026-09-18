package pgrepo

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogEarnStore(t *testing.T) *LogEarnStore {
	uri := os.Getenv("TEST_PG_URI")
	if uri == "" {
		t.Skip("TEST_PG_URI not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(uri)
	require.NoError(t, err)
	cfg.MaxConns = 8
	cfg.ConnConfig.RuntimeParams["search_path"] = "pgrepo_test"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	testSchema(t, pool, "pgrepo_test")
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "pg", "logearn", "001_dose_logs_cash_ledger.sql"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `TRUNCATE dose_logs, cash_ledger`)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return NewLogEarnStore(pool, nil)
}

func TestAppendLedger_Concurrent(t *testing.T) {
	s := testLogEarnStore(t)
	ctx := context.Background()
	cust := "11111111-1111-1111-1111-111111111111"
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.WithTx(ctx, func(tx pgx.Tx) error {
				_, err := s.AppendLedger(ctx, tx, cust, nil, 2, "DOSE_LOG")
				return err
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	bal, err := s.Balance(ctx, cust)
	require.NoError(t, err)
	assert.Equal(t, 40, bal, "sum of amounts")
	latest, err := s.LatestLedger(ctx, cust)
	require.NoError(t, err)
	assert.Equal(t, 40, latest.BalanceAfter, "newest row carries the final balance")
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := s.AppendLedger(ctx, tx, cust, nil, -41, "REDEEM")
		return err
	})
	assert.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestInsertDoseLog_UniquePerDay(t *testing.T) {
	s := testLogEarnStore(t)
	ctx := context.Background()
	cust := "22222222-2222-2222-2222-222222222222"
	order := "33333333-3333-3333-3333-333333333333"
	day := time.Date(2026, 9, 17, 18, 30, 0, 0, time.UTC)
	ins := func() error {
		return s.WithTx(ctx, func(tx pgx.Tx) error {
			return s.InsertDoseLog(ctx, tx, &DoseLog{CustomerID: cust, OrderID: order, LogDate: day, StreakDay: 1, ZTier: 0, INRCredited: 2})
		})
	}
	require.NoError(t, ins())
	assert.Error(t, ins(), "same (customer, log_date) must violate the unique index")
	logs, err := s.RecentDoseLogs(ctx, cust, 14)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
}

// testSchema isolates this package's tables in a private schema. `go test ./...` runs packages in
// parallel, so sharing public tables here deadlocked on TRUNCATE's ACCESS EXCLUSIVE lock.
func testSchema(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+name)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `SET search_path TO `+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+name+` CASCADE`)
	})
}
