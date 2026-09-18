package habit

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
)

// DayCell is one calendar strip cell.
type DayCell struct {
	Date          string  `json:"date"`
	DayLabel      string  `json:"dayLabel"`
	DayOfMonth    int     `json:"dayOfMonth"`
	IsToday       bool    `json:"isToday"`
	IsRewardDay   bool    `json:"isRewardDay"`
	Coins         int     `json:"coins"`
	Logged        bool    `json:"logged"`
	LifelineUsed  bool    `json:"lifelineUsed"`
	IsStreakBreak bool    `json:"isStreakBreak"`
	DateLabel     *string `json:"dateLabel,omitempty"`
}

// Stat is a coins/streak stat tile.
type Stat struct {
	Value    int    `json:"value"`
	Label    string `json:"label"`
	Subtitle string `json:"subtitle"`
	Action   string `json:"action"`
	Icon     string `json:"icon"`
}

// LifelineStat is the lifelines stat tile.
type LifelineStat struct {
	Total        int    `json:"total"`
	Used         int    `json:"used"`
	Label        string `json:"label"`
	Subtitle     string `json:"subtitle"`
	Action       string `json:"action"`
	Icon         string `json:"icon"`
	IconDisabled string `json:"iconDisabled"`
}

// Stats is the stats block.
type Stats struct {
	Coins     Stat         `json:"coins"`
	Streak    Stat         `json:"streak"`
	Lifelines LifelineStat `json:"lifelines"`
}

// Header is the log-and-earn header.
type Header struct {
	Overline string `json:"overline"`
	Heading  string `json:"heading"`
}

// CTA is the log-and-earn CTA.
type CTA struct {
	Label  string `json:"label"`
	Action string `json:"action"`
	Param  string `json:"param"`
}

// StripCalendar is the 15-cell day strip.
type StripCalendar struct {
	StartDate     string    `json:"startDate"`
	TodayDate     string    `json:"todayDate"`
	Days          []DayCell `json:"days"`
	EarningPaused bool      `json:"earningPaused"`
}

// LogAndEarn is getHabitTrackerData's payload.
type LogAndEarn struct {
	State                string            `json:"state,omitempty"`
	HabitTrackerDisabled *bool             `json:"habitTrackerDisabled,omitempty"`
	LoggingEnabled       bool              `json:"loggingEnabled"`
	Header               Header            `json:"header"`
	Benefits             []map[string]any  `json:"benefits,omitempty"`
	Kit                  map[string]any    `json:"kit,omitempty"`
	Milestones           []map[string]any  `json:"milestones,omitempty"`
	CTA                  CTA               `json:"cta"`
	Assets               map[string]string `json:"assets"`
	Calendar             *StripCalendar    `json:"calendar,omitempty"`
	Stats                *Stats            `json:"stats,omitempty"`
	BottomSheets         map[string]any    `json:"bottomSheets,omitempty"`
	KitLogCount          int               `json:"kitLogCount"`
	DaysToPause          *int              `json:"daysToPause"`
	CaseID               string            `json:"-"`
}

func dateKey(t time.Time) string { return common.FormatMoment(t.UTC(), "YYYY-MM-DD") }

