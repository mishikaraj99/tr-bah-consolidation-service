package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

// DosageText is one dosageDisplayText entry.
type DosageText struct {
	Text             string `json:"text"`
	IsMedicineLogged bool   `json:"isMedicineLogged"`
	LogTimeInDay     string `json:"logTimeInDay"`
}

// BahVideo is the bahVideo payload.
type BahVideo struct {
	Female string `json:"female"`
	Male   string `json:"male"`
}

// BahLogForDate is the GET /bahLogForGivenDate response.
type BahLogForDate struct {
	DailyDosageTitle             string           `json:"dailyDosageTitle"`
	DailyDosage                  []map[string]any `json:"dailyDosage"`
	WeeklyDosageTitle            string           `json:"weeklyDosageTitle"`
	WeeklyDosage                 []map[string]any `json:"weeklyDosage"`
	IsPostAPINeeded              bool             `json:"isPostApiNeeded"`
	IsPutAPINeeded               bool             `json:"isPutApiNeeded"`
	OrderEligibleForLog          bool             `json:"orderEligibleForLog"`
	MedicineReordertext          any              `json:"medicineReordertext"`
	CoinExpiryText               string           `json:"coinExpiryText"`
	BahChallengeEntryPointConfig ChallengeBanner  `json:"bahChallengeEntryPointConfig"`
	BahVideo                     BahVideo         `json:"bahVideo"`
	ArchivedProduct              []map[string]any `json:"archivedProduct"`
}

func isWeeklyDosage(code string) bool {
	switch code {
	case DosageOnceAWeek, DosageTwiceAWeek, DosageThriceAWeek, DosageTwiceOrThriceAWeek:
		return true
	}
	return false
}

func asString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func asBool(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+2)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// SplitDosage ports the daily/weekly dosage split of getBahLogForGivenDate.
func SplitDosage(items []map[string]any) (daily, weekly []map[string]any) {
	daily, weekly = []map[string]any{}, []map[string]any{}
	for _, raw := range items {
		item := copyMap(raw)
		code := asString(item, "dosageCode")
		morning, evening := asBool(item, "morningCheckIns"), asBool(item, "eveningCheckIns")
		if isWeeklyDosage(code) {
			sub := "USE 2 TIMES IN A WEEK"
			switch code {
			case DosageOnceAWeek:
				sub = "MIN ONCE IN A WEEK"
			case DosageThriceAWeek:
				sub = "USE 3 TIMES IN A WEEK"
			case DosageTwiceOrThriceAWeek:
				sub = "USE 2 OR 3 TIMES IN A WEEK"
			}
			item["subHeading"] = sub
			item["dosageDisplayText"] = []DosageText{{Text: "Log for the day", IsMedicineLogged: morning && evening, LogTimeInDay: "ANYTIME_IN_DAY"}}
			weekly = append(weekly, item)
			continue
		}
		sub := "ONCE DAILY"
		switch code {
		case Dosage202, Dosage101, Dosage1ml01ml:
			sub = "TWICE DAILY"
		}
		item["subHeading"] = sub
		morningText, eveningText := "1ml (Morning)", "1ml (Evening)"
		switch code {
		case Dosage100, Dosage001, Dosage101:
			morningText, eveningText = "1 Tablet (Morning)", "1 Tablet (Evening)"
		case DosageAsDirected:
			morningText, eveningText = "As directed (By Dr.)", "As directed (By Dr.)"
		case Dosage200, Dosage002, Dosage202:
			morningText, eveningText = "2 Tablet (Morning)", "2 Tablet (Evening)"
		}
		maxDaily := []DosageText{
			{Text: morningText, IsMedicineLogged: morning, LogTimeInDay: "MORNING"},
			{Text: eveningText, IsMedicineLogged: evening, LogTimeInDay: "EVENING"},
		}
		switch code {
		case Dosage100, Dosage200, Dosage1ml00:
			item["dosageDisplayText"] = maxDaily[:1]
		case Dosage001, Dosage002, Dosage001ml:
			item["dosageDisplayText"] = maxDaily[1:]
		default:
			item["dosageDisplayText"] = maxDaily
		}
		daily = append(daily, item)
	}
	return daily, weekly
}

// IsOrderEligibleForLog ports the hardcoded allow-list in handler.js.
func IsOrderEligibleForLog(caseID string, now time.Time) bool {
	if caseID != OrderEligibleForLogCaseID {
		return false
	}
	until, err := common.ParseYMD(OrderEligibleForLogUntil)
	if err != nil {
		return false
	}
	return now.Before(until)
}

