package habit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// All 16 states from inventory-app-backend-v85-habit-tracker.md §3.2, in resolver order.
func TestResolveState(t *testing.T) {
	base := StateInput{LifelinesTotal: 3, CoinBalance: 250, EffectiveDay: 3}
	coins := "₹25 off"
	cases := []struct {
		name                                              string
		in                                                func(StateInput) StateInput
		state, heading, cta, coinsSub, streakSub, lifeSub string
	}{
		{"kit_arriving_intro", func(s StateInput) StateInput { s.KitArrivingIntro = true; return s },
			"kit_arriving_intro", "Use kit regularly to get discounts", "Explore Now", coins, "Start Now", "3 Per Kit"},
		{"rewards_paused_unlogged", func(s StateInput) StateInput { s.EarningPaused = true; return s },
			"rewards_paused_unlogged", "Rewards are paused.\nOrder now to start earning again", "Log Now", coins, "Keep Going!", "3 Per Kit"},
		{"rewards_paused_logged", func(s StateInput) StateInput { s.EarningPaused = true; s.LoggedToday = true; return s },
			"rewards_paused_logged", "Rewards are paused.\nOrder now to start earning again", "View Logs", coins, "Keep Going!", "3 Per Kit"},
		{"new_order_placed", func(s StateInput) StateInput { s.NewOrderPlacedNotDelivered = true; return s },
			"new_order_placed", "New kit loading! You can continue\nlogging your current kit.", "Log Now", coins, "Streak Intact", "3 Per Kit"},
		{"streak_broken_reward_day", func(s StateInput) StateInput { s.StreakBroken = true; s.BrokeOnRewardDay = true; return s },
			"streak_broken_reward_day", "Your streak broke! Start afresh today", "Log Now", coins, "Start Today", "3 Per Kit"},
		{"streak_broken_coins_day", func(s StateInput) StateInput { s.StreakBroken = true; return s },
			"streak_broken_coins_day", "Your streak broke! Start afresh today", "Log Now", coins, "Start Today", "3 Per Kit"},
		{"lifelines_all_used_unlogged", func(s StateInput) StateInput { s.LifelinesUsed = 3; return s },
			"lifelines_all_used_unlogged", "No lifelines left\nLog today to keep your streak alive", "Log Now", coins, "Save Streak", "All Used"},
		{"lifeline_used_unlogged", func(s StateInput) StateInput { s.LifelinesUsed = 1; return s },
			"lifeline_used_unlogged", "Streak saved! We used 1 lifeline\n2 lifelines remaining", "Log Now", coins, "Save Streak", "1/3 Used"},
		{"new_kit_delivered", func(s StateInput) StateInput { s.NewKitDeliveredNotLogged = true; return s },
			"new_kit_delivered", "Do not miss out on your rewards!\nLog your dose now", "Log Now", coins, "Keep Going!", "3 Per Kit"},
		{"reward_day_logged", func(s StateInput) StateInput { s.RewardToday = true; s.LoggedToday = true; return s },
			"reward_day_logged", "Bonus unlocked!\nNext weekly reward is waiting for you", "View Logs", coins, "Keep Going!", "3 Per Kit"},
		{"reward_day_unlogged", func(s StateInput) StateInput { s.RewardToday = true; return s },
			"reward_day_unlogged", "Today is the day!\nClaim your weekly reward", "Claim Reward", coins, "Keep Going!", "3 Per Kit"},
		{"day_before_reward_logged", func(s StateInput) StateInput { s.DayBeforeReward = true; s.LoggedToday = true; return s },
			"day_before_reward_logged", "All done for today\nLog tomorrow to earn the bonus", "View Logs", coins, "Keep Going!", "3 Per Kit"},
		{"day_before_reward_unlogged", func(s StateInput) StateInput { s.DayBeforeReward = true; return s },
			"day_before_reward_unlogged", "Tomorrow is bonus day,\nLog today to get there", "Log Now", coins, "Keep Going!", "3 Per Kit"},
		{"active_streak_unlogged", func(s StateInput) StateInput { s.HasLoggedEver = true; s.EffectiveDay = 4; return s },
			"active_streak_unlogged", "4 days closer to the mystery bonus", "Log Now", coins, "Keep Going!", "3 Per Kit"},
		{"day_logged", func(s StateInput) StateInput { s.LoggedToday = true; return s },
			"day_logged", "All done for today\nLog daily to earn the bonus reward", "View Logs", coins, "Keep Going!", "3 Per Kit"},
		{"day1_never_logged", func(s StateInput) StateInput { return s },
			"day1_never_logged", "Did you use your kit today?", "Log Now", "Earn Discount", "Start Now", "3 Per Kit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveState(c.in(base))
			assert.Equal(t, c.state, got.State)
			assert.Equal(t, c.heading, got.Heading)
			assert.Equal(t, c.cta, got.CtaLabel)
			assert.Equal(t, c.coinsSub, got.CoinsSubtitle)
			assert.Equal(t, c.streakSub, got.StreakSubtitle)
			assert.Equal(t, c.lifeSub, got.LifelinesSubtitle)
		})
	}
}

func TestResolveState_ActiveStreakPastLadder(t *testing.T) {
	got := ResolveState(StateInput{HasLoggedEver: true, EffectiveDay: 40, LifelinesTotal: 3})
	assert.Equal(t, "active_streak_unlogged", got.State)
	assert.Equal(t, "Keep your streak going!", got.Heading, "no next ladder day past 35")
}
