package legacy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	"traya-bah-service/models"
)

func statusOf(err error) int { return common.StatusOf(err) }

func TestShopfloClient(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		b := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(b)
			gotBody = string(b)
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"total_wallet_balance":50}`))
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := &ShopfloClient{HTTP: srv.Client(), Endpoint: srv.URL + "/", IssuerID: "iss", MerchantID: "mer", APIKey: "raw-key"}

	w, err := c.Read(context.Background(), "9876543210")
	require.NoError(t, err)
	assert.Equal(t, float64(50), w.TotalWalletBalance)
	assert.Equal(t, "raw-key", gotAuth, "raw key, no Bearer prefix")
	assert.Equal(t, "/issuer/iss/merchant/mer/user-wallet?phone-number=%2B919876543210", gotPath)

	require.NoError(t, c.Debit(context.Background(), 300, "ref1", "+919876543210", "order-1"))
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(gotBody), &body))
	assert.Equal(t, float64(30), body["amount"], "coins/10")
	assert.Equal(t, "order-1", body["transaction_reference"])

	assert.Equal(t, "+919876543210", NormalizePhone("9876543210"))
	assert.Equal(t, "+919876543210", NormalizePhone("+919876543210"))
	assert.Equal(t, "+919876543210", NormalizePhone("919876543210"))
}

func TestShopfloDebitErrors(t *testing.T) {
	code := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
	defer srv.Close()
	c := &ShopfloClient{HTTP: srv.Client(), Endpoint: srv.URL + "/", IssuerID: "i", MerchantID: "m", APIKey: "k"}
	err := c.Debit(context.Background(), 10, "r", "p", "")
	assert.Equal(t, "No record found for this user", err.Error())
	code = http.StatusBadRequest
	err = c.Debit(context.Background(), 10, "r", "p", "")
	assert.Equal(t, "User have less coin balance to debit", err.Error())
}

func TestSaveRewardTransaction_ExpiryAndIdempotency(t *testing.T) {
	now := time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC)
	s, ccd, _ := newTestService(t, now)
	ctx := context.Background()
	master, err := s.Store.UpsertStreakMasterBySlug(ctx, SlugFirstCheckinExtra, &models.StreakMaster{DisplayName: "First Log", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)

	days := 90
	res, err := s.SaveRewardTransaction(ctx, SaveRewardInput{UserID: "u1", Master: master, Reason: RemarkFirstLog, CaseID: "c1", ExpiryDaysOverride: &days})
	require.NoError(t, err)
	require.NotNil(t, res.Ref)
	assert.False(t, res.Duplicate)
	// expire_at = UTC midnight of (IST now + 1 + 90 days). IST(2026-09-18T06:00Z) = 11:30 on the 18th.
	assert.Equal(t, "2026-12-18", res.Ref.ExpireAt.Format("2006-01-02"))
	assert.Equal(t, 100, res.Ref.CreditCoins)
	require.Len(t, ccd.events, 1)
	assert.Equal(t, "COIN_CREDITED", ccd.events[0]["eventType"])

	// replay → duplicate, no second CCD
	res2, err := s.SaveRewardTransaction(ctx, SaveRewardInput{UserID: "u1", Master: master, Reason: RemarkFirstLog, CaseID: "c1", ExpiryDaysOverride: &days})
	require.NoError(t, err)
	assert.True(t, res2.Duplicate)
	assert.Nil(t, res2.Ref)
	assert.Len(t, ccd.events, 1, "no duplicate CCD event")
}

func TestCreditRewardCoinsToUser_Legacy(t *testing.T) {
	now := time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	// order-service stub for coin expiry
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/coin/expiry/month/u1", r.URL.Path)
		_, _ = w.Write([]byte("90"))
	}))
	defer srv.Close()
	s.Cfg.OrderServiceBaseURL = srv.URL
	s.HTTP.OrderService = srv.Client()

	_, err := s.Store.UpsertStreakMasterBySlug(ctx, SlugFirstCheckinExtra, &models.StreakMaster{DisplayName: "First Log", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	_, err = s.Store.UpsertStreakMasterBySlug(ctx, SlugExistingCoinsStreak, &models.StreakMaster{DisplayName: "Existing", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	m3, err := s.Store.UpsertStreakMasterBySlug(ctx, "three-day", &models.StreakMaster{DisplayName: "3 Day", Days: 3, IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	_ = m3

	checkIn := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	_, err = s.CreditRewardCoinsToUser(ctx, "u1", 3, "9876543210", false, checkIn, "c1")
	require.NoError(t, err)
	txns, err := s.Store.FindRewardTransactionsByUser(ctx, "u1", true, nil)
	require.NoError(t, err)
	require.Len(t, txns, 2, "first-log bonus + 3-day milestone")

	// replay the same day → still 2 (idempotent)
	_, err = s.CreditRewardCoinsToUser(ctx, "u1", 3, "9876543210", false, checkIn, "c1")
	require.NoError(t, err)
	txns, err = s.Store.FindRewardTransactionsByUser(ctx, "u1", true, nil)
	require.NoError(t, err)
	assert.Len(t, txns, 2, "replay must not double-credit")
}

func TestCreditRewardCoinsToUser_HabitTracker(t *testing.T) {
	now := time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC)
	s, _, _ := newTestService(t, now)
	minted := true
	fh := &fakeHabit{result: HabitCreditResult{Success: true, Minted: &minted, Card: map[string]any{"id": "card1"}}}
	s.Habit = fh
	res, err := s.CreditRewardCoinsToUser(context.Background(), "u1", 7, "9876543210", true, now, "c1")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	require.Len(t, fh.calls, 1)
	assert.Equal(t, 7, fh.calls[0].StreakDay)
}

func TestCohorts(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	mk := func(caseID string, delivery time.Time, n int) []orders.Order {
		out := make([]orders.Order, 0, n)
		for i := 0; i < n; i++ {
			d := delivery
			out = append(out, orders.Order{CaseID: caseID, DeliveryDate: &d, CreatedAt: d})
		}
		return out
	}
	assert.False(t, CheckO8PlusEligibility(mk("5abc", time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC), 7), now), "needs 8 orders")
	assert.True(t, CheckO8PlusEligibility(mk("5abc", time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC), 8), now))
	assert.False(t, CheckO8PlusEligibility(mk("1abc", time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC), 8), now), "control prefix")
	assert.False(t, CheckO8PlusEligibility(mk("5abc", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 8), now), "delivery before go-live")

	assert.True(t, IsMaleStreakRestartCohort("a123", "M"))
	assert.False(t, IsMaleStreakRestartCohort("2123", "M"))
	assert.False(t, IsMaleStreakRestartCohort("a123", "F"))
	assert.True(t, IsFemaleStreakRestartCohort("f123", "F", 1))
	assert.False(t, IsFemaleStreakRestartCohort("f123", "F", 2))
	assert.False(t, CheckStreakRestartBonusEligibility("a123", "M"), "male flag is off")
}