// GetBahLogForGivenDate ports handler.js getBahLogForGivenDate.
func (s *Service) GetBahLogForGivenDate(ctx context.Context, date *time.Time, userID string, showBahV3 bool, caseID string, appVersion int) (*BahLogForDate, error) {
	now := s.now()
	utcDate := now
	if date != nil && !date.IsZero() {
		utcDate = *date
	}
	clientDate := common.ISTShift(utcDate)
	todayClientDate := common.EndOfDayUTC(common.ISTShift(now))
	yesterdayClientDate := common.StartOfDayUTC(todayClientDate.AddDate(0, 0, -1))

	var (
		logDoc *models.ActivityLog
		latest *LatestMedicines
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		doc, err := s.Store.FindActivityLogInRange(gctx, userID, mongorepo.DayRange{
			From: common.StartOfDayUTC(clientDate), To: common.EndOfDayUTC(clientDate), FromInclusive: true, ToInclusive: true}, false)
		logDoc = doc
		return err
	})
	g.Go(func() error {
		m, err := s.GetLatestMedicines(gctx, userID, showBahV3)
		latest = m
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	out := &BahLogForDate{
		DailyDosageTitle:    "Daily Dosage",
		BahVideo:            BahVideo{Female: BahVideoFemale, Male: BahVideoMale},
		ArchivedProduct:     []map[string]any{},
		MedicineReordertext: map[string]any{},
	}
	eligibleOverride := IsOrderEligibleForLog(caseID, now)
	var medicineData []map[string]any
	hasLog := logDoc != nil && !logDoc.ID.IsZero()

	if !hasLog {
		for _, m := range latest.Medicines {
			medicineData = append(medicineData, map[string]any{
				"product_id": m.ProductID, "name": m.Name, "Dosage": m.Dosage, "dosageCode": m.DosageCode,
				"description": m.Description, "image_url": m.ImageURL,
				"morningCheckIns": false, "eveningCheckIns": false, "bothCheckInsRequired": false,
			})
		}
		locked := latest.IsMedicineLocked != nil && *latest.IsMedicineLocked
		out.OrderEligibleForLog = !locked || eligibleOverride
		if !eligibleOverride && latest.BahV3ReorderText != nil {
			out.MedicineReordertext = latest.BahV3ReorderText
		}
		switch {
		case todayClientDate.Equal(common.EndOfDayUTC(clientDate)):
			out.IsPostAPINeeded = true
		case yesterdayClientDate.Equal(common.StartOfDayUTC(clientDate)):
			next, err := s.Store.FindActivityLogInRange(ctx, userID, mongorepo.DayRange{
				From: common.StartOfDayUTC(common.ISTShift(now)), To: common.EndOfDayUTC(common.ISTShift(now)), FromInclusive: true, ToInclusive: true}, false)
			if err != nil {
				return nil, err
			}
			if next == nil || next.ID.IsZero() {
				out.IsPostAPINeeded = true
			}
		}
	} else {
		out.OrderEligibleForLog = true
		if !eligibleOverride && latest.BahV3ReorderText != nil {
			out.MedicineReordertext = latest.BahV3ReorderText
		}
		medicineData = logDoc.ProductPrescriptions
		switch {
		case todayClientDate.Equal(common.EndOfDayUTC(clientDate)):
			out.IsPutAPINeeded = true
		case yesterdayClientDate.Equal(common.StartOfDayUTC(clientDate)):
			next, err := s.Store.FindActivityLogInRange(ctx, userID, mongorepo.DayRange{
				From: common.StartOfDayUTC(common.ISTShift(now)), To: common.EndOfDayUTC(common.ISTShift(now)), FromInclusive: true, ToInclusive: true}, false)
			if err != nil {
				return nil, err
			}
			if next == nil || next.ID.IsZero() {
				out.IsPutAPINeeded = true
			}
		}
	}

	if appVersion >= VersionGateArchivedFilter {
		archived, err := s.Store.ArchivedProductIDs(ctx, userID)
		if err != nil {
			return nil, err
		}
		if len(archived) > 0 {
			kept := medicineData[:0:0]
			for _, m := range medicineData {
				if archived[asString(m, "product_id")] {
					out.ArchivedProduct = append(out.ArchivedProduct, m)
				} else {
					kept = append(kept, m)
				}
			}
			medicineData = kept
		}
	}

	out.DailyDosage, out.WeeklyDosage = SplitDosage(medicineData)
	if len(out.WeeklyDosage) > 0 {
		out.WeeklyDosageTitle = "Weekly Dosage"
	}

	expiring, err := s.GetEarliestExpiringUnusedCoins(ctx, userID)
	if err != nil {
		return nil, err
	}
	streak, err := s.Store.FindActiveStreak(ctx, userID)
	if err != nil {
		return nil, err
	}
	todayLog, err := s.Store.FindActivityLogInRange(ctx, userID, mongorepo.DayRange{
		From: common.StartOfDayUTC(common.ISTShift(now)), To: common.EndOfDayUTC(common.ISTShift(now)), FromInclusive: true, ToInclusive: true}, true)
	if err != nil {
		return nil, err
	}
	if expiring != nil && expiring.RemainingCoins > 0 {
		exp, _ := common.ParseYMD(expiring.ExpiringOn)
		out.CoinExpiryText = fmt.Sprintf("%d coin%s expiring on %s", expiring.RemainingCoins,
			map[bool]string{true: "", false: "s"}[expiring.RemainingCoins == 1], common.FormatMoment(exp, "DD MMM YYYY"))
	}
	currentStreak, hasLoggedEver := 0, false
	var lastLog *time.Time
	if streak != nil && !streak.ID.IsZero() {
		currentStreak = streak.StreakAchieveDays
		hasLoggedEver = !streak.FirstDateOfLog.IsZero()
		if !streak.LastDateOfLog.IsZero() {
			l := streak.LastDateOfLog
			lastLog = &l
		}
	}
	currentStreak = StreakBrokenRecompute(currentStreak, lastLog, now)
	isLoggedToday := todayLog != nil && !todayLog.ID.IsZero()
	isMissedYesterday := lastLog != nil && !isLoggedToday && common.CalendarDaysDifference(*lastLog, now) == 2
	out.BahChallengeEntryPointConfig = GetBahChallengeEntryPointBanner(currentStreak, hasLoggedEver, isLoggedToday, isMissedYesterday, s.Cfg.S3ImageBaseURL)
	return out, nil
}

// LogActivityInput is the POST /activityLogForBAH payload plus resolved identity.
type LogActivityInput struct {
	UserID               string
	IsLogForToday        bool
	ProductPrescriptions []map[string]any
	IsHabitTracker       bool
}

// LogActivityResult is the POST /activityLogForBAH response. scratchCard is present (possibly null)
// only on the success path, matching the JS object shape.
type LogActivityResult struct {
	Message         string `json:"message"`
	ScratchCard     any    `json:"scratchCard"`
	omitScratchCard bool
}

// MarshalJSON drops scratchCard on the "cannot log" path.
func (r LogActivityResult) MarshalJSON() ([]byte, error) {
	if r.omitScratchCard {
		return json.Marshal(map[string]any{"message": r.Message})
	}
	return json.Marshal(map[string]any{"message": r.Message, "scratchCard": r.ScratchCard})
}

// SaveActivityLogs ports handler.js saveActivityLogs. Returns (nil, nil) when a log already exists.
// Fix (spec §6.1.4): the past-date guard compares IST dates instead of day-of-month.
func (s *Service) SaveActivityLogs(ctx context.Context, userID string, checkInDate time.Time, pp []map[string]any) (*models.ActivityLog, error) {
	existing, err := s.Store.FindActivityLogInRange(ctx, userID, mongorepo.DayRange{
		From: common.UTCMidnight(checkInDate), To: common.UTCMidnight(checkInDate.AddDate(0, 0, 1))}, false)
	if err != nil {
		return nil, err
	}
	if existing != nil && !existing.ID.IsZero() {
		return nil, nil
	}
	clientNow := common.ISTShift(s.now())
	if common.ISTDateString(checkInDate) < common.ISTDateString(clientNow) {
		todayLog, err := s.Store.FindActivityLogInRange(ctx, userID, mongorepo.DayRange{
			From: common.UTCMidnight(clientNow), To: clientNow}, false)
		if err != nil {
			return nil, err
		}
		if todayLog != nil && !todayLog.ID.IsZero() {
			return nil, nil
		}
	}
	if len(pp) < 1 {
		return nil, common.BadRequest(MsgBlankPrescriptionArray)
	}
	doc, err := s.Store.CreateActivityLog(ctx, &models.ActivityLog{
		UserID: userID, CheckInsForDate: checkInDate, ProductPrescriptions: pp,
		IsValidForStreak: true, IsActive: true,
	})
	if err != nil {
		return nil, err
	}
	s.invalidateKitTrackerCalendar(ctx, userID)
	return doc, nil
}

// invalidateKitTrackerCalendar busts the kit-tracker calendar cache. It deletes both the
// tenant-prefixed key this service writes and the unprefixed key traya-app-backend still owns,
// otherwise app-backend would serve a stale calendar after a log. Failures are non-fatal.
func (s *Service) invalidateKitTrackerCalendar(ctx context.Context, userID string) {
	if s.Redis == nil {
		return
	}
	keys := common.SharedKeyCandidates(
		common.KitTrackerCalendarKey(s.TenantID, userID), common.LegacyKitTrackerCalendarKey(userID))
	if err := s.Redis.Del(ctx, keys...).Err(); err != nil {
		s.Log.Warn("kit-tracker calendar cache invalidation failed", "userId", userID, "error", err.Error())
	}
}

// SaveActivityLogsAndCreateStreakAndGiveRewards ports the POST /activityLogForBAH flow.
func (s *Service) SaveActivityLogsAndCreateStreakAndGiveRewards(ctx context.Context, in LogActivityInput) (*LogActivityResult, error) {
	pp := in.ProductPrescriptions
	archived, err := s.Store.ArchivedProductIDs(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if len(archived) > 0 {
		kept := make([]map[string]any, 0, len(pp))
		for _, p := range pp {
			if !archived[asString(p, "product_id")] {
				kept = append(kept, p)
			}
		}
		pp = kept
	}
	checkInDate := common.ISTShift(s.now())
	if !in.IsLogForToday {
		checkInDate = checkInDate.AddDate(0, 0, -1)
		todayLog, err := s.Store.FindActivityLogInRange(ctx, in.UserID, mongorepo.DayRange{
			From: common.UTCMidnight(checkInDate), To: common.UTCMidnight(common.ISTShift(s.now())), FromInclusive: true}, false)
		if err != nil {
			return nil, err
		}
		if todayLog != nil && !todayLog.ID.IsZero() {
			return &LogActivityResult{Message: fmt.Sprintf(MsgCannotLogForDate, common.JSDateString(checkInDate)), omitScratchCard: true}, nil
		}
	}
	logRef, err := s.SaveActivityLogs(ctx, in.UserID, checkInDate, pp)
	if err != nil {
		return nil, err
	}
	out := &LogActivityResult{Message: fmt.Sprintf(MsgCheckedInSuccess, common.JSDateString(checkInDate))}
	if logRef == nil {
		return out, nil
	}
	if in.IsHabitTracker {
		card, err := s.ProcessStreakAndRewards(ctx, in.UserID, checkInDate, logRef, true)
		if err != nil {
			return nil, err
		}
		out.ScratchCard = card
		return out, nil
	}
	userID, date, ref := in.UserID, checkInDate, logRef
	s.Dispatcher.Go(func(bg context.Context) {
		if _, err := s.ProcessStreakAndRewards(bg, userID, date, ref, false); err != nil {
			s.Log.Error("create_streak_and_update_task failed", "userId", userID, "error", err.Error())
		}
	})
	return out, nil
}

// ProcessStreakAndRewards ports handler.js processStreakAndRewards.
func (s *Service) ProcessStreakAndRewards(ctx context.Context, userID string, checkInDate time.Time, logRef *models.ActivityLog, isHabitTracker bool) (any, error) {
	userCase, err := s.PG.UserCaseByUserID(ctx, userID)
	if err != nil {
		s.Log.Warn("user/case lookup failed during log", "userId", userID, "error", err.Error())
	}
	caseID, phone, gender := "", "", "M"
	if userCase != nil {
		caseID, phone, gender = userCase.CaseID, userCase.PhoneNumber, GenderOf(userCase.Gender)
	}
	streakRes, err := s.CreateStreakLogForUser(ctx, userID, checkInDate, logRef.IsValidForStreak, caseID)
	if err != nil {
		return nil, err
	}
	if streakRes.AlreadyLogged {
		return nil, nil
	}
	kitStartedTask := TaskMaleKitStarted
	if gender == "F" {
		kitStartedTask = TaskFemaleKitStarted
	}
	var bonus *int
	if streakRes.IsStreakBreaked {
		bonus, err = s.ResolveStreakRestartBonusAmount(ctx, userID, caseID, gender)
		if err != nil {
			s.Log.Warn("streak restart bonus resolution failed", "userId", userID, "error", err.Error())
		}
	}
	streakDays := 0
	if streakRes.Streak != nil {
		streakDays = streakRes.Streak.StreakAchieveDays
	}

	var card any
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		res, err := s.CreditRewardCoinsToUser(gctx, userID, streakDays, phone, isHabitTracker, checkInDate, caseID)
		if err != nil {
			return err
		}
		if res != nil {
			card = res.Card
		}
		return nil
	})
	if bonus != nil {
		amount := *bonus
		g.Go(func() error { return s.CreditStreakRestartBonus(gctx, userID, phone, amount, checkInDate, caseID) })
	}
	for _, name := range []string{TaskBuildAHabbitSticky, TaskBuildAHabit, kitStartedTask} {
		taskName := name
		g.Go(func() error {
			if err := s.UpdateTaskForUserToDisplay(gctx, userID, caseID, taskName); err != nil {
				s.Log.Warn("task update failed", "userId", userID, "task", taskName, "error", err.Error())
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if logRef.IsValidForStreak && s.Lifeline != nil {
		if err := s.Lifeline.EnqueueForLog(ctx, userID, checkInDate); err != nil {
			s.Log.Warn("lifeline enqueue failed", "userId", userID, "error", err.Error())
		}
	}
	return card, nil
}

// MultipleLogInput is the PUT /multipleActivityLogForBAH payload.
type MultipleLogInput struct {
	UserID           string
	IsLogForToday    bool
	LogProductDetail []map[string]any
}

// UpdateMultipleMedicineLogForUser ports handler.js updateMultipleMedicineLogForUser.
// The forgiving semantics are deliberate: a missing log, an empty payload and a yesterday conflict
// all answer success.
func (s *Service) UpdateMultipleMedicineLogForUser(ctx context.Context, in MultipleLogInput) (map[string]string, error) {
	ok := map[string]string{"message": MsgMultipleLogSuccess}
	if len(in.LogProductDetail) == 0 {
		return ok, nil
	}
	clientNow := common.ISTShift(s.now())
	checkInDate := clientNow
	if !in.IsLogForToday {
		todayLog, err := s.Store.FindActivityLogInRange(ctx, in.UserID, mongorepo.DayRange{
			From: common.UTCMidnight(clientNow), To: clientNow}, false)
		if err != nil {
			return nil, err
		}
		if todayLog != nil && !todayLog.ID.IsZero() {
			return ok, nil
		}
		checkInDate = checkInDate.AddDate(0, 0, -1)
	}
	// bounds computed before the query (the JS mutated checkInDate in place)
	dayStart := common.UTCMidnight(checkInDate)
	nextDay := common.UTCMidnight(checkInDate.AddDate(0, 0, 1))
	logDoc, err := s.Store.FindActivityLogInRange(ctx, in.UserID, mongorepo.DayRange{From: dayStart, To: nextDay}, false)
	if err != nil {
		return nil, err
	}
	if logDoc == nil || logDoc.ID.IsZero() {
		return ok, nil
	}
	if len(logDoc.ProductPrescriptions) < 1 {
		return nil, common.BadRequest(MsgBlankPrescriptionExisting)
	}
	wanted := map[string][2]bool{}
	for _, d := range in.LogProductDetail {
		id := ""
		switch v := d["productId"].(type) {
		case string:
			id = v
		case float64:
			id = strconv.FormatInt(int64(v), 10)
		}
		if id != "" {
			wanted[id] = [2]bool{asBool(d, "morningCheckIns"), asBool(d, "eveningCheckIns")}
		}
	}
	updates := map[string][2]bool{}
	for _, p := range logDoc.ProductPrescriptions {
		pid := asString(p, "product_id")
		flags, requested := wanted[pid]
		if !requested {
			continue
		}
		if asBool(p, "morningCheckIns") && asBool(p, "eveningCheckIns") {
			continue
		}
		updates[pid] = flags
	}
	if len(updates) > 0 {
		if err := s.Store.BulkSetProductCheckIns(ctx, logDoc.ID, updates); err != nil {
			return nil, err
		}
	}
	return ok, nil
}
