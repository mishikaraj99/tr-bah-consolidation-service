package legacy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 6, 0, 0, 0, time.UTC) }

func TestCreateStreakLogForUser(t *testing.T) {
	now := day(2026, 9, 18)
	s, ccd, _ := newTestService(t, now)
	ctx := context.Background()

	// first ever log
	r, err := s.CreateStreakLogForUser(ctx, "u1", day(2026, 9, 10), true, "c1")
	require.NoError(t, err)
	require.NotNil(t, r.Streak)
	assert.Equal(t, 1, r.Streak.StreakAchieveDays)
	assert.False(t, r.IsStreakBreaked)
	require.Len(t, ccd.events, 1)
	assert.Equal(t, "ACTIVITY_LOG", ccd.events[0]["eventType"])
	assert.Equal(t, 1, ccd.events[0]["payload"].(map[string]any)["streakCount"])

	// same day → alreadyLogged, no new CCD
	r, err = s.CreateStreakLogForUser(ctx, "u1", day(2026, 9, 10), true, "c1")
	require.NoError(t, err)
	assert.True(t, r.AlreadyLogged)
	assert.Len(t, ccd.events, 1)

	// next day → 2
	r, err = s.CreateStreakLogForUser(ctx, "u1", day(2026, 9, 11), true, "c1")
	require.NoError(t, err)
	assert.Equal(t, 2, r.Streak.StreakAchieveDays)
	assert.Equal(t, 2, r.Streak.LongestStreakDays)
	assert.False(t, r.IsStreakBreaked)

	// gap of 2 days → break, reset to 1, first_date_of_log moves
	r, err = s.CreateStreakLogForUser(ctx, "u1", day(2026, 9, 13), true, "c1")
	require.NoError(t, err)
	assert.True(t, r.IsStreakBreaked)
	assert.Equal(t, 1, r.Streak.StreakAchieveDays)
	assert.Equal(t, day(2026, 9, 13).Format("2006-01-02"), r.Streak.FirstDateOfLog.Format("2006-01-02"))
	assert.Equal(t, 2, r.Streak.LongestStreakDays, "longest is preserved")

	// invalid → 400
	_, err = s.CreateStreakLogForUser(ctx, "u1", now, false, "c1")
	assert.Equal(t, 400, statusOf(err))
	assert.Equal(t, MsgNotEligibleToCreateStreak, err.Error())
}

func TestCreateStreakLogForUser_ResetAt21(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	_, err := s.CreateStreakLogForUser(ctx, "u2", day(2026, 9, 1), true, "c2")
	require.NoError(t, err)
	_, err = s.Store.UpdateActiveStreak(ctx, "u2", map[string]any{"streak_achieve_days": 21, "longest_streak_days": 21})
	require.NoError(t, err)
	r, err := s.CreateStreakLogForUser(ctx, "u2", day(2026, 9, 2), true, "c2")
	require.NoError(t, err)
	assert.True(t, r.IsStreakBreaked, "day 21 resets on the next log")
	assert.Equal(t, 1, r.Streak.StreakAchieveDays)
}

func TestStreakHelpers(t *testing.T) {
	last := day(2026, 9, 15)
	now := day(2026, 9, 18)
	assert.Equal(t, 0, StreakBrokenRecompute(7, &last, now), "gap 3 breaks")
	l2 := day(2026, 9, 17)
	assert.Equal(t, 7, StreakBrokenRecompute(7, &l2, now), "gap 1 survives")
	assert.Equal(t, 0, StreakBrokenRecompute(21, &l2, now), "day 21 + 1 breaks")
	assert.Equal(t, 5, StreakBrokenRecompute(5, nil, now))

	assert.True(t, IsLoggedToday(&now, now))
	assert.False(t, IsLoggedToday(&l2, now))
	assert.False(t, IsLoggedToday(nil, now))

	assert.Equal(t, "M", GenderOf(""))
	assert.Equal(t, "F", GenderOf("f"))
	assert.Equal(t, "M", GenderOf("m"))
	assert.Equal(t, "O", GenderOf("o"))
}
