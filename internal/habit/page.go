package habit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
)

// BuildReorderBanner ports kitTrackerPageService.buildReorderBanner.
func BuildReorderBanner(daysSinceDelivery *int, kitCount int) map[string]any {
	if daysSinceDelivery == nil {
		return nil
	}
	kits := kitCount
	if kits < 1 {
		kits = 1
	}
	kitDays := ReorderBannerDaysPerKit * kits
	showFromDay := kitDays + ReorderBannerStartOff
	pauseDay := kitDays + ReorderBannerPauseOff
	days := *daysSinceDelivery
	if days < showFromDay {
		return nil
	}
	daysToPause := pauseDay - days
	if daysToPause < 0 {
		daysToPause = 0
	}
	return map[string]any{
		"text":              fmt.Sprintf("%d %s since the last order was placed.", days, plural(days, "day", "days")),
		"daysToPause":       daysToPause,
		"daysSinceDelivery": days,
		"kitCount":          kits,
		"paused":            days >= pauseDay,
		"cta":               map[string]any{"label": ReorderCtaLabel, "action": "reorder"},
	}
}

// DailyStripDays ports dailyStripDays: at most 7 cells ending on the next reward day.
func DailyStripDays(days []DayCell, todayDate string) []DayCell {
	todayIdx := 0
	for i, d := range days {
		if d.Date == todayDate {
			todayIdx = i
			break
		}
	}
	endIdx := todayIdx + 6
	if endIdx > len(days)-1 {
		endIdx = len(days) - 1
	}
	for i := todayIdx; i < len(days); i++ {
		if days[i].IsRewardDay {
			endIdx = i
			break
		}
	}
	start := endIdx - 6
	if start < 0 {
		start = 0
	}
	if endIdx+1 > len(days) {
		return days[start:]
	}
	return days[start : endIdx+1]
}

// DailyLogDateLabel ports dailyLogDateLabel ("2 Apr" / "2 Apr, Today").
func DailyLogDateLabel(cell DayCell) *string {
	d, err := common.ParseYMD(cell.Date)
	if err != nil {
		return nil
	}
	label := common.FormatMoment(d, "D MMM")
	if cell.IsToday {
		label += ", Today"
	}
	return &label
}

// GetRewardsCount ports getRewardsCount: active cards plus cards claimed today in IST.
func (s *Service) GetRewardsCount(ctx context.Context, userID string) int {
	cards, err := s.GetScratchCards(ctx, userID)
	if err != nil {
		s.Log.Error("getRewardsCount failed", "userId", userID, "error", err.Error())
		return 0
	}
	count := len(cards.Active)
	todayIST := common.ISTDateString(s.now())
	for _, h := range cards.History {
		if claimed, ok := h["claimed_at"].(*time.Time); ok && claimed != nil && common.ISTDateString(*claimed) == todayIST {
			count++
		}
	}
	return count
}

