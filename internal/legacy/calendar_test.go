package legacy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
)

func statusFor(r *CalendarResponse, key string) string {
	for _, m := range r.Data {
		for _, d := range m.MonthData {
			if d.Date == key {
				return d.Status
			}
		}
	}
	return ""
}

func TestGetBahCalendarLogData_CalendarMode(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	delivered := day(2026, 9, 10)
	po.firstDeliv = &delivered

	// a valid log two days ago and a reward the same day
	logDate := common.ISTDate(day(2026, 9, 16))
	_, err := s.Store.CreateActivityLog(ctx, &models.ActivityLog{UserID: "u1", CheckInsForDate: logDate,
		ProductPrescriptions: []map[string]any{{"product_id": "p"}}, IsValidForStreak: true, IsActive: true})
	require.NoError(t, err)

	out, err := s.GetBahCalendarLogData(ctx, "u1", "2026-09-18", "calendar")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-18", out.EndDate, "endDate is always today (preserved quirk)")
	assert.Equal(t, "logged", statusFor(out, "2026-09-16"))
	assert.Equal(t, "active", statusFor(out, "2026-09-18"), "today unlogged becomes active")
	assert.Equal(t, "active", statusFor(out, "2026-09-17"), "yesterday unlogged and today not logged")
	assert.Equal(t, "2026-09-10", out.StartDate, "range starts at the first delivery")
	assert.Equal(t, "", statusFor(out, "2026-09-09"), "days before the first delivery are outside the range entirely")
	assert.Equal(t, "inactive", statusFor(out, "2026-09-25"), "after the input date")
	require.NotEmpty(t, out.Data)
	assert.Equal(t, "September 2026", out.Data[0].Month)
}

func TestGetBahCalendarLogData_Errors(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	_, err := s.GetBahCalendarLogData(ctx, "u1", "not-a-date", "calendar")
	require.Error(t, err)
	assert.Equal(t, MsgInvalidDateFormat, err.Error())
	_, err = s.GetBahCalendarLogData(ctx, "u1", "2026-09-18", "weird")
	require.Error(t, err)
	assert.Equal(t, MsgInvalidMode, err.Error())
	_, err = s.GetBahCalendarLogData(ctx, "u1", "2026-09-18", "streak")
	require.Error(t, err)
	assert.Equal(t, MsgNoActiveStreak, err.Error())
}

