package habit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyCoins(t *testing.T) {
	cases := map[int]int{1: 30, 6: 30, 7: 30, 8: 40, 14: 40, 15: 50, 21: 50, 22: 60, 28: 60, 29: 70, 35: 70, 60: 70}
	for day, want := range cases {
		assert.Equal(t, want, DailyCoins(day), "day %d", day)
	}
}

func TestResolveReward(t *testing.T) {
	assert.Nil(t, ResolveReward(0))
	assert.Nil(t, ResolveReward(36))
	r := ResolveReward(1)
	require.NotNil(t, r)
	assert.Equal(t, "daily", r.Type)
	assert.Equal(t, "habit-daily-week-1", r.Slug)
	assert.Equal(t, 30, r.Coins)
	r = ResolveReward(7)
	require.NotNil(t, r)
	assert.Equal(t, "ladder", r.Type)
	assert.Equal(t, "habit-ladder-7", r.Slug)
	assert.Equal(t, 100, r.Coins)
	r = ResolveReward(35)
	require.NotNil(t, r)
	assert.Equal(t, "ladder", r.Type)
	assert.Equal(t, 300, r.Coins)
	r = ResolveReward(8)
	require.NotNil(t, r)
	assert.Equal(t, "daily", r.Type)
	assert.Equal(t, 40, r.Coins)
}

func TestRemarksFor(t *testing.T) {
	d := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "habit-tracker-ladder-2026-09-18", RemarksFor(Reward{Type: "ladder"}, d))
	assert.Equal(t, "habit-tracker-daily-2026-09-18", RemarksFor(Reward{Type: "daily"}, d))
}

func TestDaysToNextRewardAndLabels(t *testing.T) {
	n := DaysToNextReward(0)
	require.NotNil(t, n)
	assert.Equal(t, 7, *n)
	n = DaysToNextReward(7)
	require.NotNil(t, n)
	assert.Equal(t, 7, *n)
	assert.Nil(t, DaysToNextReward(35))
	assert.Equal(t, "₹25 off", Rupees(250))
	assert.Equal(t, "₹0 off", Rupees(5))
	assert.Equal(t, "3 Per Kit", LifelineLabel(0, 3))
	assert.Equal(t, "1/3 Used", LifelineLabel(1, 3))
	assert.Equal(t, "All Used", LifelineLabel(3, 3))
}