// BuildRewardScreen ports buildRewardScreen.
func BuildRewardScreen(core *LogAndEarn, todayCell *DayCell, daysToReward *int, weekDays []map[string]any,
	activeCard *ActiveCardRef, scratchEnabled bool, feedbackCard any) map[string]any {
	rawStreak := 0
	if core != nil && core.Stats != nil {
		rawStreak = core.Stats.Streak.Value
	}
	loggedToday := todayCell != nil && (todayCell.Logged || todayCell.LifelineUsed)
	projected := rawStreak
	if !loggedToday {
		projected = rawStreak + 1
	}
	if todayCell != nil && todayCell.IsRewardDay {
		scratchCard := map[string]any{
			"coverAsset":  scratchCoverAsset(),
			"revealAsset": scratchRevealAsset(),
			"reward":      map[string]any{"type": "coins", "value": todayCell.Coins, "displayText": CoinsDisplayText},
		}
		if scratchEnabled && activeCard != nil {
			scratchCard["id"] = activeCard.ID
			scratchCard["status"] = activeCard.Status
		}
		if !scratchEnabled {
			scratchCard["autoCreditOnComplete"] = false
		}
		rewardClaimed := true
		if scratchEnabled {
			rewardClaimed = activeCard == nil
		}
		title := fmt.Sprintf("%d-day streak reward unlocked!", projected)
		subtitle := "Scratch to reveal your bonus coins"
		if rewardClaimed {
			title = fmt.Sprintf("%d-day streak reward claimed", projected)
			subtitle = "Rewards already credited"
		}
		return map[string]any{"mode": "reward", "streak": projected, "title": title, "subtitle": subtitle,
			"loggedToday": loggedToday, "rewardClaimed": rewardClaimed, "scratchCard": scratchCard, "feedbackCard": feedbackCard}
	}
	var coinsEarned any
	var coinsEarnedText any
	if todayCell != nil {
		coinsEarned = todayCell.Coins
		coinsEarnedText = fmt.Sprintf("%d Coins Earned", todayCell.Coins)
	}
	var subCopy any
	if daysToReward != nil {
		subCopy = fmt.Sprintf("Log %d more %s to scratch mystery reward", *daysToReward, plural(*daysToReward, "day", "days"))
	}
	if weekDays == nil {
		weekDays = []map[string]any{}
	}
	return map[string]any{"mode": "normal", "streak": projected, "title": fmt.Sprintf("%d-Day Streak!", projected),
		"loggedToday": loggedToday, "coinsEarned": coinsEarned, "coinsEarnedText": coinsEarnedText,
		"subCopy": subCopy, "animationAsset": Assets["coins"], "days": weekDays, "feedbackCard": feedbackCard}
}

