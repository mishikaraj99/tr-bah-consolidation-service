package habit

import "fmt"

// StateInput is resolveHabitTrackerState's argument set.
type StateInput struct {
	LoggedToday                bool
	HasLoggedEver              bool
	RewardToday                bool
	DayBeforeReward            bool
	StreakBroken               bool
	BrokeOnRewardDay           bool
	KitArrivingIntro           bool
	NewOrderPlacedNotDelivered bool
	NewKitDeliveredNotLogged   bool
	EarningPaused              bool
	EffectiveDay               int
	LifelinesUsed              int
	LifelinesTotal             int
	CoinBalance                int
}

// Resolved is the resolver's output.
type Resolved struct {
	State             string
	Heading           string
	CtaLabel          string
	CoinsSubtitle     string
	StreakSubtitle    string
	LifelinesSubtitle string
}

// Rupees mirrors the resolver's rupees(): "₹<floor(balance/10)> off".
func Rupees(coinBalance int) string {
	if coinBalance < 0 {
		coinBalance = 0
	}
	return fmt.Sprintf("₹%d off", coinBalance/CoinToRupeeDivisor)
}

// LifelineLabel mirrors lifelineLabel().
func LifelineLabel(used, total int) string {
	switch {
	case used <= 0:
		return fmt.Sprintf("%d Per Kit", total)
	case used >= total:
		return "All Used"
	default:
		return fmt.Sprintf("%d/%d Used", used, total)
	}
}

// DaysToNextReward mirrors daysToNextReward(): distance to the next ladder day, nil past 35.
func DaysToNextReward(day int) *int {
	for _, d := range LadderDays {
		if d > day {
			out := d - day
			return &out
		}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ResolveState ports habitTrackerStateResolver.js: 16 states, first match wins.
func ResolveState(in StateInput) Resolved {
	coins := Rupees(in.CoinBalance)
	actual := LifelineLabel(in.LifelinesUsed, in.LifelinesTotal)
	perKit := fmt.Sprintf("%d Per Kit", in.LifelinesTotal)
	mk := func(state, heading, cta, coinsSub, streakSub, lifeSub string) Resolved {
		return Resolved{State: state, Heading: heading, CtaLabel: cta, CoinsSubtitle: coinsSub, StreakSubtitle: streakSub, LifelinesSubtitle: lifeSub}
	}

	switch {
	case in.KitArrivingIntro:
		return mk("kit_arriving_intro", IntroHeading, IntroCtaLabel, coins, "Start Now", perKit)
	case in.EarningPaused && !in.LoggedToday:
		return mk("rewards_paused_unlogged", "Rewards are paused.\nOrder now to start earning again", "Log Now", coins, "Keep Going!", actual)
	case in.EarningPaused && in.LoggedToday:
		return mk("rewards_paused_logged", "Rewards are paused.\nOrder now to start earning again", "View Logs", coins, "Keep Going!", actual)
	case in.NewOrderPlacedNotDelivered:
		return mk("new_order_placed", "New kit loading! You can continue\nlogging your current kit.", "Log Now", coins, "Streak Intact", perKit)
	case in.StreakBroken && in.BrokeOnRewardDay:
		return mk("streak_broken_reward_day", "Your streak broke! Start afresh today", "Log Now", coins, "Start Today", actual)
	case in.StreakBroken:
		return mk("streak_broken_coins_day", "Your streak broke! Start afresh today", "Log Now", coins, "Start Today", actual)
	case in.LifelinesUsed >= in.LifelinesTotal && !in.LoggedToday:
		return mk("lifelines_all_used_unlogged", "No lifelines left\nLog today to keep your streak alive", "Log Now", coins, "Save Streak", "All Used")
	case in.LifelinesUsed > 0 && !in.LoggedToday:
		remaining := in.LifelinesTotal - in.LifelinesUsed
		heading := fmt.Sprintf("Streak saved! We used %d %s\n%d %s remaining",
			in.LifelinesUsed, plural(in.LifelinesUsed, "lifeline", "lifelines"), remaining, plural(remaining, "lifeline", "lifelines"))
		return mk("lifeline_used_unlogged", heading, "Log Now", coins, "Save Streak", fmt.Sprintf("%d/%d Used", in.LifelinesUsed, in.LifelinesTotal))
	case in.NewKitDeliveredNotLogged:
		return mk("new_kit_delivered", "Do not miss out on your rewards!\nLog your dose now", "Log Now", coins, "Keep Going!", perKit)
	case in.RewardToday && in.LoggedToday:
		return mk("reward_day_logged", "Bonus unlocked!\nNext weekly reward is waiting for you", "View Logs", coins, "Keep Going!", perKit)
	case in.RewardToday && !in.LoggedToday:
		return mk("reward_day_unlogged", "Today is the day!\nClaim your weekly reward", "Claim Reward", coins, "Keep Going!", perKit)
	case in.DayBeforeReward && in.LoggedToday:
		return mk("day_before_reward_logged", "All done for today\nLog tomorrow to earn the bonus", "View Logs", coins, "Keep Going!", perKit)
	case in.DayBeforeReward && !in.LoggedToday:
		return mk("day_before_reward_unlogged", "Tomorrow is bonus day,\nLog today to get there", "Log Now", coins, "Keep Going!", actual)
	case !in.LoggedToday && in.HasLoggedEver:
		heading := "Keep your streak going!"
		if n := DaysToNextReward(in.EffectiveDay - 1); n != nil {
			heading = fmt.Sprintf("%d days closer to the mystery bonus", *n)
		}
		return mk("active_streak_unlogged", heading, "Log Now", coins, "Keep Going!", actual)
	case in.LoggedToday:
		return mk("day_logged", "All done for today\nLog daily to earn the bonus reward", "View Logs", coins, "Keep Going!", actual)
	default:
		return mk("day1_never_logged", HeaderHeading, "Log Now", "Earn Discount", "Start Now", perKit)
	}
}
