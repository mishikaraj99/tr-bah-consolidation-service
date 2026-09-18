package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

type primitiveID = primitive.ObjectID

// SaveRewardInput is saveRewardTransaction's argument set.
type SaveRewardInput struct {
	UserID             string
	Master             *models.StreakMaster
	PhoneNumber        string
	Reason             string
	CustomAmount       *int
	CaseID             string
	ExpiryDaysOverride *int
}

// SaveRewardResult reports the inserted transaction, or that an idempotent duplicate was rejected.
type SaveRewardResult struct {
	Ref       *models.RewardTransaction
	Duplicate bool
}

// CoinExpiryDays calls order-service coin/expiry/month/<userId>; the response is a number of days.
func (s *Service) CoinExpiryDays(ctx context.Context, userID string) (int, error) {
	base := s.Cfg.OrderServiceBaseURL
	if base == "" {
		return 0, common.Internal("ORDER_SERVICE_BASE_URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/coin/expiry/month/"+userID, nil)
	if err != nil {
		return 0, err
	}
	client := s.httpClient(s.HTTP.OrderService)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, common.Internal(fmt.Sprintf("order-service coin expiry returned %d", resp.StatusCode))
	}
	var n float64
	if err := json.Unmarshal(body, &n); err == nil {
		return int(n), nil
	}
	var wrapper struct {
		Data float64 `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return 0, err
	}
	return int(wrapper.Data), nil
}

// SaveRewardTransaction ports handler.js saveRewardTransaction (expiry, CCD, Shopflo mirror),
// adding idempotency: a duplicate key on credit_remarks means the credit already happened.
func (s *Service) SaveRewardTransaction(ctx context.Context, in SaveRewardInput) (SaveRewardResult, error) {
	if in.Master == nil {
		return SaveRewardResult{}, common.BadRequest(MsgInvalidStreakMaster)
	}
	days := 0
	switch {
	case in.ExpiryDaysOverride != nil:
		days = *in.ExpiryDaysOverride
	case in.Master.Slug == Coins800StreakRewardsID:
		days = Coins800ExpiryDays
	default:
		d, err := s.CoinExpiryDays(ctx, in.UserID)
		if err != nil {
			return SaveRewardResult{}, err
		}
		days = d
	}
	// expire_at = UTC midnight of (IST now + 1 day + expiry days)
	future := common.UTCMidnight(common.ISTShift(s.now()).AddDate(0, 0, 1+days))

	coins := in.Master.RewardCoins
	if in.CustomAmount != nil {
		coins = *in.CustomAmount
	}
	remarks := in.Reason
	if remarks == "" {
		remarks = in.Master.DisplayName
	}
	doc := &models.RewardTransaction{
		UserID: in.UserID, StreakMasterID: in.Master.ID, CreditCoins: coins,
		IsCreditTransaction: true, IsDebitTransaction: false, TotalDebitCoins: 0,
		CreditRemarks: remarks, AllCoinsUsed: false, Status: "success", ExpireAt: &future,
	}
	ref, err := s.Store.InsertRewardTransaction(ctx, doc)
	if err != nil {
		if mongorepo.IsDuplicateKey(err) {
			s.Log.Info("credit already applied, skipping duplicate", "userId", in.UserID, "remarks", remarks)
			return SaveRewardResult{Duplicate: true}, nil
		}
		return SaveRewardResult{}, err
	}
	if s.CCD != nil && in.CaseID != "" {
		s.CCD.Publish(ctx, s.TenantID, CCDEventCoinCredited, in.CaseID, map[string]any{"amount": ref.CreditCoins})
	}
	if in.PhoneNumber != "" && s.Shopflo != nil {
		if err := s.Shopflo.EnsureWallet(ctx, in.PhoneNumber); err != nil {
			s.Log.Warn("shopflo ensure wallet failed", "phone", common.MaskPhone(in.PhoneNumber), "error", err.Error())
		}
		if err := s.Shopflo.Credit(ctx, ref.CreditCoins, future.UnixMilli(), ref.ID.Hex(), in.PhoneNumber); err != nil {
			s.Log.Warn("shopflo credit failed", "phone", common.MaskPhone(in.PhoneNumber), "error", err.Error())
		}
	}
	return SaveRewardResult{Ref: ref}, nil
}

// CreditRewardCoinsToUser ports handler.js creditRewardCoinsToUser.
// isHabitTracker routes to the v85 economy; the legacy path credits the first-log bonus and 3/7/21 milestones
// with idempotent credit_remarks.
func (s *Service) CreditRewardCoinsToUser(ctx context.Context, userID string, continuousDays int, phone string, isHabitTracker bool, checkInDate time.Time, caseID string) (*HabitCreditResult, error) {
	if isHabitTracker {
		if s.Habit == nil {
			return &HabitCreditResult{Success: false, Message: "Failed to credit habit tracker coins"}, nil
		}
		res, err := s.Habit.MintOrCredit(ctx, HabitCreditInput{UserID: userID, StreakDay: continuousDays, PhoneNumber: phone, CheckInDate: checkInDate})
		if err != nil {
			s.Log.Error("habit tracker credit failed", "userId", userID, "streakDay", continuousDays, "error", err.Error())
			return &HabitCreditResult{Success: false, Message: "Failed to credit habit tracker coins"}, nil
		}
		return &res, nil
	}

	firstMaster, err := s.Store.FindStreakMasterBySlug(ctx, SlugFirstCheckinExtra, true)
	if err != nil {
		return nil, err
	}
	existingMaster, err := s.Store.FindStreakMasterBySlug(ctx, SlugExistingCoinsStreak, true)
	if err != nil {
		return nil, err
	}
	if firstMaster != nil && !firstMaster.ID.IsZero() {
		ids := []primitiveID{firstMaster.ID}
		if existingMaster != nil && !existingMaster.ID.IsZero() {
			ids = append(ids, existingMaster.ID)
		}
		already, err := s.Store.ExistsRewardTxnForMasters(ctx, userID, ids)
		if err != nil {
			return nil, err
		}
		if !already {
			if _, err := s.SaveRewardTransaction(ctx, SaveRewardInput{
				UserID: userID, Master: firstMaster, PhoneNumber: phone, Reason: RemarkFirstLog, CaseID: caseID,
			}); err != nil {
				return nil, err
			}
		}
	}
	if continuousDays == 3 || continuousDays == 7 || continuousDays == 21 {
		master, err := s.Store.FindStreakMasterByDays(ctx, continuousDays)
		if err != nil {
			return nil, err
		}
		if master != nil && !master.ID.IsZero() {
			// O8+ enhanced coins are display-only: the write path hardcodes false (inventory §7.8).
			if _, err := s.SaveRewardTransaction(ctx, SaveRewardInput{
				UserID: userID, Master: master, PhoneNumber: phone, CaseID: caseID,
				Reason: RemarkMilestone(master.Slug, common.ISTDateString(checkInDate)),
			}); err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

// CreditStreakRestartBonus credits the restart bonus once per IST day.
// credit_remarks stays the human string for display parity; the guard is an explicit same-day check.
func (s *Service) CreditStreakRestartBonus(ctx context.Context, userID, phone string, amount int, checkInDate time.Time, caseID string) error {
	master, err := s.Store.FindStreakMasterBySlug(ctx, SlugExistingCoinsStreak, true)
	if err != nil || master == nil || master.ID.IsZero() {
		return err
	}
	dayStart := common.UTCMidnight(checkInDate)
	dayEnd := dayStart.Add(24*time.Hour - time.Millisecond)
	existing, err := s.Store.RewardTxnsCreatedBetween(ctx, userID, dayStart, dayEnd)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		txns, err := s.Store.FindRewardTransactionsByUser(ctx, userID, true, nil)
		if err != nil {
			return err
		}
		for _, t := range txns {
			if t.CreditRemarks == RemarkStreakRestart && !t.CreatedAt.Before(dayStart) && !t.CreatedAt.After(dayEnd) {
				return nil // already granted today
			}
		}
	}
	_, err = s.SaveRewardTransaction(ctx, SaveRewardInput{
		UserID: userID, Master: master, PhoneNumber: phone, Reason: RemarkStreakRestart, CustomAmount: &amount, CaseID: caseID,
	})
	return err
}