// LiveRun mirrors habitTrackerLiveRun: consecutive valid days ending today (or yesterday).
func LiveRun(valid map[string]bool, today time.Time) int {
	cursor := today
	if !valid[dateKey(today)] {
		cursor = today.AddDate(0, 0, -1)
	}
	count := 0
	for valid[dateKey(cursor)] {
		count++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return count
}

// LastRunBeforeGap mirrors habitTrackerLastRunBeforeGap.
func LastRunBeforeGap(valid map[string]bool, today time.Time) int {
	cursor := today.AddDate(0, 0, -1)
	for i := 0; i < 400; i++ {
		if valid[dateKey(cursor)] {
			break
		}
		cursor = cursor.AddDate(0, 0, -1)
	}
	count := 0
	for valid[dateKey(cursor)] {
		count++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return count
}

// EarningPaused mirrors habitTrackerEarningPaused.
func EarningPaused(uniqueLogsInWindow int, windowOpen *time.Time, today time.Time, kitCount int) bool {
	if uniqueLogsInWindow >= PauseAfterLogs {
		return true
	}
	if windowOpen == nil {
		return false
	}
	if kitCount < 1 {
		kitCount = 1
	}
	windowDays := KitWindowDaysPerKit*kitCount + KitWindowBufferDays
	elapsed := int(common.UTCMidnight(today).Sub(common.UTCMidnight(*windowOpen)).Hours() / 24)
	return elapsed >= windowDays
}

// OrderFlags mirrors deriveHabitTrackerOrderFlags.
type OrderFlags struct {
	NewOrderPlacedNotDelivered bool
	NewKitDeliveredNotLogged   bool
}

// DeriveOrderFlags ports deriveHabitTrackerOrderFlags.
func DeriveOrderFlags(ords []orders.Order, logged map[string]bool, loggedToday, hasLoggedEver bool, today time.Time) OrderFlags {
	var flags OrderFlags
	last := orders.LastDelivered(ords)
	for _, o := range ords {
		if o.Status == "delivered" {
			continue
		}
		if last == nil || o.CreatedAt.After(orders.AnchorDate(*last)) {
			flags.NewOrderPlacedNotDelivered = true
			break
		}
	}
	if last != nil && hasLoggedEver && !loggedToday {
		since := common.UTCMidnight(orders.AnchorDate(*last))
		loggedSince := false
		for key := range logged {
			if d, err := common.ParseYMD(key); err == nil && !d.Before(since) {
				loggedSince = true
				break
			}
		}
		flags.NewKitDeliveredNotLogged = !loggedSince
	}
	return flags
}

// LogAndEarnInput is BuildLogAndEarn's argument set.
type LogAndEarnInput struct {
	Today         time.Time
	CurrentStreak int
	LoggedDates   map[string]bool
	ValidDates    map[string]bool
	BridgedDays   map[string]bool
	FirstLogDate  *time.Time
	CoinBalance   int
}

// BuildLogAndEarn ports buildHabitTrackerLogAndEarn (15-cell window, run map with bridged carry-over).
func BuildLogAndEarn(in LogAndEarnInput) *LogAndEarn {
	today := common.UTCMidnight(in.Today)
	start := today.AddDate(0, 0, -WindowDaysBack)
	total := WindowDaysBack + WindowDaysForward + 1

	runMap := map[string]int{}
	run := 0
	for d := start.AddDate(0, 0, -MaxRewardDay); !d.After(today); d = d.AddDate(0, 0, 1) {
		key := dateKey(d)
		if in.ValidDates[key] {
			run++
		} else if !in.BridgedDays[key] {
			run = 0
		}
		runMap[key] = run
	}
	liveRun := runMap[dateKey(today.AddDate(0, 0, -1))]
	if in.ValidDates[dateKey(today)] {
		liveRun = runMap[dateKey(today)]
	}
	effectiveDay := liveRun + 1
	if in.ValidDates[dateKey(today)] {
		effectiveDay = liveRun
	}

	days := make([]DayCell, 0, total)
	for i := 0; i < total; i++ {
		d := start.AddDate(0, 0, i)
		key := dateKey(d)
		diff := int(d.Sub(today).Hours() / 24)
		isPast, isFuture := diff < 0, diff > 0
		streakDayNumber := effectiveDay + diff
		if isPast {
			streakDayNumber = runMap[key]
		}
		rewardCoins, isRewardDay := RewardLadder[streakDayNumber]
		coins := DailyCoins(streakDayNumber)
		if isRewardDay {
			coins = rewardCoins
		}
		logged := !isFuture && in.LoggedDates[key]
		valid := !isFuture && in.ValidDates[key]
		lifelineUsed := isPast && in.BridgedDays[key]
		prevRun := runMap[dateKey(d.AddDate(0, 0, -1))]
		streakBreak := isPast && !valid && !lifelineUsed && prevRun > 0 &&
			(in.FirstLogDate == nil || !d.Before(common.UTCMidnight(*in.FirstLogDate)))
		days = append(days, DayCell{
			Date: key, DayLabel: DayLabels[int(d.Weekday())], DayOfMonth: d.Day(), IsToday: diff == 0,
			IsRewardDay: isRewardDay, Coins: coins, Logged: logged, LifelineUsed: lifelineUsed, IsStreakBreak: streakBreak,
		})
	}

	return &LogAndEarn{
		Header:   Header{Overline: HeaderOverline, Heading: HeaderHeading},
		CTA:      CTA{Label: CtaLabel, Action: CtaAction, Param: ""},
		Assets:   Assets,
		Calendar: &StripCalendar{StartDate: dateKey(start), TodayDate: dateKey(today), Days: days},
		Stats: &Stats{
			Coins:  Stat{Value: in.CoinBalance, Label: "Total Coins", Subtitle: Rupees(in.CoinBalance), Action: "coin_bottomsheet", Icon: Assets["coins"]},
			Streak: Stat{Value: in.CurrentStreak, Label: "Current Streak", Subtitle: "Keep going", Action: "streak_bottomsheet", Icon: Assets["streak"]},
			Lifelines: LifelineStat{Total: LifelinesTotal, Used: 0, Label: "Lifelines", Subtitle: "0/3 used",
				Action: "lifeline_bottomsheet", Icon: Assets["lifeline"], IconDisabled: Assets["lifelineDisabled"]},
		},
	}
}

// BuildIntro ports buildHabitTrackerIntro (state kit_arriving_intro).
func BuildIntro() *LogAndEarn {
	disabled := true
	milestones := make([]map[string]any, 0, 4)
	for _, day := range []int{1, 2, 3, 4} {
		m := map[string]any{"day": day, "coins": DailyCoinsBase, "icon": Assets["coins"], "locked": day != 1}
		if day == 1 {
			m["label"] = "1st Log"
		}
		milestones = append(milestones, m)
	}
	return &LogAndEarn{
		State: "kit_arriving_intro", HabitTrackerDisabled: &disabled, LoggingEnabled: false,
		Header: Header{Overline: HeaderOverline, Heading: IntroHeading},
		Benefits: []map[string]any{
			{"key": "coins", "icon": Assets["coins"], "label": "Earn Coins"},
			{"key": "streak", "icon": Assets["streak"], "label": "Start Streak"},
			{"key": "lifelines", "icon": Assets["lifeline"], "label": "Get Lifelines"},
		},
		Kit:        map[string]any{"image": Assets["kitBox"], "badge": map[string]any{"label": "Arriving"}},
		Milestones: milestones,
		CTA:        CTA{Label: IntroCtaLabel, Action: CtaAction, Param: ""},
		Assets:     Assets,
	}
}

// GetHabitTrackerData ports bahService.getHabitTrackerData. Returns nil (not an error) when the
// component cannot be built, so the caller hides it — matching the JS.
func (s *Service) GetHabitTrackerData(ctx context.Context, userID string, version int, nonVoid []orders.Order) *LogAndEarn {
	out, err := s.buildHabitTrackerData(ctx, userID, version, nonVoid)
	if err != nil {
		s.Log.Error("getHabitTrackerData failed", "userId", userID, "error", err.Error())
		return nil
	}
	return out
}

func (s *Service) buildHabitTrackerData(ctx context.Context, userID string, version int, nonVoid []orders.Order) (*LogAndEarn, error) {
	now := s.now()
	todayM := common.UTCMidnight(now)
	fetchStart := todayM.AddDate(0, 0, -(WindowDaysBack + MaxRewardDay))
	fetchEnd := common.EndOfDayUTC(now)

	var (
		balance       mongoBalanceView
		summary       *streakSummaryT
		logs          []activityDate
		lifelineDates []time.Time
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		b, err := s.Store.ActiveCoinBalance(gctx, userID, now)
		balance = mongoBalanceView{BalanceCoins: b.BalanceCoins, EarliestExpiry: b.EarliestExpiry}
		return err
	})
	g.Go(func() error {
		st, err := s.Store.FindActiveStreak(gctx, userID)
		if err != nil || st == nil || st.ID.IsZero() {
			return err
		}
		summary = &streakSummaryT{Days: st.StreakAchieveDays, First: st.FirstDateOfLog, Last: st.LastDateOfLog}
		return nil
	})
	g.Go(func() error {
		docs, err := s.Store.FindActivityLogsBetween(gctx, userID, fetchStart, fetchEnd, true, nil)
		for _, d := range docs {
			logs = append(logs, activityDate{Date: d.CheckInsForDate, Valid: d.IsValidForStreak})
		}
		return err
	})
	g.Go(func() error {
		d, err := s.Store.LifelineDates(gctx, userID)
		lifelineDates = d
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	coinBalance := int(roundHalfUp(balance.BalanceCoins))
	currentStreak := 0
	var lastLog, firstLog *time.Time
	if summary != nil {
		currentStreak = summary.Days
		if !summary.Last.IsZero() {
			l := summary.Last
			lastLog = &l
		}
		if !summary.First.IsZero() {
			f := summary.First
			firstLog = &f
		}
	}
	if version >= 71 && lastLog != nil && common.CalendarDaysDifference(*lastLog, todayM) > 2 {
		currentStreak = 0
	}

	// is_lifeline is not projected by the JS query, so every activity doc lands in loggedDates.
	loggedDates := map[string]bool{}
	validDates := map[string]bool{}
	for _, l := range logs {
		key := dateKey(l.Date)
		loggedDates[key] = true
		if l.Valid {
			validDates[key] = true
		}
	}
	loggedToday := lastLog != nil && sameUTCDay(*lastLog, now)
	hasLoggedEver := firstLog != nil

	kitStart := orders.CurrentRunningKitStartDate(nonVoid, now)
	if kitStart == nil && firstLog != nil {
		k := common.UTCMidnight(*firstLog)
		kitStart = &k
	} else if kitStart != nil {
		k := common.UTCMidnight(*kitStart)
		kitStart = &k
	}

	flags := DeriveOrderFlags(nonVoid, loggedDates, loggedToday, hasLoggedEver, todayM)
	hasDeliveredKit := orders.LastDelivered(nonVoid) != nil
	if flags.NewOrderPlacedNotDelivered && !hasLoggedEver && !hasDeliveredKit {
		return BuildIntro(), nil
	}

	validLogCount, err := s.Store.CountValidStreakLogs(ctx, userID, kitStart)
	if err != nil {
		return nil, err
	}

	bridged := map[string]bool{}
	for _, d := range lifelineDates {
		if kitStart != nil && common.UTCMidnight(d).Before(*kitStart) {
			continue
		}
		key := dateKey(d)
		bridged[key] = true
		validDates[key] = true
	}
	lifelinesUsed := len(bridged)
	liveRun := LiveRun(validDates, todayM)

	breakAnchor := firstLog
	for key := range mergeSets(loggedDates, validDates, bridged) {
		if d, err := common.ParseYMD(key); err == nil {
			if breakAnchor == nil || d.Before(*breakAnchor) {
				dd := d
				breakAnchor = &dd
			}
		}
	}

	out := BuildLogAndEarn(LogAndEarnInput{
		Today: todayM, CurrentStreak: currentStreak, LoggedDates: loggedDates, ValidDates: validDates,
		BridgedDays: bridged, FirstLogDate: breakAnchor, CoinBalance: coinBalance,
	})

	effectiveDay := liveRun + 1
	if loggedToday {
		effectiveDay = liveRun
	}
	_, rewardToday := RewardLadder[effectiveDay]
	_, dayBeforeReward := RewardLadder[effectiveDay+1]
	streakBroken := hasLoggedEver && liveRun == 0
	brokeOnRewardDay := false
	if streakBroken {
		_, brokeOnRewardDay = RewardLadder[LastRunBeforeGap(validDates, todayM)+1]
	}
	currentKitOrder := orders.LastDelivered(nonVoid)

	kitCount := 1
	runningKitStart := orders.CurrentRunningKitStartDate(nonVoid, now)
	var windowOpen *time.Time
	if runningKitStart != nil {
		d := common.UTCMidnight(*runningKitStart)
		windowOpen = &d
		first, err := s.Store.FirstRealActivityLogOnOrAfter(ctx, userID, d)
		if err != nil {
			return nil, err
		}
		if first != nil && !first.ID.IsZero() {
			w := common.UTCMidnight(first.CheckInsForDate)
			windowOpen = &w
		}
	}
	paused := EarningPaused(int(validLogCount), windowOpen, todayM, kitCount)
	var daysToPause *int
	if windowOpen != nil {
		v := 0
		if !paused {
			windowDaysTotal := KitWindowDaysPerKit*kitCount + KitWindowBufferDays
			elapsed := int(todayM.Sub(common.UTCMidnight(*windowOpen)).Hours() / 24)
			v = windowDaysTotal - elapsed
			if v < 0 {
				v = 0
			}
		}
		daysToPause = &v
	}

	resolved := ResolveState(StateInput{
		LoggedToday: loggedToday, HasLoggedEver: hasLoggedEver, RewardToday: rewardToday, DayBeforeReward: dayBeforeReward,
		StreakBroken: streakBroken, BrokeOnRewardDay: brokeOnRewardDay, KitArrivingIntro: false,
		NewOrderPlacedNotDelivered: flags.NewOrderPlacedNotDelivered, NewKitDeliveredNotLogged: flags.NewKitDeliveredNotLogged,
		EarningPaused: paused, EffectiveDay: effectiveDay, LifelinesUsed: lifelinesUsed, LifelinesTotal: LifelinesTotal,
		CoinBalance: coinBalance,
	})
	out.State = resolved.State
	out.Header.Heading = resolved.Heading
	out.CTA.Label = resolved.CtaLabel
	out.Stats.Coins.Subtitle = resolved.CoinsSubtitle
	out.Stats.Streak.Subtitle = resolved.StreakSubtitle
	out.Stats.Streak.Value = currentStreak
	if streakBroken {
		out.Stats.Streak.Value = 0
	}
	out.Stats.Lifelines.Subtitle = resolved.LifelinesSubtitle
	out.Stats.Lifelines.Used = lifelinesUsed
	out.BottomSheets = map[string]any{
		"coin_bottomsheet":     BuildCoinsBottomSheet(coinBalance, balance.EarliestExpiry, nonVoid, now),
		"streak_bottomsheet":   BuildStreakBottomSheet(out.Stats.Streak.Value),
		"lifeline_bottomsheet": BuildLifelineBottomSheet(lifelinesUsed, LifelinesTotal),
	}
	out.Calendar.EarningPaused = paused
	out.KitLogCount = int(validLogCount)
	out.LoggingEnabled = currentKitOrder != nil
	out.DaysToPause = daysToPause
	return out, nil
}

type mongoBalanceView struct {
	BalanceCoins   float64
	EarliestExpiry *time.Time
}

type streakSummaryT struct {
	Days        int
	First, Last time.Time
}

type activityDate struct {
	Date  time.Time
	Valid bool
}

func sameUTCDay(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func mergeSets(sets ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, s := range sets {
		for k := range s {
			out[k] = true
		}
	}
	return out
}

func roundHalfUp(f float64) float64 {
	if f < 0 {
		return -roundHalfUp(-f)
	}
	return float64(int64(f + 0.5))
}

var _ = fmt.Sprintf
