package habit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCoinsBottomSheet(t *testing.T) {
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	expiry := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	sheet := BuildCoinsBottomSheet(250, &expiry, nil, now)
	header := sheet["header"].(map[string]any)
	assert.Equal(t, "250 Coin balance", header["title"])
	assert.Equal(t, "Get ₹25 off on your next kit", header["pill"])
	assert.Equal(t, "10 coins = ₹1", header["conversion"])
	exp := sheet["expiry"].(map[string]any)
	assert.Equal(t, "250 coins expire on 01 Oct 2026", exp["text"])
	weekly := sheet["weeklyCoins"].([]map[string]any)
	require.Len(t, weekly, 5)
	assert.Equal(t, "Week 1", weekly[0]["week"])
	assert.Equal(t, "Day 1-6", weekly[0]["days"])
	assert.Equal(t, "30 coins per day", weekly[0]["label"])
	assert.Equal(t, "Day 29-34", weekly[4]["days"])
	assert.Equal(t, 70, weekly[4]["coins"])
	note := sheet["note"].(map[string]any)
	assert.Equal(t, "Note: Daily coins reset to 30 per day if you", note["title"])

	zero := BuildCoinsBottomSheet(0, &expiry, nil, now)
	zh := zero["header"].(map[string]any)
	assert.Equal(t, "Log daily to start earning coins", zh["title"])
	assert.Equal(t, "Earn coins, unlock rewards, & save on your next kit", zh["subtitle"])
	assert.Nil(t, zero["expiry"])
}

func TestBuildStreakBottomSheet(t *testing.T) {
	zero := BuildStreakBottomSheet(0)["header"].(map[string]any)
	assert.Equal(t, "Start your streak today", zero["title"])
	assert.Equal(t, "Streak is the number of days in a row you have logged, even if a lifeline was used", zero["subtitle"])

	ten := BuildStreakBottomSheet(10)["header"].(map[string]any)
	assert.Equal(t, "10-Day Streak", ten["title"])
	assert.Equal(t, "Keep going to unlock your Day 14 bonus.", ten["subtitle"])

	past := BuildStreakBottomSheet(40)["header"].(map[string]any)
	assert.Equal(t, "Keep your streak going!", past["subtitle"])
}

func TestBuildLifelineBottomSheet(t *testing.T) {
	none := BuildLifelineBottomSheet(0, 3)
	h := none["header"].(map[string]any)
	assert.Equal(t, "3 Lifelines", h["title"])
	assert.Equal(t, "If you miss logging a day, we use 1 lifeline to keep your streak active.", h["subtitle"])
	hearts := h["hearts"].([]string)
	require.Len(t, hearts, 3)
	assert.Equal(t, Assets["lifeline"], hearts[0])

	one := BuildLifelineBottomSheet(1, 3)
	h1 := one["header"].(map[string]any)
	assert.Equal(t, "2 of 3 Lifelines left", h1["title"])
	assert.Equal(t, Assets["lifelineUsed"], h1["hearts"].([]string)[0])
	legend := one["legend"].([]map[string]any)
	require.Len(t, legend, 4)
	assert.Equal(t, "Full Lifelines", legend[0]["label"])
	assert.Equal(t, "1/3 Lifeline Used", legend[1]["label"])
	assert.Equal(t, "2/3 Lifelines Used", legend[2]["label"])
	assert.Equal(t, "All Lifelines Used", legend[3]["label"])
	info := one["infoPoints"].([]map[string]any)
	assert.Equal(t, "You get 3 lifelines per kit. Unused ones don't carry forward to next month.", info[1]["text"])
}
