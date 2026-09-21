package legacy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
)

func TestSplitDosage(t *testing.T) {
	mk := func(code string, morning, evening bool) map[string]any {
		return map[string]any{"product_id": "p", "dosageCode": code, "morningCheckIns": morning, "eveningCheckIns": evening}
	}
	cases := []struct {
		code   string
		weekly bool
		sub    string
		texts  []string
		times  []string
	}{
		{DosageOnceAWeek, true, "MIN ONCE IN A WEEK", []string{"Log for the day"}, []string{"ANYTIME_IN_DAY"}},
		{DosageTwiceAWeek, true, "USE 2 TIMES IN A WEEK", []string{"Log for the day"}, []string{"ANYTIME_IN_DAY"}},
		{DosageThriceAWeek, true, "USE 3 TIMES IN A WEEK", []string{"Log for the day"}, []string{"ANYTIME_IN_DAY"}},
		{DosageTwiceOrThriceAWeek, true, "USE 2 OR 3 TIMES IN A WEEK", []string{"Log for the day"}, []string{"ANYTIME_IN_DAY"}},
		{Dosage100, false, "ONCE DAILY", []string{"1 Tablet (Morning)"}, []string{"MORNING"}},
		{Dosage200, false, "ONCE DAILY", []string{"2 Tablet (Morning)"}, []string{"MORNING"}},
		{Dosage001, false, "ONCE DAILY", []string{"1 Tablet (Evening)"}, []string{"EVENING"}},
		{Dosage002, false, "ONCE DAILY", []string{"2 Tablet (Evening)"}, []string{"EVENING"}},
		{Dosage101, false, "TWICE DAILY", []string{"1 Tablet (Morning)", "1 Tablet (Evening)"}, []string{"MORNING", "EVENING"}},
		{Dosage202, false, "TWICE DAILY", []string{"2 Tablet (Morning)", "2 Tablet (Evening)"}, []string{"MORNING", "EVENING"}},
		{Dosage111, false, "ONCE DAILY", []string{"1ml (Morning)", "1ml (Evening)"}, []string{"MORNING", "EVENING"}},
		{Dosage1ml00, false, "ONCE DAILY", []string{"1ml (Morning)"}, []string{"MORNING"}},
		{Dosage001ml, false, "ONCE DAILY", []string{"1ml (Evening)"}, []string{"EVENING"}},
		{Dosage1ml01ml, false, "TWICE DAILY", []string{"1ml (Morning)", "1ml (Evening)"}, []string{"MORNING", "EVENING"}},
		{DosageAsDirected, false, "ONCE DAILY", []string{"As directed (By Dr.)", "As directed (By Dr.)"}, []string{"MORNING", "EVENING"}},
		{Dosage010, false, "ONCE DAILY", []string{"1ml (Morning)", "1ml (Evening)"}, []string{"MORNING", "EVENING"}},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			daily, weekly := SplitDosage([]map[string]any{mk(c.code, true, false)})
			bucket := daily
			if c.weekly {
				assert.Empty(t, daily)
				bucket = weekly
			} else {
				assert.Empty(t, weekly)
			}
			require.Len(t, bucket, 1)
			assert.Equal(t, c.sub, bucket[0]["subHeading"])
			texts := bucket[0]["dosageDisplayText"].([]DosageText)
			require.Len(t, texts, len(c.texts))
			for i := range texts {
				assert.Equal(t, c.texts[i], texts[i].Text)
				assert.Equal(t, c.times[i], texts[i].LogTimeInDay)
			}
		})
	}
	// weekly isMedicineLogged is morning && evening
	_, weekly := SplitDosage([]map[string]any{mk(DosageOnceAWeek, true, true)})
	assert.True(t, weekly[0]["dosageDisplayText"].([]DosageText)[0].IsMedicineLogged)
}

func TestIsOrderEligibleForLog(t *testing.T) {
	before := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	assert.True(t, IsOrderEligibleForLog(OrderEligibleForLogCaseID, before))
	assert.False(t, IsOrderEligibleForLog(OrderEligibleForLogCaseID, after))
	assert.False(t, IsOrderEligibleForLog("someone-else", before))
}

