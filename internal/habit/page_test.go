package habit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReorderBanner(t *testing.T) {
	assert.Nil(t, BuildReorderBanner(nil, 1))
	d := 20
	assert.Nil(t, BuildReorderBanner(&d, 1), "before day 21 there is no banner")
	d = 21
	b := BuildReorderBanner(&d, 1)
	require.NotNil(t, b)
	assert.Equal(t, "21 days since the last order was placed.", b["text"])
	assert.Equal(t, 14, b["daysToPause"])
	assert.Equal(t, false, b["paused"])
	assert.Equal(t, map[string]any{"label": "Order Next Kit", "action": "reorder"}, b["cta"])
	d = 36
	b = BuildReorderBanner(&d, 1)
	require.NotNil(t, b)
	assert.Equal(t, true, b["paused"], "pauses at 30n+5")
	assert.Equal(t, 0, b["daysToPause"])
	// singular day copy
	one := 1
	single := BuildReorderBanner(&one, 0)
	assert.Nil(t, single, "day 1 is before the window even for kitCount 0 → 1")
	// bulk kit widens the window: 2 kits → shows from day 51, pauses at 65
	d = 50
	assert.Nil(t, BuildReorderBanner(&d, 2), "still inside a 2-kit window")
	d = 55
	b = BuildReorderBanner(&d, 2)
	require.NotNil(t, b)
	assert.Equal(t, 2, b["kitCount"])
	assert.Equal(t, 10, b["daysToPause"])
	assert.Equal(t, false, b["paused"])
}

func TestDailyStripDays(t *testing.T) {
	days := []DayCell{}
	for i := 0; i < 15; i++ {
		days = append(days, DayCell{Date: string(rune('a' + i))})
	}
	days[7].IsToday = true
	days[7].Date = "today"
	days[10].IsRewardDay = true
	strip := DailyStripDays(days, "today")
	require.Len(t, strip, 7)
	assert.True(t, strip[6].IsRewardDay, "the strip ends on the next reward day")

	// no reward day ahead → today + 6
	plain := make([]DayCell, 15)
	for i := range plain {
		plain[i].Date = string(rune('a' + i))
	}
	plain[2].IsToday = true
	plain[2].Date = "today"
	strip = DailyStripDays(plain, "today")
	require.Len(t, strip, 7)
	assert.Equal(t, "today", strip[0].Date)
}

func TestDailyLogDateLabel(t *testing.T) {
	lbl := DailyLogDateLabel(DayCell{Date: "2026-04-02"})
	require.NotNil(t, lbl)
	assert.Equal(t, "2 Apr", *lbl)
	lbl = DailyLogDateLabel(DayCell{Date: "2026-04-02", IsToday: true})
	require.NotNil(t, lbl)
	assert.Equal(t, "2 Apr, Today", *lbl)
	assert.Nil(t, DailyLogDateLabel(DayCell{Date: "nope"}))
}

func TestBuildRewardScreen(t *testing.T) {
	core := &LogAndEarn{Stats: &Stats{Streak: Stat{Value: 6}}}
	// normal mode, not logged today
	daysToReward := 1
	out := BuildRewardScreen(core, &DayCell{Coins: 30}, &daysToReward, nil, nil, true, nil)
	assert.Equal(t, "normal", out["mode"])
	assert.Equal(t, 7, out["streak"], "projected streak adds today's log")
	assert.Equal(t, "7-Day Streak!", out["title"])
	assert.Equal(t, "30 Coins Earned", out["coinsEarnedText"])
	assert.Equal(t, "Log 1 more day to scratch mystery reward", out["subCopy"])

	// reward day, unclaimed
	today := &DayCell{Coins: 100, IsRewardDay: true}
	out = BuildRewardScreen(core, today, nil, nil, &ActiveCardRef{Status: "active"}, true, nil)
	assert.Equal(t, "reward", out["mode"])
	assert.Equal(t, false, out["rewardClaimed"])
	assert.Equal(t, "7-day streak reward unlocked!", out["title"])
	assert.Equal(t, "Scratch to reveal your bonus coins", out["subtitle"])
	card := out["scratchCard"].(map[string]any)
	assert.Equal(t, "active", card["status"])

	// reward day, already claimed (no active card)
	out = BuildRewardScreen(core, today, nil, nil, nil, true, nil)
	assert.Equal(t, true, out["rewardClaimed"])
	assert.Equal(t, "7-day streak reward claimed", out["title"])
	assert.Equal(t, "Rewards already credited", out["subtitle"])

	// scratch cards disabled → autoCreditOnComplete and always claimed
	out = BuildRewardScreen(core, today, nil, nil, nil, false, nil)
	card = out["scratchCard"].(map[string]any)
	assert.Equal(t, false, card["autoCreditOnComplete"])
	assert.Equal(t, true, out["rewardClaimed"])

	// logged today → projected streak unchanged
	out = BuildRewardScreen(core, &DayCell{Coins: 30, Logged: true}, nil, nil, nil, true, nil)
	assert.Equal(t, 6, out["streak"])
	assert.Equal(t, true, out["loggedToday"])
}
