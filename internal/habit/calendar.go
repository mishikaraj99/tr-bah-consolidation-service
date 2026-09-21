package habit

import (
	"context"
	"encoding/json"
	"time"

	"traya-bah-service/internal/common"
)

// MonthDay is one month-calendar cell.
type MonthDay struct {
	Date         string `json:"date"`
	DayOfMonth   int    `json:"dayOfMonth"`
	Weekday      int    `json:"weekday"`
	IsToday      bool   `json:"isToday"`
	State        string `json:"state"`
	IsRewardDay  bool   `json:"isRewardDay"`
	RewardEarned bool   `json:"rewardEarned"`
	Coins        *int   `json:"coins"`
	InStreakRun  bool   `json:"inStreakRun"`
	StreakBreak  bool   `json:"streakBreak"`
	LifeUsed     bool   `json:"lifeUsed"`
}

// MonthSummary is the per-month tally.
type MonthSummary struct {
	DaysLogged    int `json:"daysLogged"`
	DaysMissed    int `json:"daysMissed"`
	CoinsMissed   int `json:"coinsMissed"`
	RewardsMissed int `json:"rewardsMissed"`
	LifelinesUsed int `json:"lifelinesUsed"`
}

// MonthCalendar is one month block.
type MonthCalendar struct {
	Month        int                 `json:"month"`
	Year         int                 `json:"year"`
	MonthLabel   string              `json:"monthLabel"`
	FirstWeekday int                 `json:"firstWeekday"`
	CanGoPrev    bool                `json:"canGoPrev"`
	CanGoNext    bool                `json:"canGoNext"`
	Days         []MonthDay          `json:"days"`
	Legend       []map[string]string `json:"legend"`
	Summary      MonthSummary        `json:"summary"`
	Assets       map[string]string   `json:"assets"`
}

// CalendarResponse is GET /config/cms/kit-tracker-calendar.
type CalendarResponse struct {
	FirstLogDate *string             `json:"firstLogDate"`
	CurrentMonth map[string]int      `json:"currentMonth"`
	Legend       []map[string]string `json:"legend"`
	Assets       map[string]string   `json:"assets"`
	Months       []MonthCalendar     `json:"months"`
}

func calendarLegend() []map[string]string {
	return []map[string]string{{"key": "logged"}, {"key": "lifeline"}, {"key": "missed"}, {"key": "reward"}}
}

// RunState is computeHabitTrackerRunState's result.
type RunState struct {
	RunMap       map[string]int
	BandDays     map[string]bool
	LiveRun      int
	EffectiveDay int
}

// ComputeRunState ports computeHabitTrackerRunState (no bridged carry-over here).
func ComputeRunState(today, rangeStart time.Time, activeStart *time.Time, valid map[string]bool) RunState {
	rs := RunState{RunMap: map[string]int{}, BandDays: map[string]bool{}}
	run := 0
	for d := common.UTCMidnight(rangeStart).AddDate(0, 0, -MaxRewardDay); !d.After(common.UTCMidnight(today)); d = d.AddDate(0, 0, 1) {
		key := dateKey(d)
		if valid[key] {
			run++
		} else {
			run = 0
		}
		rs.RunMap[key] = run
	}
	lastValid := common.UTCMidnight(today)
	for i := 0; i < 400; i++ {
		if valid[dateKey(lastValid)] {
			break
		}
		if activeStart != nil && lastValid.Before(common.UTCMidnight(*activeStart)) {
			break
		}
		lastValid = lastValid.AddDate(0, 0, -1)
	}
	cursor := lastValid
	for valid[dateKey(cursor)] {
		rs.BandDays[dateKey(cursor)] = true
		cursor = cursor.AddDate(0, 0, -1)
	}
	rs.LiveRun = rs.RunMap[dateKey(lastValid)]
	rs.EffectiveDay = rs.LiveRun + 1
	if valid[dateKey(common.UTCMidnight(today))] {
		rs.EffectiveDay = rs.LiveRun
	}
	return rs
}

