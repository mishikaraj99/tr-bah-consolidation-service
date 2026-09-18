package logearn

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/setup"
)

func TestZAmountForDay(t *testing.T) {
	cases := map[int][2]int{1: {0, 2}, 7: {0, 2}, 8: {1, 3}, 14: {1, 3}, 15: {2, 4}, 99: {2, 4}}
	for day, want := range cases {
		tier, amount := ZAmountForDay(day, DefaultZTiers)
		assert.Equal(t, want[0], tier, "day %d tier", day)
		assert.Equal(t, want[1], amount, "day %d amount", day)
	}
}

func TestBonusForDay(t *testing.T) {
	b := DefaultBonuses
	assert.Equal(t, 5, BonusForDay(1, true, b), "first ever")
	assert.Equal(t, 0, BonusForDay(1, false, b))
	assert.Equal(t, 5, BonusForDay(3, false, b), "day 3")
	assert.Equal(t, 10, BonusForDay(3, true, b), "first ever on day 3 stacks")
	assert.Equal(t, 10, BonusForDay(7, false, b), "milestone")
	assert.Equal(t, 10, BonusForDay(21, false, b))
	assert.Equal(t, 0, BonusForDay(8, false, b))
}

func TestComputeStreakDay(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	day, first := ComputeStreakDay(nil, now, false)
	assert.Equal(t, 1, day)
	assert.True(t, first)

	yesterday := []DoseLog{{LogDate: now.AddDate(0, 0, -1), StreakDay: 4}}
	day, first = ComputeStreakDay(yesterday, now, false)
	assert.Equal(t, 5, day)
	assert.False(t, first)

	twoDaysAgo := []DoseLog{{LogDate: now.AddDate(0, 0, -2), StreakDay: 4}}
	day, _ = ComputeStreakDay(twoDaysAgo, now, false)
	assert.Equal(t, 5, day, "a single missed day still continues")

	threeDaysAgo := []DoseLog{{LogDate: now.AddDate(0, 0, -3), StreakDay: 4}}
	day, _ = ComputeStreakDay(threeDaysAgo, now, false)
	assert.Equal(t, 1, day, "gap of 3 resets")
	day, _ = ComputeStreakDay(threeDaysAgo, now, true)
	assert.Equal(t, 5, day, "a lifeline bridges the gap")
}

func TestConfigClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/carestack/config/mool", r.URL.Path)
		assert.Equal(t, "mool", r.Header.Get("x-tenant-id"))
		_, _ = w.Write([]byte(`{"data":{"rewards":{"zTiers":[{"upTo":5,"amount":9}],"bonuses":{"firstEver":7}}}}`))
	}))
	defer srv.Close()
	c := &ConfigClient{HTTP: srv.Client(), BaseURL: srv.URL, TTL: time.Minute}
	cfg := c.Get(context.Background(), "mool")
	require.Len(t, cfg.ZTiers, 1)
	assert.Equal(t, 9, cfg.ZTiers[0].Amount)
	assert.Equal(t, 7, cfg.Bonuses.FirstEver)
	assert.Equal(t, 5, cfg.Bonuses.DayThree, "unset bonuses keep defaults")

	bad := &ConfigClient{HTTP: srv.Client(), BaseURL: "http://127.0.0.1:1"}
	def := bad.Get(context.Background(), "mool")
	assert.Equal(t, DefaultZTiers, def.ZTiers, "unreachable CMS falls back to defaults")
}

// ---- integration ----

func testService(t *testing.T, now time.Time, orderSrv *httptest.Server) *Service {
	uri := os.Getenv("TEST_PG_URI")
	if uri == "" {
		t.Skip("TEST_PG_URI not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(uri)
	require.NoError(t, err)
	cfg.MaxConns = 8
	cfg.ConnConfig.RuntimeParams["search_path"] = "logearn_test"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	testSchema(t, pool, "logearn_test")
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "pg", "logearn", "001_dose_logs_cash_ledger.sql"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(sql))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `TRUNCATE dose_logs, cash_ledger`)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	client := http.DefaultClient
	base := ""
	if orderSrv != nil {
		client, base = orderSrv.Client(), orderSrv.URL
	}
	return New("mool", Deps{
		Store:  pgrepo.NewLogEarnStore(pool, nil),
		Orders: &orders.TROrderClient{HTTP: client, BaseURL: base},
		Config: &ConfigClient{},
		Clock:  common.FixedClock{T: now},
		Cfg:    &setup.Config{S3ImageBaseURL: "https://cdn.test/"},
		Log:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

func orderServer(t *testing.T, deliveredDaysAgo int, now time.Time) *httptest.Server {
	delivery := now.AddDate(0, 0, -deliveredDaysAgo).Format("2006-01-02T15:04:05.000Z")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/orders/orders/get-orders-by-order-ids" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"11111111-1111-1111-1111-111111111111","status":"delivered","delivery_date":"` + delivery + `","created_at":"` + delivery + `","bulk_order_duration":1}]}`))
	}))
}

const testCustomer = "22222222-2222-2222-2222-222222222222"

func TestLogToday(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	srv := orderServer(t, 5, now)
	defer srv.Close()
	s := testService(t, now, srv)
	ctx := context.Background()

	res, err := s.LogToday(ctx, testCustomer, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, res.StreakDay)
	assert.Equal(t, 7, res.INRCredited, "₹2 daily + ₹5 first-ever")
	assert.Equal(t, 7, res.Balance)

	_, err = s.LogToday(ctx, testCustomer, nil)
	require.Error(t, err)
	assert.Equal(t, MsgAlreadyLoggedToday, err.Error())
	assert.Equal(t, 400, common.StatusOf(err))
}

