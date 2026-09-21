package habit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/legacy"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

// Reward is resolveHabitTrackerReward's result.
type Reward struct {
	Type  string `json:"type"` // "ladder" | "daily"
	Slug  string `json:"slug"`
	Coins int    `json:"coins"`
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func ceilDiv(a, b int) int {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

// WeekForDay mirrors habitTrackerWeekForDay.
func WeekForDay(streakDay int) int { return clamp(ceilDiv(streakDay, 7), 1, len(DailyTiers)) }

// DailyCoins mirrors habitTrackerDailyCoins: 30/40/50/60/70 by streak week.
func DailyCoins(streakDay int) int {
	week := clamp(ceilDiv(streakDay, 7), 1, DailyMaxWeek)
	return DailyCoinsBase + (week-1)*DailyWeeklyStep
}

// ResolveReward mirrors resolveHabitTrackerReward; nil past the ladder (earning paused).
func ResolveReward(streakDay int) *Reward {
	if streakDay < 1 || streakDay > MaxRewardDay {
		return nil
	}
	for _, l := range Ladder {
		if l.Days == streakDay {
			return &Reward{Type: "ladder", Slug: l.Slug, Coins: l.Coins}
		}
	}
	week := WeekForDay(streakDay)
	for _, t := range DailyTiers {
		if t.Week == week {
			return &Reward{Type: "daily", Slug: t.Slug, Coins: t.Coins}
		}
	}
	return nil
}

// RemarksFor builds the idempotency key for a reward day.
func RemarksFor(r Reward, checkInDate time.Time) string {
	prefix := DailyRemarksPrefix
	if r.Type == "ladder" {
		prefix = LadderRemarksPrefix
	}
	return prefix + common.FormatMoment(checkInDate.UTC(), "YYYY-MM-DD")
}

// EnsureStreakMaster mirrors ensureHabitTrackerStreakMaster (upsert by slug, $setOnInsert).
func (s *Service) EnsureStreakMaster(ctx context.Context, r Reward) (*models.StreakMaster, error) {
	isLadder := r.Type == "ladder"
	days := 0
	display := "Habit Tracker Daily - " + strings.Replace(strings.TrimPrefix(r.Slug, "habit-daily-"), "-", " ", 1)
	if isLadder {
		if n, err := strconv.Atoi(strings.TrimPrefix(r.Slug, "habit-ladder-")); err == nil {
			days = n
		}
		display = fmt.Sprintf("Habit Tracker Ladder - Day %d", days)
	}
	return s.Store.UpsertStreakMasterBySlug(ctx, r.Slug, &models.StreakMaster{
		DisplayName: display, Days: days, IsActive: true, IsForSuperadmin: false, RewardCoins: r.Coins,
	})
}

// SaveRewardTransaction90 credits coins with the v85 90-day expiry and no Shopflo mirror.
func (s *Service) SaveRewardTransaction90(ctx context.Context, userID string, master *models.StreakMaster, reason string, coins int) (*models.RewardTransaction, bool, error) {
	future := common.UTCMidnight(s.now().AddDate(0, 0, 1+CoinExpiryDays))
	key := reason // habit-tracker credits are keyed by their remarks (daily/ladder + IST date)
	doc, err := s.Store.InsertRewardTransaction(ctx, &models.RewardTransaction{
		UserID: userID, StreakMasterID: master.ID, CreditCoins: coins,
		IsCreditTransaction: true, IsDebitTransaction: false, TotalDebitCoins: 0,
		CreditRemarks: reason, IdempotencyKey: &key, AllCoinsUsed: false, Status: "success", ExpireAt: &future,
	})
	if err != nil {
		if mongorepo.IsDuplicateKey(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return doc, false, nil
}

func boolPtr(b bool) *bool { return &b }
func intPtr(v int) *int    { return &v }

// CreditHabitTrackerCoins mirrors bahService.creditHabitTrackerCoins (the daily-tier path).
func (s *Service) CreditHabitTrackerCoins(ctx context.Context, in legacy.HabitCreditInput) (legacy.HabitCreditResult, error) {
	reward := ResolveReward(in.StreakDay)
	if reward == nil {
		return legacy.HabitCreditResult{Success: true, Credited: boolPtr(false), Paused: boolPtr(true)}, nil
	}
	remarks := RemarksFor(*reward, in.CheckInDate)
	exists, err := s.Store.ExistsCreditTransactionWithRemarks(ctx, in.UserID, remarks)
	if err != nil {
		return legacy.HabitCreditResult{}, err
	}
	if exists {
		return legacy.HabitCreditResult{Success: true, Credited: boolPtr(false), AlreadyCredited: boolPtr(true)}, nil
	}
	master, err := s.EnsureStreakMaster(ctx, *reward)
	if err != nil {
		return legacy.HabitCreditResult{}, err
	}
	_, dup, err := s.SaveRewardTransaction90(ctx, in.UserID, master, remarks, reward.Coins)
	if err != nil {
		return legacy.HabitCreditResult{}, err
	}
	if dup {
		return legacy.HabitCreditResult{Success: true, Credited: boolPtr(false), AlreadyCredited: boolPtr(true)}, nil
	}
	return legacy.HabitCreditResult{Success: true, Credited: boolPtr(true), Coins: intPtr(reward.Coins), Type: reward.Type}, nil
}

// MintOrCredit is the scratchCardController.mintHabitTrackerScratchCard branch:
// ladder days mint a scratch card when enabled, everything else credits immediately.
func (s *Service) MintOrCredit(ctx context.Context, in legacy.HabitCreditInput) (legacy.HabitCreditResult, error) {
	if in.UserID == "" {
		return legacy.HabitCreditResult{}, common.BadRequest(MsgInvalidUserID)
	}
	if in.StreakDay < 1 {
		return legacy.HabitCreditResult{}, common.BadRequest(MsgInvalidStreakDay)
	}
	if in.CheckInDate.IsZero() {
		return legacy.HabitCreditResult{}, common.BadRequest(MsgMissingCheckInDate)
	}
	var reward *Reward
	if s.scratchCardsEnabled() {
		reward = ResolveReward(in.StreakDay)
	}
	if reward != nil && reward.Type == "ladder" {
		return s.MintHabitTrackerScratchCard(ctx, in)
	}
	return s.CreditHabitTrackerCoins(ctx, in)
}