func TestGetBahLogForGivenDate(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	po.byUser = orderFixture(now)
	po.products = productFixture()

	// no log for today → POST needed
	out, err := s.GetBahLogForGivenDate(ctx, nil, "u1", true, "c1", 83)
	require.NoError(t, err)
	assert.True(t, out.IsPostAPINeeded)
	assert.False(t, out.IsPutAPINeeded)
	assert.Equal(t, "Daily Dosage", out.DailyDosageTitle)
	assert.Equal(t, "", out.WeeklyDosageTitle, "empty when no weekly items")
	assert.Equal(t, BahVideoFemale, out.BahVideo.Female)
	assert.NotEmpty(t, out.BahChallengeEntryPointConfig.Title)
	require.Len(t, out.DailyDosage, 1)

	// with a log for today → PUT needed
	_, err = s.Store.CreateActivityLog(ctx, &models.ActivityLog{UserID: "u1", CheckInsForDate: common.ISTShift(now),
		ProductPrescriptions: []map[string]any{{"product_id": "41645770309810", "dosageCode": "1-0-1", "morningCheckIns": false, "eveningCheckIns": false}},
		IsValidForStreak:     true, IsActive: true})
	require.NoError(t, err)
	out, err = s.GetBahLogForGivenDate(ctx, nil, "u1", true, "c1", 83)
	require.NoError(t, err)
	assert.False(t, out.IsPostAPINeeded)
	assert.True(t, out.IsPutAPINeeded)
	assert.True(t, out.OrderEligibleForLog)

	// appVersion 85 filters archived products
	_, err = s.Store.AddArchivedProduct(ctx, "u1", "41645770309810", now)
	require.NoError(t, err)
	out, err = s.GetBahLogForGivenDate(ctx, nil, "u1", true, "c1", 85)
	require.NoError(t, err)
	assert.Empty(t, out.DailyDosage)
	require.Len(t, out.ArchivedProduct, 1)
	// below 85 the filter does not apply
	out, err = s.GetBahLogForGivenDate(ctx, nil, "u1", true, "c1", 83)
	require.NoError(t, err)
	assert.Len(t, out.DailyDosage, 1)
	assert.Empty(t, out.ArchivedProduct)
}

