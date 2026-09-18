package habit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/orders"
)

func d(y, m, day int) time.Time { return time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC) }

func setOf(keys ...string) map[string]bool {
	out := map[string]bool{}
	for _, k := range keys {
		out[k] = true
	}
	return out
}

func TestLiveRunAndLastRunBeforeGap(t *testing.T) {
	today := d(2026, 9, 18)
	valid := setOf("2026-09-16", "2026-09-17", "2026-09-18")
	assert.Equal(t, 3, LiveRun(valid, today))
	// not logged today → run counted from yesterday
	valid = setOf("2026-09-16", "2026-09-17")
	assert.Equal(t, 2, LiveRun(valid, today))
	// gap → live run 0, but the prior run is found
	valid = setOf("2026-09-10", "2026-09-11", "2026-09-12")
	assert.Equal(t, 0, LiveRun(valid, today))
	assert.Equal(t, 3, LastRunBeforeGap(valid, today))
}

func TestEarningPaused(t *testing.T) {
	today := d(2026, 9, 18)
	open := d(2026, 9, 1)
	assert.True(t, EarningPaused(35, &open, today, 1), "log cap")
	assert.False(t, EarningPaused(10, &open, today, 1), "17 days elapsed of 40")
	old := d(2026, 8, 1)
	assert.True(t, EarningPaused(10, &old, today, 1), "48 days elapsed of 40")
	assert.False(t, EarningPaused(10, nil, today, 1), "no window open")
	assert.False(t, EarningPaused(10, &old, today, 2), "bulk kit widens the window to 70")
}

func TestBuildLogAndEarn(t *testing.T) {
	today := d(2026, 9, 18)
	valid := setOf("2026-09-16", "2026-09-17", "2026-09-18")
	out := BuildLogAndEarn(LogAndEarnInput{Today: today, CurrentStreak: 3, LoggedDates: valid, ValidDates: valid, CoinBalance: 250})
	require.NotNil(t, out.Calendar)
	assert.Len(t, out.Calendar.Days, 15)
	assert.Equal(t, "2026-09-11", out.Calendar.StartDate)
	assert.Equal(t, "2026-09-18", out.Calendar.TodayDate)
	var todayCell *DayCell
	for i := range out.Calendar.Days {
		if out.Calendar.Days[i].IsToday {
			todayCell = &out.Calendar.Days[i]
		}
	}
	require.NotNil(t, todayCell)
	assert.True(t, todayCell.Logged)
	assert.Equal(t, 30, todayCell.Coins, "day 3 is a daily-tier day")
	// day 7 of the run (2026-09-22) is a ladder day worth 100
	for _, c := range out.Calendar.Days {
		if c.Date == "2026-09-22" {
			assert.True(t, c.IsRewardDay)
			assert.Equal(t, 100, c.Coins)
		}
	}
	assert.Equal(t, "KIT TRACKER", out.Header.Overline)
	assert.Equal(t, 250, out.Stats.Coins.Value)
	assert.Equal(t, "₹25 off", out.Stats.Coins.Subtitle)
	assert.Equal(t, 3, out.Stats.Streak.Value)
	assert.Equal(t, 3, out.Stats.Lifelines.Total)
}

func TestBuildLogAndEarn_BridgedCarriesRun(t *testing.T) {
	today := d(2026, 9, 18)
	valid := setOf("2026-09-15", "2026-09-17", "2026-09-18")
	bridged := setOf("2026-09-16")
	valid["2026-09-16"] = true
	out := BuildLogAndEarn(LogAndEarnInput{Today: today, LoggedDates: setOf("2026-09-15", "2026-09-17", "2026-09-18"),
		ValidDates: valid, BridgedDays: bridged, CoinBalance: 0})
	for _, c := range out.Calendar.Days {
		if c.Date == "2026-09-16" {
			assert.True(t, c.LifelineUsed)
			assert.False(t, c.IsStreakBreak, "a bridged day is not a break")
		}
	}
}

func TestBuildIntro(t *testing.T) {
	out := BuildIntro()
	assert.Equal(t, "kit_arriving_intro", out.State)
	require.NotNil(t, out.HabitTrackerDisabled)
	assert.True(t, *out.HabitTrackerDisabled)
	assert.False(t, out.LoggingEnabled)
	assert.Equal(t, "Use kit regularly to get discounts", out.Header.Heading)
	assert.Equal(t, "Explore Now", out.CTA.Label)
	require.Len(t, out.Benefits, 3)
	assert.Equal(t, "Earn Coins", out.Benefits[0]["label"])
	require.Len(t, out.Milestones, 4)
	assert.Equal(t, "1st Log", out.Milestones[0]["label"])
	assert.Equal(t, false, out.Milestones[0]["locked"])
	assert.Equal(t, true, out.Milestones[1]["locked"])
	assert.Equal(t, Assets["kitBox"], out.Kit["image"])
}