func TestLogToday_Day3Bonus(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	srv := orderServer(t, 5, now)
	defer srv.Close()
	s := testService(t, now, srv)
	ctx := context.Background()

	// seed two prior days so today is streak day 3
	for i := 2; i >= 1; i-- {
		s.Clock = common.FixedClock{T: now.AddDate(0, 0, -i)}
		_, err := s.LogToday(ctx, testCustomer, nil)
		require.NoError(t, err)
	}
	s.Clock = common.FixedClock{T: now}
	res, err := s.LogToday(ctx, testCustomer, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, res.StreakDay)
	assert.Equal(t, 7, res.INRCredited, "₹2 daily + ₹5 day-3 bonus")
}

func TestLogToday_WindowExpiredAndNoOrder(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	expired := orderServer(t, 60, now) // 60 days > 30 cap + 15 grace
	defer expired.Close()
	s := testService(t, now, expired)
	_, err := s.LogToday(context.Background(), testCustomer, nil)
	require.Error(t, err)
	assert.Equal(t, MsgWindowExpired, err.Error())

	none := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer none.Close()
	s2 := testService(t, now, none)
	_, err = s2.LogToday(context.Background(), testCustomer, nil)
	require.Error(t, err)
	assert.Equal(t, MsgNoActiveOrder, err.Error())
}

func TestBackfillAndLifelineAndRedeem(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	srv := orderServer(t, 5, now)
	defer srv.Close()
	s := testService(t, now, srv)
	ctx := context.Background()

	_, err := s.BackfillYesterday(ctx, testCustomer, nil)
	require.Error(t, err)
	assert.Equal(t, MsgLogTodayFirst, err.Error())

	_, err = s.LogToday(ctx, testCustomer, nil)
	require.NoError(t, err)
	res, err := s.BackfillYesterday(ctx, testCustomer, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, res.StreakDay)
	assert.Equal(t, 2, res.INRCredited, "backfill never grants the first-ever bonus")

	_, err = s.BackfillYesterday(ctx, testCustomer, nil)
	require.Error(t, err)
	assert.Equal(t, MsgYesterdayLogged, err.Error())

	// lifeline is refused while the streak is intact
	_, err = s.UseLifeline(ctx, testCustomer)
	require.Error(t, err)
	assert.Equal(t, MsgStreakNotBroken, err.Error())

	// redeem: capped at 50% of subtotal
	balance := s.balance(ctx, testCustomer)
	assert.Equal(t, 9, balance)
	r, err := s.Redeem(ctx, testCustomer, 10, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 5, r.Redeemed, "50% of subtotal 10")
	assert.Equal(t, 4, r.Balance)

	order := "33333333-3333-3333-3333-333333333333"
	r, err = s.Redeem(ctx, testCustomer, 100, nil, &order)
	require.NoError(t, err)
	assert.Equal(t, 4, r.Redeemed)
	_, err = s.Redeem(ctx, testCustomer, 100, nil, &order)
	require.Error(t, err)
	assert.Equal(t, MsgOrderAlreadyRedeemed, err.Error())

	_, err = s.Redeem(ctx, testCustomer, 100, nil, nil)
	require.Error(t, err)
	assert.Equal(t, MsgNothingToRedeem, err.Error())
}

func TestUseLifeline(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	srv := orderServer(t, 10, now)
	defer srv.Close()
	s := testService(t, now, srv)
	ctx := context.Background()

	s.Clock = common.FixedClock{T: now.AddDate(0, 0, -5)}
	_, err := s.LogToday(ctx, testCustomer, nil)
	require.NoError(t, err)
	s.Clock = common.FixedClock{T: now}

	res, err := s.UseLifeline(ctx, testCustomer)
	require.NoError(t, err)
	assert.Equal(t, MsgLifelineApplied, res.Message)
	assert.Equal(t, 5, res.DaysBridged)
	assert.Equal(t, 1, res.PriorStreakDay)

	_, err = s.UseLifeline(ctx, testCustomer)
	require.Error(t, err)
	assert.Equal(t, MsgLifelineAlreadyUsed, err.Error())

	// after the lifeline, today's log continues the streak instead of resetting
	log, err := s.LogToday(ctx, testCustomer, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, log.StreakDay)
}

func TestGetState(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	srv := orderServer(t, 5, now)
	defer srv.Close()
	s := testService(t, now, srv)
	ctx := context.Background()

	st, err := s.GetState(ctx, testCustomer)
	require.NoError(t, err)
	assert.Equal(t, 0, st.StreakDay)
	assert.False(t, st.LogDoneToday)
	assert.Equal(t, 2, st.ZForNext)
	assert.Equal(t, 1, st.BulkX)
	assert.Equal(t, 30, st.EarningCapDays)
	assert.Equal(t, 45, st.TotalWindowDays)
	assert.False(t, st.BackfillAvailable, "no logs yet")

	_, err = s.LogToday(ctx, testCustomer, nil)
	require.NoError(t, err)
	st, err = s.GetState(ctx, testCustomer)
	require.NoError(t, err)
	assert.True(t, st.LogDoneToday)
	assert.Equal(t, 1, st.StreakDay)
	assert.Equal(t, 7, st.Balance)
	require.Len(t, st.Last7, 1)
	assert.Equal(t, "2026-09-18", st.Last7[0].Date)
	assert.True(t, st.BackfillAvailable, "yesterday is still open")
	assert.False(t, st.LifelineAvailable)
	assert.Equal(t, 1, st.EarningDaysConsumed)
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