func TestSaveActivityLogsAndCreateStreakAndGiveRewards(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	po.userCase = pgUserCase("c1", "M")
	po.byUser = orderFixture(now)
	s.Dispatcher = NewDispatcher(2, 8, s.Log)
	defer s.Dispatcher.Shutdown(context.Background())

	pp := []map[string]any{{"product_id": "p1", "name": "n", "Dosage": "1-0-1", "dosageCode": "1-0-1",
		"morningCheckIns": true, "eveningCheckIns": false, "bothCheckInsRequired": false, "image_url": map[string]any{}}}

	res, err := s.SaveActivityLogsAndCreateStreakAndGiveRewards(ctx, LogActivityInput{UserID: "u1", IsLogForToday: true, ProductPrescriptions: pp})
	require.NoError(t, err)
	assert.Contains(t, res.Message, "User checked in successfully for date")
	b, err := json.Marshal(res)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"scratchCard":null`)

	// duplicate same-day log → saveActivityLogs returns nil, message still success
	res, err = s.SaveActivityLogsAndCreateStreakAndGiveRewards(ctx, LogActivityInput{UserID: "u1", IsLogForToday: true, ProductPrescriptions: pp})
	require.NoError(t, err)
	assert.Contains(t, res.Message, "User checked in successfully")
	logs, err := s.Store.FindAllActivityLogs(ctx, "u1", nil)
	require.NoError(t, err)
	assert.Len(t, logs, 1, "no duplicate document")

	// Logging for yesterday while only today is logged is allowed: the guard looks for a log on
	// the target day (yesterday), not on today. This mirrors api-server's range
	// [zero(checkInDate), zero(clientNow)).
	res, err = s.SaveActivityLogsAndCreateStreakAndGiveRewards(ctx, LogActivityInput{UserID: "u1", IsLogForToday: false, ProductPrescriptions: pp})
	require.NoError(t, err)
	assert.Contains(t, res.Message, "User checked in successfully for date")
	logs, err = s.Store.FindAllActivityLogs(ctx, "u1", nil)
	require.NoError(t, err)
	assert.Len(t, logs, 2, "yesterday is now logged too")

	// Repeating it now trips the guard, and the scratchCard key is omitted from that response.
	res, err = s.SaveActivityLogsAndCreateStreakAndGiveRewards(ctx, LogActivityInput{UserID: "u1", IsLogForToday: false, ProductPrescriptions: pp})
	require.NoError(t, err)
	assert.Contains(t, res.Message, "User cannot log for date")
	assert.Contains(t, res.Message, "GMT+0000 (Coordinated Universal Time)")
	b, err = json.Marshal(res)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "scratchCard")
}

func TestSaveActivityLogs_HabitTrackerReturnsCard(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	po.userCase = pgUserCase("c1", "M")
	minted := true
	s.Habit = &fakeHabit{result: HabitCreditResult{Success: true, Minted: &minted, Card: map[string]any{"id": "card-1"}}}
	pp := []map[string]any{{"product_id": "p1", "morningCheckIns": true, "eveningCheckIns": false}}
	res, err := s.SaveActivityLogsAndCreateStreakAndGiveRewards(context.Background(),
		LogActivityInput{UserID: "u9", IsLogForToday: true, ProductPrescriptions: pp, IsHabitTracker: true})
	require.NoError(t, err)
	card, ok := res.ScratchCard.(map[string]any)
	require.True(t, ok, "habit tracker path returns the minted card synchronously")
	assert.Equal(t, "card-1", card["id"])
}

func TestSaveActivityLogs_BlankPrescriptions(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	_, err := s.SaveActivityLogs(context.Background(), "u1", common.ISTShift(now), nil)
	require.Error(t, err)
	assert.Equal(t, MsgBlankPrescriptionArray, err.Error())
	assert.Equal(t, 400, common.StatusOf(err))
}

func TestUpdateMultipleMedicineLogForUser(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	ctx := context.Background()
	ok := map[string]string{"message": MsgMultipleLogSuccess}

	// empty payload → success no-op
	got, err := s.UpdateMultipleMedicineLogForUser(ctx, MultipleLogInput{UserID: "u1", IsLogForToday: true})
	require.NoError(t, err)
	assert.Equal(t, ok, got)

	// missing log → success no-op
	got, err = s.UpdateMultipleMedicineLogForUser(ctx, MultipleLogInput{UserID: "u1", IsLogForToday: true,
		LogProductDetail: []map[string]any{{"productId": float64(1), "morningCheckIns": true, "eveningCheckIns": true}}})
	require.NoError(t, err)
	assert.Equal(t, ok, got)

	// real update
	logDoc, err := s.Store.CreateActivityLog(ctx, &models.ActivityLog{UserID: "u1", CheckInsForDate: common.ISTShift(now),
		ProductPrescriptions: []map[string]any{{"product_id": "1", "morningCheckIns": false, "eveningCheckIns": false}},
		IsValidForStreak:     true, IsActive: true})
	require.NoError(t, err)
	got, err = s.UpdateMultipleMedicineLogForUser(ctx, MultipleLogInput{UserID: "u1", IsLogForToday: true,
		LogProductDetail: []map[string]any{{"productId": float64(1), "morningCheckIns": true, "eveningCheckIns": true}}})
	require.NoError(t, err)
	assert.Equal(t, ok, got)
	updated, err := s.Store.FindActivityLogInRange(ctx, "u1", dayRangeFor(now), false)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, updated.ProductPrescriptions[0]["morningCheckIns"].(bool))
	assert.True(t, updated.ProductPrescriptions[0]["eveningCheckIns"].(bool))
	_ = logDoc

	// yesterday when today exists → success no-op
	got, err = s.UpdateMultipleMedicineLogForUser(ctx, MultipleLogInput{UserID: "u1", IsLogForToday: false,
		LogProductDetail: []map[string]any{{"productId": float64(1), "morningCheckIns": true, "eveningCheckIns": true}}})
	require.NoError(t, err)
	assert.Equal(t, ok, got)
}
