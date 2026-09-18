package legacy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatters(t *testing.T) {
	assert.Equal(t, "1 Day", FormatDays(1))
	assert.Equal(t, "0 Days", FormatDays(0))
	assert.Equal(t, "1 Coin", FormatCoins(1))
	assert.Equal(t, "250 Coins", FormatCoins(250))
}

func TestGetBannerWidgetData(t *testing.T) {
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	cdn := "https://cdn.test/"
	yesterday := now.AddDate(0, 0, -1)
	fourDaysAgo := now.AddDate(0, 0, -4)

	// A: never logged, unlocked
	b := GetBannerWidgetData(BannerInput{Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "Log everyday to earn up to 20% off on your next kit.", b.Title)
	assert.Equal(t, "Discounts worth ₹54L won last month!", b.SubTitle)
	assert.Equal(t, cdn+"App/bah/new_bah/Coins.svg", b.SubTitleIcon)
	assert.Equal(t, "Log & Earn", b.CtaLabel)
	assert.True(t, b.ShowBlueBar)
	assert.Empty(t, b.CoinBalance)

	// B1: logged before, streak broken by gap
	b = GetBannerWidgetData(BannerInput{HasLoggedEver: true, CurrentDaysStreakCount: 7, LastLogDate: &fourDaysAgo, RewardBalance: 250, Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "Your streak broke. Log today to restart.", b.Title)
	assert.Equal(t, "250 Coins", b.CoinBalance)
	assert.Equal(t, "0 Days", b.StreakDays)
	assert.Equal(t, "Log Now", b.CtaLabel)
	assert.True(t, b.ShowStreakTimeline)

	// B1 with restart bonus
	b = GetBannerWidgetData(BannerInput{HasLoggedEver: true, CurrentDaysStreakCount: 7, LastLogDate: &fourDaysAgo, Now: now, CDNBaseURL: cdn, IsEligibleForStreakRestartBonus: true})
	assert.Equal(t, "Get bonus 100 coins to restart your streak!", b.Title)
	assert.Equal(t, "Get back on track today", b.SubTitle)

	// B2: streak alive, not logged today
	b = GetBannerWidgetData(BannerInput{HasLoggedEver: true, CurrentDaysStreakCount: 5, LastLogDate: &yesterday, RewardBalance: 1, Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "Log for today. Keep your streak going!", b.Title)
	assert.Equal(t, "1 Coin", b.CoinBalance)
	assert.Equal(t, "5 Days", b.StreakDays)

	// 21 + 1 day → streakDays shows 0
	b = GetBannerWidgetData(BannerInput{HasLoggedEver: true, CurrentDaysStreakCount: 21, LastLogDate: &yesterday, Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "0 Days", b.StreakDays)

	// C: logged today
	b = GetBannerWidgetData(BannerInput{HasLoggedEver: true, LoggedToday: true, CurrentDaysStreakCount: 3, LastLogDate: &now, Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "Log done for today!", b.Title)
	assert.Equal(t, "View Log", b.CtaLabel)

	// D / E: locked
	b = GetBannerWidgetData(BannerInput{IsBahLocked: true, HasLoggedEver: true, Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "Log & Earn is locked.", b.Title)
	assert.Equal(t, "Order next kit to unlock it.", b.SubTitle)
	assert.Equal(t, cdn+"App/bah/new_bah/Lock.svg", b.Icon)
	b = GetBannerWidgetData(BannerInput{IsBahLocked: true, Now: now, CDNBaseURL: cdn})
	assert.Equal(t, "Order next kit to start earning coins.", b.SubTitle)
	assert.Equal(t, "Know More", b.CtaLabel)
	assert.Equal(t, cdn+"App/bah/new_bah/coin.svg", b.Icon)
}

func TestGetBahChallengeEntryPointBanner(t *testing.T) {
	s3 := "https://cdn.test/"
	bg := s3 + "App/Home/WeeklyChallengeBannerIMG.png"
	cases := []struct {
		name                                     string
		streak                                   int
		everLogged, loggedToday, missedYesterday bool
		title, subtitle, cta                     string
	}{
		{"missed yesterday", 5, true, false, true, "You missed yesterday's log.", "Restore your streak by logging for yesterday.", "Log now"},
		{"never logged, not today", 0, false, false, false, "Log now and claim your 100 coins", "Start your 3-day streak today", "Log now"},
		{"never logged, logged today", 1, false, true, false, "Log for 2 more days to earn 100 more coins.", "Continue your 3-day streak", "View Log"},
		{"completed 21, not logged", 21, true, false, false, "Congratulations! You've completed the 21-day streak!", "Keep logging to maintain your healthy habit.", "Log now"},
		{"completed 21, logged", 22, true, true, false, "Congratulations! You've completed the 21-day streak!", "Keep logging to maintain your healthy habit.", "View Log"},
		{"pre-log streak 0", 0, true, false, false, "Start logging today and build streaks.", "3 days to unlock 100 coins", "Log now"},
		{"pre-log streak 1", 1, true, false, false, "2 days left to unlock 100 coins.", "Continue your 3 day streak", "Log now"},
		{"pre-log streak 2", 2, true, false, false, "Log now to earn 100 coins.", "Complete your 3 day streak now.", "Log now"},
		{"pre-log streak 4", 4, true, false, false, "3 days left to unlock 400 coins", "Build your 7 day streak", "Log now"},
		{"pre-log streak 6", 6, true, false, false, "Log today to earn 400 coins.", "Complete your 7 day streak today", "Log now"},
		{"pre-log streak 19", 19, true, false, false, "2 days left to unlock 2000 coins.", "Complete your 21-day streak.", "Log now"},
		{"pre-log streak 20", 20, true, false, false, "Log today to earn 2000 coins.", "Complete your 21 day streak", "Log now"},
		// `remaining <= 0` is unreachable with these tier bands (see GetBahChallengeEntryPointBanner),
		// so a just-completed milestone renders the next-tier progress copy.
		{"post-log streak 3", 3, true, true, false, "Log for 4 more days to earn 400 coins.", "4 days to 7 day streak", "View Log"},
		{"post-log streak 7", 7, true, true, false, "Log for 14 more days to earn 2000 coins.", "Complete your 21-day streak", "View Log"},
		{"post-log streak 10", 10, true, true, false, "Log for 11 more days to earn 2000 coins.", "Complete your 21-day streak", "View Log"},
		{"post-log streak 5", 5, true, true, false, "Log for 2 more days to earn 400 coins.", "2 days to 7 day streak", "View Log"},
		{"post-log streak 1", 1, true, true, false, "Log for 2 more days to earn 100 coins.", "", "View Log"},
		{"post-log streak 2 singular", 2, true, true, false, "Log for 1 more day to earn 100 coins.", "", "View Log"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GetBahChallengeEntryPointBanner(c.streak, c.everLogged, c.loggedToday, c.missedYesterday, s3)
			assert.Equal(t, c.title, got.Title)
			assert.Equal(t, c.subtitle, got.Subtitle)
			assert.Equal(t, c.cta, got.Cta)
			assert.Equal(t, bg, got.BgImg)
		})
	}
}

func TestGetPostLoggingModalContent(t *testing.T) {
	cases := []struct {
		name                                string
		streak                              int
		ever, missedYesterday, restartBonus bool
		title, desc, cta, action            string
	}{
		{"missed yesterday", 5, true, true, false, "Well done!", "Log for today and continue your streak", "Log now", "setDateToToday"},
		{"first ever", 1, false, false, false, "100 coins unlocked for your 1st log.", "Log for 2 more days to build 3-day streak and earn 100 more coins", "Okay", "close"},
		{"streak 1 restart bonus", 1, true, false, true, "Bonus 100 coins credited!", "", "Okay", "close"},
		{"streak 1", 1, true, false, false, "🔥 Great start!", "2 more days to unlock 100 coins.\nCome tomorrow and log to continue your streak.", "Keep Going", "close"},
		{"streak 2", 2, true, false, false, "🎉 2-Day Streak!", "Next: Come tomorrow and log to complete your 3-day streak.", "Continue", "close"},
		{"streak 3", 3, true, false, false, "🎉 3-Day Streak!", "Next: Log for 4 more days to complete your 7-day streak.", "Continue", "close"},
		{"streak 4", 4, true, false, false, "4-Day Streak!", "Log for 3 more days to hit 7 day streak.", "Keep Building", "close"},
		{"streak 5", 5, true, false, false, "5-Day Streak!", "Log for 2 more days to hit 7 day streak.", "Keep Building", "close"},
		{"streak 6", 6, true, false, false, "6-Day Streak!", "Come tomorrow and log to complete your 7-day streak.", "Keep Building", "close"},
		{"streak 7", 7, true, false, false, "🏆 7-Day Streak!", "Log for 14 more days to complete your 21-day streak.", "Go for 21", "close"},
		{"streak 12", 12, true, false, false, "🚀 12 Days Strong!", "Log for 9 more days to unlock 2000 coins.", "Stay Consistent", "close"},
		{"streak 20", 20, true, false, false, "🚀 20 Days Strong!", "Log for 1 more day to unlock 2000 coins.", "Stay Consistent", "close"},
		{"streak 21", 21, true, false, false, "🎊 21-Day Streak!", "Congratulations! You've completed the 21-day streak!\nKeep logging to maintain your healthy habit.", "Amazing!", "close"},
		{"streak 0 falls through", 0, true, false, false, "🎊 0-Day Streak!", "Congratulations! You've completed the 21-day streak!\nKeep logging to maintain your healthy habit.", "Amazing!", "close"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GetPostLoggingModalContent(c.streak, c.ever, c.missedYesterday, c.restartBonus)
			assert.Equal(t, c.title, got.Title)
			assert.Equal(t, c.desc, got.Description)
			assert.Equal(t, c.cta, got.Cta)
			assert.Equal(t, c.action, got.CtaAction)
		})
	}
}

func TestBuildModals(t *testing.T) {
	post := GetPostLoggingModalContent(7, true, false, false)
	postY := GetPostLoggingModalContent(7, true, true, false)
	community := &ModalButton{Label: "Share on Community", Action: "web_page", Variant: "primary",
		URL: CommunityShareURL("https://community.test/", "tok", 7, 400)}
	m := BuildModals(ModalsInput{PostLog: post, PostLogYesterday: postY, CurrentCoins: 400, CurrentMilestone: 7, CommunityShareButton: community})

	assert.Equal(t, "streakModal", m.RewardsModal.Type)
	assert.Equal(t, "400", m.RewardsModal.RewardAmount)
	assert.Equal(t, MediaYellowCoin, m.RewardsModal.MediaURL)
	assert.Equal(t, "You won rewards for doing your first log", m.RewardsModal.Description.FirstLog)
	assert.Equal(t, "You won rewards for completing\n7-Day streak.", m.RewardsModal.Description.StreakComplete)
	require.Len(t, m.RewardsModal.Buttons, 2)
	assert.Equal(t, "Go for 21", m.RewardsModal.Buttons[0].Label)
	assert.Equal(t, "https://community.test/landing/tok?share=true&streak_count=7&reward_coins=400&cross=no&preventBack=true", m.RewardsModal.Buttons[1].URL)
	assert.Nil(t, m.StreakRestartBonusModal)
	assert.Equal(t, "Your streak broke ☹️", m.StreakBrokeModal.Title)
	assert.Equal(t, "Did Not Use Kit Yesterday", m.MissedLogModal.Buttons[0].Label)

	// feedback button replaces the primary; 15-day copy replaces streakComplete
	fb := &ModalButton{Label: "Share treatment feedback", Action: "web_page", Variant: "primary", URL: "https://feedback.test/form"}
	m2 := BuildModals(ModalsInput{PostLog: post, PostLogYesterday: postY, CurrentCoins: 400, CurrentMilestone: 7,
		FeedbackShareButton: fb, IsFifteenDayCheckinEligible: true, ShowStreakRestartModal: true})
	require.Len(t, m2.RewardsModal.Buttons, 1)
	assert.Equal(t, "Share treatment feedback", m2.RewardsModal.Buttons[0].Label)
	assert.Equal(t, FifteenDayCheckinSubtext, m2.RewardsModal.Description.StreakComplete)
	require.NotNil(t, m2.StreakRestartBonusModal)
	assert.Equal(t, "Bonus 100 coins credited!", m2.StreakRestartBonusModal.Title)

	b, err := json.Marshal(m)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	for _, k := range []string{"logDoneModal", "rewardsModal", "streakRestartBonusModal", "errorModal", "streakBrokeModal", "featureUpdateModal", "newOrderModal", "missedLogModal"} {
		assert.Contains(t, raw, k)
	}
	assert.Nil(t, raw["streakRestartBonusModal"], "null when not eligible")
}
