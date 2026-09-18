package legacy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyStringsVerbatim(t *testing.T) {
	assert.Equal(t, "YOU WON 100 COINS! 🎊", PopupText.OneDay.H1)
	assert.Equal(t, "3 DAY STREAK 💰", PopupText.ThreeDays.H1)
	assert.Equal(t, "7 DAY STREAK 💎", PopupText.SevenDays.H1)
	assert.Equal(t, "21 DAY STREAK 🔥", PopupText.TwentyOneD.H1)
	assert.Equal(t, "HOW TO REDEEM COINS?", PopupText.CtaText)
	assert.Equal(t, "You are staying consistent. Let your coach know if you are facing any issues with treatment", FifteenDayCheckinSubtext)
	assert.Equal(t, "Total coins: {TotalCoins}, Max usable: {ClaimableCoin}", CoinNotApplied.SubTitle)
}

func TestPopupTextJSONKeys(t *testing.T) {
	b, err := json.Marshal(PopupText)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	for _, k := range []string{"_1_day", "_3_days", "_7_days", "_21_days", "ctaText", "dismissText"} {
		assert.Contains(t, m, k)
	}
}

func TestIdempotencyKeys(t *testing.T) {
	assert.Equal(t, "bah-legacy-existing_coins_streak-2026-09-18", RemarkMilestone("existing_coins_streak", "2026-09-18"))
	assert.Equal(t, "bah-legacy-first-log", RemarkFirstLog)
	assert.Equal(t, "bah-legacy-restart-2026-09-18", RemarkRestart("2026-09-18"))
}

func TestRewardLadders(t *testing.T) {
	assert.Equal(t, 100, StreakRewards[0].Coin)
	assert.Equal(t, 2000, StreakRewards[2].Coin)
	assert.Equal(t, 2500, O8PlusStreakRewards[21])
	assert.False(t, StreakRestartBonusEnabled)
	assert.True(t, StreakRestartBonusFemaleEnabled)
}
