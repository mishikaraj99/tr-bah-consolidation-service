package legacy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/orders"
	"traya-bah-service/models"
)

func TestGetStreakAndRewardBalance_NeverLogged(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	po.userCase = pgUserCase("c1", "M")
	ctx := context.Background()

	out, err := s.GetStreakAndRewardBalance(ctx, "u1", 83, "")
	require.NoError(t, err)
	assert.Equal(t, 0, out.RewardBalance)
	assert.True(t, out.IsUserEligibleForNewBahFlow)
	assert.Equal(t, "Log And Earn", out.BahTitle)
	assert.Equal(t, 25, out.CoinDiscountCap.Value)
	assert.Equal(t, "0.1", out.CoinConversionRatio)
	assert.True(t, out.HasUserSeenBahUpdatedModalResult, "literal true")
	assert.Equal(t, 0, out.ThreeDaysStreakCount, "always zero, preserved quirk")
	banner, ok := out.BahBanner.(BahBanner)
	require.True(t, ok)
	assert.Equal(t, 100, banner.Coins)
	assert.Equal(t, "Unlocking Soon!", banner.UnlockText)
	require.NotNil(t, out.BannerWidgetData)
	assert.Equal(t, "Log everyday to earn up to 20% off on your next kit.", out.BannerWidgetData.Title)
	assert.Equal(t, "You won rewards for doing your first log", out.Modals.RewardsModal.Description.FirstLog)
}

func TestGetStreakAndRewardBalance_VersionGate(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	po.userCase = pgUserCase("c1", "M")
	ctx := context.Background()
	active := true
	_, err := s.Store.CreateStreak(ctx, &models.StreakLog{UserID: "u1", StreakAchieveDays: 7, LongestStreakDays: 7,
		FirstDateOfLog: day(2026, 9, 8), LastDateOfLog: day(2026, 9, 14), IsActive: &active})
	require.NoError(t, err)

	// version 70 → no bannerWidgetData and no streak-broken recompute
	out, err := s.GetStreakAndRewardBalance(ctx, "u1", 70, "")
	require.NoError(t, err)
	assert.Nil(t, out.BannerWidgetData)
	assert.Equal(t, 7, out.CurrentDaysStreakCount, "no recompute below v71")

	// version 83 → banner present and the 4-day gap resets the streak
	out, err = s.GetStreakAndRewardBalance(ctx, "u1", 83, "")
	require.NoError(t, err)
	require.NotNil(t, out.BannerWidgetData)
	assert.Equal(t, "Your streak broke. Log today to restart.", out.BannerWidgetData.Title)
	assert.Equal(t, 0, out.CurrentDaysStreakCount)
}

func TestGetStreakAndRewardBalance_CommunityCTA(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	po.userCase = pgUserCase("c1", "M")
	ctx := context.Background()
	active := true
	_, err := s.Store.CreateStreak(ctx, &models.StreakLog{UserID: "u1", StreakAchieveDays: 7, LongestStreakDays: 7,
		FirstDateOfLog: day(2026, 9, 12), LastDateOfLog: day(2026, 9, 18), IsActive: &active})
	require.NoError(t, err)

	out, err := s.GetStreakAndRewardBalance(ctx, "u1", 83, "tok123")
	require.NoError(t, err)
	btns := out.Modals.RewardsModal.Buttons
	require.Len(t, btns, 2)
	assert.Equal(t, "Share on Community", btns[1].Label)
	assert.Equal(t, "https://community.test/landing/tok123?share=true&streak_count=7&reward_coins=400&cross=no&preventBack=true", btns[1].URL)
	assert.Equal(t, "400", out.Modals.RewardsModal.RewardAmount)
}

func TestGetStreakAndRewardBalance_ResponseShape(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	po.userCase = pgUserCase("c1", "F")
	out, err := s.GetStreakAndRewardBalance(context.Background(), "u1", 83, "")
	require.NoError(t, err)
	b, err := json.Marshal(out)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	for _, k := range []string{
		"rewardBalance", "currentDaysStreakCount", "longestDaysStreakCount", "threeDaysStreakCount",
		"sevenDaysStreakCount", "twentyOneDaysStreakCount", "lastLogDate", "firstLogDate",
		"isUserEligibleForNewBahFlow", "bahBanner", "bahTitle", "popupText", "coinDiscountCap",
		"coinConversionRatio", "coinNotApplied", "coinApplied", "autoApplyCoins", "bahbannerNewTitle",
		"streakRewardMessage", "hasUserSeenBahUpdatedModalResult", "bannerWidgetData",
		"showBAHLogMissedToUser", "modals", "reminderOnBahPage",
	} {
		assert.Contains(t, raw, k, "response must carry %s", k)
	}
	assert.Equal(t, "top", raw["reminderOnBahPage"])
}

func TestGetUserRewardBalance(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	po.userCase = pgUserCase("c1", "M")
	ctx := context.Background()
	master, err := s.Store.UpsertStreakMasterBySlug(ctx, "m", &models.StreakMaster{DisplayName: "M", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	exp := now.Add(72 * time.Hour)
	_, err = s.Store.InsertRewardTransaction(ctx, &models.RewardTransaction{UserID: "u1", StreakMasterID: master.ID,
		CreditCoins: 250, TotalDebitCoins: 50, IsCreditTransaction: true, Status: "success", ExpireAt: &exp})
	require.NoError(t, err)
	bal, err := s.GetUserRewardBalance(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, 200, bal)
}

func TestGetEarliestExpiringUnusedCoins(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	master, err := s.Store.UpsertStreakMasterBySlug(ctx, "m", &models.StreakMaster{DisplayName: "M", IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	exp := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_, err = s.Store.InsertRewardTransaction(ctx, &models.RewardTransaction{UserID: "u1", StreakMasterID: master.ID,
		CreditCoins: 120, TotalDebitCoins: 20, IsCreditTransaction: true, Status: "success", ExpireAt: &exp})
	require.NoError(t, err)
	got, err := s.GetEarliestExpiringUnusedCoins(ctx, "u1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 100, got.RemainingCoins)
	assert.Equal(t, "2026-10-01", got.ExpiringOn)
	_ = orders.Order{}
}
