package habit

import (
	"fmt"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
)

// BuildCoinsBottomSheet ports buildHabitTrackerCoinsBottomSheet.
func BuildCoinsBottomSheet(coinBalance int, coinExpiryDate *time.Time, ords []orders.Order, now time.Time) map[string]any {
	rupees := coinBalance / CoinToRupeeDivisor
	header := map[string]any{"conversion": "10 coins = ₹1"}
	if coinBalance > 0 {
		header["title"] = fmt.Sprintf("%d Coin balance", coinBalance)
		header["pill"] = fmt.Sprintf("Get ₹%d off on your next kit", rupees)
	} else {
		header["title"] = "Log daily to start earning coins"
		header["subtitle"] = "Earn coins, unlock rewards, & save on your next kit"
	}
	var expiry any
	if coinBalance > 0 && coinExpiryDate != nil && !coinExpiryDate.IsZero() {
		expiry = map[string]any{
			"text": fmt.Sprintf("%d coins expire on %s", coinBalance, common.FormatMoment(*coinExpiryDate, "DD MMM YYYY")),
			"cta":  ResolveReorderCta(ords, now),
		}
	}
	weekly := make([]map[string]any, 0, len(DailyTiers))
	for _, t := range DailyTiers {
		coins := DailyCoinsBase + (t.Week-1)*DailyWeeklyStep
		weekly = append(weekly, map[string]any{
			"week":  fmt.Sprintf("Week %d", t.Week),
			"days":  fmt.Sprintf("Day %d-%d", 7*(t.Week-1)+1, 7*t.Week-1),
			"coins": coins,
			"label": fmt.Sprintf("%d coins per day", coins),
			"icon":  Assets["coins"],
		})
	}
	return map[string]any{
		"header":      header,
		"expiry":      expiry,
		"weeklyCoins": weekly,
		"bonus":       map[string]any{"icon": Assets["rewardUpcoming"], "text": "Unlock a bonus reward every 7 days you log"},
		"note": map[string]any{"title": "Note: Daily coins reset to 30 per day if you",
			"points": []string{"Break your streak", "Start a new kit"}},
	}
}

// ResolveReorderCta mirrors resolveReorderCta: nil until the kit is at least 21 days old.
func ResolveReorderCta(ords []orders.Order, now time.Time) any {
	details := orders.GetAllOrderDetails(ords, now)
	if details.MinDaysAfterOrderDelivered < ReorderCTAMinKitAge {
		return nil
	}
	return map[string]any{"label": ReorderCtaLabel, "action": "reorder"}
}

// BuildStreakBottomSheet ports buildHabitTrackerStreakBottomSheet.
func BuildStreakBottomSheet(currentStreak int) map[string]any {
	streak := currentStreak
	if streak < 0 {
		streak = 0
	}
	nextBonus := DaysToNextReward(streak)
	header := map[string]any{}
	if streak == 0 {
		header["title"] = "Start your streak today"
		header["subtitle"] = "Streak is the number of days in a row you have logged, even if a lifeline was used"
	} else {
		header["title"] = fmt.Sprintf("%d-Day Streak", streak)
		if nextBonus != nil {
			header["subtitle"] = fmt.Sprintf("Keep going to unlock your Day %d bonus.", streak+*nextBonus)
		} else {
			header["subtitle"] = "Keep your streak going!"
		}
	}
	return map[string]any{
		"header": header,
		"infoPoints": []map[string]any{
			{"icon": Assets["streak"], "text": "Streak stays alive when you log or when a lifeline is used.", "example": Assets["streakStaysAlive"]},
			{"icon": Assets["streak"], "text": "Streak breaks only if a log is missed and no lifeline is available.", "example": Assets["streakBreaksBS"]},
		},
	}
}

func heartsRow(used, total int) []string {
	out := make([]string, 0, total)
	for i := 0; i < total; i++ {
		if i < used {
			out = append(out, Assets["lifelineUsed"])
		} else {
			out = append(out, Assets["lifeline"])
		}
	}
	return out
}

// BuildLifelineBottomSheet ports buildHabitTrackerLifelineBottomSheet.
func BuildLifelineBottomSheet(lifelinesUsed, lifelinesTotal int) map[string]any {
	used := clamp(lifelinesUsed, 0, lifelinesTotal)
	remaining := lifelinesTotal - used
	header := map[string]any{}
	if used == 0 {
		header["title"] = fmt.Sprintf("%d Lifelines", lifelinesTotal)
		header["subtitle"] = "If you miss logging a day, we use 1 lifeline to keep your streak active."
		header["hearts"] = heartsRow(0, lifelinesTotal)
	} else {
		header["title"] = fmt.Sprintf("%d of %d Lifelines left", remaining, lifelinesTotal)
		header["subtitle"] = "Keep logging daily to save your remaining lifelines"
		header["hearts"] = heartsRow(used, lifelinesTotal)
	}
	legend := []map[string]any{{"hearts": heartsRow(0, lifelinesTotal), "label": "Full Lifelines"}}
	for u := 1; u <= lifelinesTotal; u++ {
		label := fmt.Sprintf("%d/%d Lifeline%s Used", u, lifelinesTotal, plural(u, "", "s"))
		if u == lifelinesTotal {
			label = "All Lifelines Used"
		}
		legend = append(legend, map[string]any{"hearts": heartsRow(u, lifelinesTotal), "label": label})
	}
	return map[string]any{
		"header": header,
		"infoPoints": []map[string]any{
			{"icon": Assets["coins"], "text": "You earn coins even when a lifeline is used."},
			{"icon": Assets["lifeline"], "text": fmt.Sprintf("You get %d lifelines per kit. Unused ones don't carry forward to next month.", lifelinesTotal)},
			{"icon": Assets["streak"], "text": "Your streak breaks only if you miss a day and have no lifelines left."},
		},
		"legend": legend,
	}
}