// GetKitTrackerPage ports kitTrackerPageService.getKitTrackerPage.
func (s *Service) GetKitTrackerPage(ctx context.Context, userID, caseID string, version int) (map[string]any, error) {
	now := s.now()
	nonVoid, err := s.PG.NonVoidOrdersByCase(ctx, caseID)
	if err != nil {
		return nil, err
	}

	var (
		core         *LogAndEarn
		badges       *BadgesResponse
		rewardsCount int
		feedbackCard any
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { core = s.GetHabitTrackerData(gctx, userID, version, nonVoid); return nil })
	g.Go(func() error {
		b, err := s.GetHabitTrackerBadges(gctx, userID, caseID, nonVoid, "")
		if err != nil {
			s.Log.Warn("badges unavailable for kit tracker page", "userId", userID, "error", err.Error())
			return nil
		}
		badges = b
		return nil
	})
	g.Go(func() error { rewardsCount = s.GetRewardsCount(gctx, userID); return nil })
	g.Go(func() error {
		feedbackCard = s.GetFeedbackCard(gctx, userID, caseID, version, len(nonVoid))
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if core == nil {
		return nil, common.Internal(MsgKitTrackerDataUnavail)
	}
	core.CaseID = caseID
	badgesCount := 0
	if badges != nil {
		badgesCount = badges.EarnedCount
	}
	return s.assembleKitTrackerPage(core, nonVoid, badgesCount, userID, rewardsCount, feedbackCard, now), nil
}

func (s *Service) assembleKitTrackerPage(core *LogAndEarn, ords []orders.Order, badgesCount int,
	userID string, rewardsCount int, feedbackCard any, now time.Time) map[string]any {
	var days []DayCell
	todayDate := ""
	if core.Calendar != nil {
		days = core.Calendar.Days
		todayDate = core.Calendar.TodayDate
	}
	var todayCell *DayCell
	todayIdx := -1
	for i := range days {
		if days[i].IsToday {
			todayCell = &days[i]
			todayIdx = i
			break
		}
	}
	var daysToReward *int
	if todayIdx >= 0 {
		for i := todayIdx; i < len(days); i++ {
			if days[i].IsRewardDay {
				v := i - todayIdx
				daysToReward = &v
				break
			}
		}
	}
	loggedToday := todayCell != nil && (todayCell.Logged || todayCell.LifelineUsed)

	weekDays := []map[string]any{}
	for _, c := range DailyStripDays(days, todayDate) {
		cell := c
		row := map[string]any{
			"date": cell.Date, "dayLabel": cell.DayLabel, "dayOfMonth": cell.DayOfMonth, "isToday": cell.IsToday,
			"isRewardDay": cell.IsRewardDay, "coins": cell.Coins, "logged": cell.Logged,
			"lifelineUsed": cell.LifelineUsed, "isStreakBreak": cell.IsStreakBreak, "dateLabel": DailyLogDateLabel(cell),
		}
		weekDays = append(weekDays, row)
	}

	var activeCard *ActiveCardRef
	if s.scratchCardsEnabled() && todayCell != nil && todayCell.IsRewardDay && userID != "" {
		activeCard = s.GetActiveCardForReveal(context.Background(), userID, todayCell.Date)
	}

	details := orders.GetAllOrderDetails(ords, now)
	var banner any
	switch {
	case core.State == "kit_arriving_intro":
		banner = map[string]any{"text": "Start logging once your kit arrives"}
	case details.MinDaysAfterOrderDelivered >= 0 && !details.IsOrderPlaced:
		kitCount := 1
		if details.KitExpireDays > 0 {
			kitCount = int(roundHalfUp(float64(details.KitExpireDays) / 30))
		}
		d := details.MinDaysAfterOrderDelivered
		banner = BuildReorderBanner(&d, kitCount)
	}

	var footer any
	if daysToReward != nil {
		switch {
		case *daysToReward == 0:
			switch {
			case !loggedToday:
				footer = "Log today to unlock reward"
			case s.scratchCardsEnabled() && activeCard != nil:
				footer = "Scratch your reward to claim it"
			default:
				footer = "Reward claimed"
			}
		default:
			n := *daysToReward
			if !loggedToday {
				n++
			}
			footer = fmt.Sprintf("Log %d more %s to unlock reward", n, plural(n, "day", "days"))
		}
	}

	var todayCoins, coinsLabel, dateLabel any
	if todayCell != nil {
		todayCoins = fmt.Sprintf("₹%d", todayCell.Coins)
		coinsLabel = fmt.Sprintf("Earn %d coins", todayCell.Coins)
		dateLabel = DailyLogDateLabel(*todayCell)
	}

	formBase := strings.TrimRight(s.Cfg.FormBaseURL, "/")
	kitNumber := details.RunningMonthForHairKit
	page := map[string]any{
		"state":                core.State,
		"habitTrackerDisabled": core.State == "kit_arriving_intro",
		"header": map[string]any{
			"title":    "Kit Tracker",
			"badges":   map[string]any{"count": badgesCount, "icon": "Award"},
			"rewards":  map[string]any{"count": rewardsCount, "icon": Assets["rewardUpcoming"]},
			"reminder": map[string]any{"enabled": true, "icon": "BellRing", "iconOff": "BellOff"},
		},
		"kitGoal": map[string]any{
			"logged": core.KitLogCount, "goal": KitGoalLogs, "kitNumber": kitNumber,
			"title": KitGoalTitle, "topLevelText": fmt.Sprintf("Kit %d Goal", kitNumber),
			"navigation": map[string]any{"action": "web_page",
				"url": fmt.Sprintf("%s/pages/care-plan/%s?source=app", formBase, core.CaseID)},
		},
		"stats":  core.Stats,
		"assets": core.Assets,
		"banner": banner,
		"dailyLog": map[string]any{
			"todayCoins": todayCoins, "dateLabel": dateLabel, "coinsLabel": coinsLabel,
			"days": weekDays, "footer": footer,
		},
		"rewardScreen":   BuildRewardScreen(core, todayCell, daysToReward, weekDays, activeCard, s.scratchCardsEnabled(), feedbackCard),
		"bottomSheets":   core.BottomSheets,
		"kitLogCount":    core.KitLogCount,
		"loggingEnabled": core.LoggingEnabled,
		"earningPaused":  core.Calendar != nil && core.Calendar.EarningPaused,
	}
	if page["assets"] == nil {
		page["assets"] = map[string]string{}
	}
	if page["bottomSheets"] == nil {
		page["bottomSheets"] = map[string]any{}
	}
	return page
}