// MonthCalendarInput is BuildMonthCalendar's argument set.
type MonthCalendarInput struct {
	Month, Year   int
	Today         time.Time
	FirstLogDate  *time.Time
	ActiveStart   *time.Time
	LoggedDates   map[string]bool
	ValidDates    map[string]bool
	LifelineDates map[string]bool
	RunState      RunState
	RangeStart    time.Time
}

// BuildMonthCalendar ports buildHabitTrackerMonthCalendar.
func BuildMonthCalendar(in MonthCalendarInput) MonthCalendar {
	monthStart := time.Date(in.Year, time.Month(in.Month), 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := monthStart.AddDate(0, 1, 0).AddDate(0, 0, -1).Day()
	today := common.UTCMidnight(in.Today)
	todayKey := dateKey(today)

	out := MonthCalendar{
		Month: in.Month, Year: in.Year, MonthLabel: common.FormatMoment(monthStart, "MMMM YYYY"),
		FirstWeekday: int(monthStart.Weekday()), Legend: calendarLegend(), Assets: Assets,
		Days: make([]MonthDay, 0, daysInMonth),
	}
	out.CanGoPrev = in.FirstLogDate != nil && monthStart.After(common.UTCMidnight(*in.FirstLogDate))
	out.CanGoNext = monthStart.AddDate(0, 1, 0).Before(today)

	for dom := 1; dom <= daysInMonth; dom++ {
		d := time.Date(in.Year, time.Month(in.Month), dom, 0, 0, 0, 0, time.UTC)
		key := dateKey(d)
		isFuture := d.After(today)
		isPast := d.Before(today)

		state := "none"
		switch {
		case isFuture:
			state = "future"
		case in.LoggedDates[key]:
			state = "logged"
		case in.LifelineDates[key]:
			state = "lifeline"
		case in.ActiveStart != nil && !d.Before(common.UTCMidnight(*in.ActiveStart)) && d.Before(today):
			state = "missed"
		}
		lifeUsed := !isFuture && in.LifelineDates[key]
		dayRun := in.RunState.RunMap[key]
		if !isPast {
			dayRun = in.RunState.EffectiveDay + int(d.Sub(today).Hours()/24)
		}
		rewardCoins, isRewardDay := RewardLadder[dayRun]
		rewardEarned := isRewardDay && !isFuture && in.ValidDates[key]
		showsCoins := state == "logged" || state == "lifeline" || (isRewardDay && !isFuture)
		var coins *int
		if showsCoins {
			v := DailyCoins(dayRun)
			if isRewardDay {
				v = rewardCoins
			}
			coins = &v
		}
		prevRun := in.RunState.RunMap[dateKey(d.AddDate(0, 0, -1))]
		streakBreak := state == "missed" && prevRun > 0

		if state == "logged" {
			out.Summary.DaysLogged++
		}
		if lifeUsed {
			out.Summary.LifelinesUsed++
		}
		if state == "missed" {
			out.Summary.DaysMissed++
			out.Summary.CoinsMissed += DailyCoins(prevRun + 1)
			if _, ok := RewardLadder[prevRun+1]; ok {
				out.Summary.RewardsMissed++
			}
		}
		out.Days = append(out.Days, MonthDay{
			Date: key, DayOfMonth: dom, Weekday: int(d.Weekday()), IsToday: key == todayKey, State: state,
			IsRewardDay: isRewardDay, RewardEarned: rewardEarned, Coins: coins,
			InStreakRun: in.RunState.BandDays[key], StreakBreak: streakBreak, LifeUsed: lifeUsed,
		})
	}
	return out
}

// GetHabitTrackerCalendar ports bahService.getHabitTrackerCalendar (Redis-cached in production).
func (s *Service) GetHabitTrackerCalendar(ctx context.Context, userID string) (*CalendarResponse, error) {
	if cached := s.readCalendarCache(ctx, userID); cached != nil {
		return cached, nil
	}
	now := s.now()
	today := common.UTCMidnight(now)

	logs, err := s.Store.FindAllActivityLogs(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	lifelines, err := s.Store.LifelineDates(ctx, userID)
	if err != nil {
		return nil, err
	}
	streak, err := s.Store.FindActiveStreak(ctx, userID)
	if err != nil {
		return nil, err
	}

	loggedDates, validDates, lifelineDates := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var earliest *time.Time
	for _, l := range logs {
		key := dateKey(l.CheckInsForDate)
		if l.Lifeline() {
			lifelineDates[key] = true
		} else {
			loggedDates[key] = true
		}
		if l.IsValidForStreak {
			validDates[key] = true
		}
		d := common.UTCMidnight(l.CheckInsForDate)
		if earliest == nil || d.Before(*earliest) {
			earliest = &d
		}
	}
	for _, d := range lifelines {
		key := dateKey(d)
		lifelineDates[key] = true
		validDates[key] = true
	}

	var firstLogDate *time.Time
	if streak != nil && !streak.ID.IsZero() && !streak.FirstDateOfLog.IsZero() {
		f := common.UTCMidnight(streak.FirstDateOfLog)
		firstLogDate = &f
	}
	rangeStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	anchor := firstLogDate
	if earliest != nil && (anchor == nil || earliest.Before(*anchor)) {
		anchor = earliest
	}
	if anchor != nil {
		rangeStart = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	runState := ComputeRunState(today, rangeStart, anchor, validDates)
	out := &CalendarResponse{
		CurrentMonth: map[string]int{"month": int(today.Month()), "year": today.Year()},
		Legend:       calendarLegend(), Assets: Assets,
	}
	if firstLogDate != nil {
		k := dateKey(*firstLogDate)
		out.FirstLogDate = &k
	}
	for m, i := rangeStart, 0; !m.After(today) && i < 60; m, i = m.AddDate(0, 1, 0), i+1 {
		out.Months = append(out.Months, BuildMonthCalendar(MonthCalendarInput{
			Month: int(m.Month()), Year: m.Year(), Today: today, FirstLogDate: firstLogDate, ActiveStart: anchor,
			LoggedDates: loggedDates, ValidDates: validDates, LifelineDates: lifelineDates,
			RunState: runState, RangeStart: rangeStart,
		}))
	}
	s.writeCalendarCache(ctx, userID, out)
	return out, nil
}

func (s *Service) cacheEnabled() bool { return s.Redis != nil && s.Cfg != nil && s.Cfg.IsProduction }

func (s *Service) readCalendarCache(ctx context.Context, userID string) *CalendarResponse {
	if !s.cacheEnabled() {
		return nil
	}
	for _, key := range common.SharedKeyCandidates(CalendarCacheKey(s.TenantID, userID), LegacyCalendarCacheKey(userID)) {
		raw, err := s.Redis.Get(ctx, key).Bytes()
		if err != nil || len(raw) == 0 {
			continue
		}
		var out CalendarResponse
		if json.Unmarshal(raw, &out) != nil {
			continue
		}
		return &out
	}
	return nil
}

func (s *Service) writeCalendarCache(ctx context.Context, userID string, v *CalendarResponse) {
	if !s.cacheEnabled() {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := s.Redis.Set(ctx, CalendarCacheKey(s.TenantID, userID), raw, CalendarCacheTTLSeconds*time.Second).Err(); err != nil {
		s.Log.Warn("kit-tracker calendar cache write failed", "userId", userID, "error", err.Error())
	}
}

// InvalidateCalendarCache removes the cached calendar for a user. It deletes the legacy key too,
// otherwise traya-app-backend would keep serving a stale calendar after a log.
func (s *Service) InvalidateCalendarCache(ctx context.Context, userID string) {
	if s.Redis == nil {
		return
	}
	keys := common.SharedKeyCandidates(CalendarCacheKey(s.TenantID, userID), LegacyCalendarCacheKey(userID))
	if err := s.Redis.Del(ctx, keys...).Err(); err != nil {
		s.Log.Warn("kit-tracker calendar cache invalidation failed", "userId", userID, "error", err.Error())
	}
}