func TestGetRewardCoinHistoryPaginated(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	master, err := s.Store.UpsertStreakMasterBySlug(ctx, "m", &models.StreakMaster{DisplayName: "3 Day Streak", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	future := now.Add(72 * time.Hour)
	for i := 0; i < 8; i++ {
		_, err = s.Store.InsertRewardTransaction(ctx, &models.RewardTransaction{UserID: "u1", StreakMasterID: master.ID,
			CreditCoins: 100, IsCreditTransaction: true, Status: "success", ExpireAt: &future, CreditRemarks: "r"})
		require.NoError(t, err)
	}
	page1, err := s.GetRewardCoinHistoryPaginated(ctx, "u1", 1, 6)
	require.NoError(t, err)
	assert.Len(t, page1.RewardHistory, 6)
	assert.Equal(t, 8, page1.Pagination.Total)
	assert.Equal(t, 2, page1.Pagination.TotalPages)
	assert.True(t, page1.Pagination.HasNextPage)
	assert.False(t, page1.Pagination.HasPrevPage)
	assert.Equal(t, "3 Day Streak", page1.RewardHistory[0].StreakName)

	page2, err := s.GetRewardCoinHistoryPaginated(ctx, "u1", 2, 6)
	require.NoError(t, err)
	assert.Len(t, page2.RewardHistory, 2)
	assert.True(t, page2.Pagination.HasPrevPage)
	assert.False(t, page2.Pagination.HasNextPage)
}

func TestGetRewardCoinHistory_CRM(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	master, err := s.Store.UpsertStreakMasterBySlug(ctx, "m", &models.StreakMaster{DisplayName: "M", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	future := now.Add(72 * time.Hour)
	_, err = s.Store.InsertRewardTransaction(ctx, &models.RewardTransaction{UserID: "u1", StreakMasterID: master.ID,
		CreditCoins: 100, IsCreditTransaction: true, Status: "success", ExpireAt: &future, CreditRemarks: "credit"})
	require.NoError(t, err)
	oid := "order-1"
	_, err = s.Store.InsertRedeemTransaction(ctx, &models.RedeemTransaction{UserID: "u1", RedeemedCoins: 50,
		RedeemedAmount: 5, OrderID: &oid, Status: "failure", Remarks: "cancelled"})
	require.NoError(t, err)

	out, err := s.GetRewardCoinHistory(ctx, "u1", true, true)
	require.NoError(t, err)
	require.Len(t, out.RewardHistory, 2)
	assert.Equal(t, MsgRewardHistoryText1, out.Text1)
	assert.Equal(t, "", out.Text2)
	var debit map[string]any
	for _, r := range out.RewardHistory {
		if r["txnType"] == "Debit" {
			debit = r
		}
	}
	require.NotNil(t, debit)
	assert.Equal(t, false, debit["showCoins"])
	assert.Equal(t, "50 coins redeemed on this order added back to wallet", debit["text"])

	creditOnly, err := s.GetRewardCoinHistory(ctx, "u1", true, false)
	require.NoError(t, err)
	assert.Len(t, creditOnly.RewardHistory, 1)
}

func TestCRMHandlers(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	po.userCase = pgUserCase("c1", "M")
	// A manual grant credits through saveRewardTransaction, which reads the coin expiry from
	// order-service, so that upstream must be stubbed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("90"))
	}))
	defer srv.Close()
	s.Cfg.OrderServiceBaseURL = srv.URL
	s.HTTP.OrderService = srv.Client()

	// streak masters
	_, err := s.Store.UpsertStreakMasterBySlug(ctx, SlugExistingCoinsStreak,
		&models.StreakMaster{DisplayName: "Existing", IsActive: true, IsForSuperadmin: true, RewardCoins: 100})
	require.NoError(t, err)
	opts, err := s.GetMasterStreakDataForExtraBonusStreak(ctx)
	require.NoError(t, err)
	require.Len(t, opts, 1)
	assert.Equal(t, SlugExistingCoinsStreak, opts[0].SlugName)

	// extra rewards validation
	_, err = s.SaveExtraBonusForUsersForAnyReason(ctx, "c1", "", "reason", nil)
	assert.Equal(t, MsgAllFieldsMandatory, err.Error())
	_, err = s.SaveExtraBonusForUsersForAnyReason(ctx, "c1", "not-hex", "reason", nil)
	assert.Equal(t, MsgInvalidStreakMaster, err.Error())

	master := opts[0].StreakMasterID
	days := 30
	_ = days
	amount := 250
	res, err := s.SaveExtraBonusForUsersForAnyReason(ctx, "c1", master, "goodwill", &amount)
	require.NoError(t, err)
	assert.Equal(t, "250 coins credited successfully", res.Message)

	// custom amount on a non-existing_coins master is rejected
	other, err := s.Store.UpsertStreakMasterBySlug(ctx, "other-master",
		&models.StreakMaster{DisplayName: "Other", IsActive: true, IsForSuperadmin: true, RewardCoins: 10})
	require.NoError(t, err)
	_, err = s.SaveExtraBonusForUsersForAnyReason(ctx, "c1", other.ID.Hex(), "goodwill", &amount)
	assert.Equal(t, MsgInvalidStreakMasterCustom, err.Error())
}

func TestGetActivityLogs(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	_, err := s.Store.CreateActivityLog(ctx, &models.ActivityLog{UserID: "u1", CheckInsForDate: day(2026, 9, 5),
		ProductPrescriptions: []map[string]any{{"product_id": "p1"}}, IsValidForStreak: true, IsActive: true})
	require.NoError(t, err)
	y, m := 2026, 9
	out, err := s.GetActivityLogs(ctx, "u1", &y, &m)
	require.NoError(t, err)
	assert.Len(t, out.ActivityLogs, 30, "September has 30 day keys")
	require.Len(t, out.ActivityLogs["5"], 1)
	assert.NotNil(t, out.ActivityLogTime["5"])
	assert.Nil(t, out.ActivityLogTime["6"])
	assert.False(t, out.IsYesterdayLogExist)
}