func TestDeriveOrderFlags(t *testing.T) {
	today := d(2026, 9, 18)
	del := d(2026, 9, 10)
	delivered := orders.Order{Status: "delivered", CreatedAt: d(2026, 9, 7), DeliveryDate: &del}
	placed := orders.Order{Status: "placed", CreatedAt: d(2026, 9, 15)}

	f := DeriveOrderFlags([]orders.Order{placed, delivered}, map[string]bool{}, false, true, today)
	assert.True(t, f.NewOrderPlacedNotDelivered)
	assert.True(t, f.NewKitDeliveredNotLogged, "delivered kit with no log since delivery")

	f = DeriveOrderFlags([]orders.Order{delivered}, setOf("2026-09-12"), false, true, today)
	assert.False(t, f.NewOrderPlacedNotDelivered)
	assert.False(t, f.NewKitDeliveredNotLogged, "logged after delivery")
}

func TestBadges(t *testing.T) {
	windows := []orders.KitWindow{
		{KitNumber: 1, Start: d(2026, 6, 1), End: d(2026, 7, 1)},
		{KitNumber: 2, Start: d(2026, 7, 1), End: d(2026, 7, 31)},
	}
	valid := map[string]bool{}
	for i := 0; i < 30; i++ {
		valid[dateKey(d(2026, 6, 1).AddDate(0, 0, i))] = true
	}
	for i := 0; i < 5; i++ {
		valid[dateKey(d(2026, 7, 1).AddDate(0, 0, i))] = true
	}
	all, earnedCount, newly := DeriveBadges(windows, valid, map[int]time.Time{}, "F")
	assert.Equal(t, 1, earnedCount)
	assert.Equal(t, []int{1}, newly)
	assert.Len(t, all, 21, "at least the full ladder")
	assert.True(t, all[0].Earned)
	assert.Equal(t, 30, all[0].LogsInKit)
	assert.Equal(t, "Kit 1", all[0].Name)
	require.NotNil(t, all[0].Image)
	assert.Contains(t, *all[0].Image, "Female_badges_1.png")
	assert.False(t, all[1].Earned)
	assert.Equal(t, 5, all[1].LogsInKit)
	require.NotNil(t, all[1].Image)
	assert.Contains(t, *all[1].Image, "Disable_Badge_2.png")
	assert.Equal(t, "Upcoming", all[1].EarnedAt)

	display := SelectBadgesForDisplay(all)
	require.Len(t, display, 2, "earned + exactly one upcoming")
	assert.Equal(t, 2, display[0].KitNumber, "newest first")
	require.NotNil(t, display[0].Upcoming)
	assert.True(t, *display[0].Upcoming)
	assert.False(t, *display[1].Upcoming)

	assert.Nil(t, ResolveBadgeImage(22, true, "M"), "no art past kit 21")
}

func TestComputeRunStateAndMonthCalendar(t *testing.T) {
	today := d(2026, 9, 18)
	valid := setOf("2026-09-15", "2026-09-16", "2026-09-17", "2026-09-18")
	start := d(2026, 9, 1)
	rs := ComputeRunState(today, start, &start, valid)
	assert.Equal(t, 4, rs.LiveRun)
	assert.Equal(t, 4, rs.EffectiveDay, "logged today → effectiveDay == liveRun")
	assert.True(t, rs.BandDays["2026-09-15"])

	m := BuildMonthCalendar(MonthCalendarInput{Month: 9, Year: 2026, Today: today, ActiveStart: &start,
		LoggedDates: valid, ValidDates: valid, LifelineDates: map[string]bool{}, RunState: rs, RangeStart: start})
	assert.Equal(t, "September 2026", m.MonthLabel)
	assert.Len(t, m.Days, 30)
	assert.Equal(t, 4, m.Summary.DaysLogged)
	assert.Equal(t, 14, m.Summary.DaysMissed, "1st-14th are missed")
	byDate := map[string]MonthDay{}
	for _, day := range m.Days {
		byDate[day.Date] = day
	}
	assert.Equal(t, "logged", byDate["2026-09-18"].State)
	assert.True(t, byDate["2026-09-18"].IsToday)
	assert.Equal(t, "future", byDate["2026-09-20"].State)
	assert.Equal(t, "missed", byDate["2026-09-10"].State)
	require.NotNil(t, byDate["2026-09-18"].Coins)
	assert.Equal(t, 30, *byDate["2026-09-18"].Coins)
	assert.Nil(t, byDate["2026-09-10"].Coins, "missed non-reward days carry no coins")
	assert.Len(t, m.Legend, 4)
}
