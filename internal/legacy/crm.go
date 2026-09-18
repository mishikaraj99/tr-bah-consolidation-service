package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/sync/errgroup"

	"github.com/google/uuid"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

type mongoDayRange = mongorepo.DayRange

func newUUID() string { return uuid.NewString() }

// mergeStruct folds a JSON-marshalable struct into a map (the JS spread-merge).
func mergeStruct(dst map[string]any, src any) {
	if src == nil {
		return
	}
	b, err := json.Marshal(src)
	if err != nil {
		return
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return
	}
	for k, v := range m {
		dst[k] = v
	}
}

// StreakMasterOption is one GET /streakMaster row.
type StreakMasterOption struct {
	StreakMasterID string `json:"streakMasterId"`
	SlugName       string `json:"slugName"`
	DisplayName    string `json:"displayName"`
	RewardCoins    int    `json:"rewardCoins"`
}

// GetMasterStreakDataForExtraBonusStreak ports handler.js getMasterStreakDataForExtraBonusStreak.
func (s *Service) GetMasterStreakDataForExtraBonusStreak(ctx context.Context) ([]StreakMasterOption, error) {
	masters, err := s.Store.FindSuperadminStreakMasters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StreakMasterOption, 0, len(masters))
	for _, m := range masters {
		out = append(out, StreakMasterOption{StreakMasterID: m.ID.Hex(), SlugName: m.Slug, DisplayName: m.DisplayName, RewardCoins: m.RewardCoins})
	}
	return out, nil
}

// ActivityLogsMonth is the getActivityLogs month grid.
type ActivityLogsMonth struct {
	ActivityLogs        map[string][]map[string]any `json:"activityLogs"`
	ActivityLogTime     map[string]*time.Time       `json:"activityLogTime"`
	IsYesterdayLogExist bool                        `json:"isYesterdayLogExist"`
}

// GetActivityLogs ports handler.js getActivityLogs (month grid keyed by day-of-month).
func (s *Service) GetActivityLogs(ctx context.Context, userID string, year, month *int) (*ActivityLogsMonth, error) {
	now := s.now()
	y, m := now.Year(), int(now.Month())
	start := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
	end := now
	if year != nil && month != nil {
		y, m = *year, *month
		start = time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, 0).Add(-time.Millisecond)
	}
	daysInMonth := start.AddDate(0, 1, 0).AddDate(0, 0, -1).Day()

	out := &ActivityLogsMonth{ActivityLogs: map[string][]map[string]any{}, ActivityLogTime: map[string]*time.Time{}}
	for d := 1; d <= daysInMonth; d++ {
		key := fmt.Sprintf("%d", d)
		out.ActivityLogs[key] = []map[string]any{}
		out.ActivityLogTime[key] = nil
	}
	logs, err := s.Store.FindActivityLogsBetween(ctx, userID, start, end, false, nil)
	if err != nil {
		return nil, err
	}
	for _, l := range logs {
		key := fmt.Sprintf("%d", l.CheckInsForDate.UTC().Day())
		if _, ok := out.ActivityLogs[key]; !ok {
			continue
		}
		out.ActivityLogs[key] = l.ProductPrescriptions
		ist := common.ISTShift(l.CreatedAt)
		out.ActivityLogTime[key] = &ist
	}
	yesterday := common.ISTShift(now).AddDate(0, 0, -1)
	yLog, err := s.Store.FindActivityLogInRange(ctx, userID, dayRange(yesterday), false)
	if err != nil {
		return nil, err
	}
	out.IsYesterdayLogExist = yLog != nil && !yLog.ID.IsZero()
	return out, nil
}

// GetBahHistoryOfUser ports handler.js getBahHistoryOfUser (CRM agent view).
func (s *Service) GetBahHistoryOfUser(ctx context.Context, caseID string, isCredit, isDebit bool, year, month *int, loggedInEmail string) (map[string]any, error) {
	userCase, err := s.PG.UserCaseByCaseID(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if userCase == nil || userCase.UserID == "" {
		return nil, common.BadRequest("Invalid case id")
	}
	userID := userCase.UserID

	var (
		history *RewardCoinHistory
		balance *StreakAndRewardBalance
		logs    *ActivityLogsMonth
		meds    *LatestMedicines
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		h, err := s.GetRewardCoinHistory(gctx, userID, isCredit, isDebit)
		history = h
		return err
	})
	g.Go(func() error {
		b, err := s.GetStreakAndRewardBalance(gctx, userID, 70, "")
		balance = b
		return err
	})
	g.Go(func() error {
		l, err := s.GetActivityLogs(gctx, userID, year, month)
		logs = l
		return err
	})
	g.Go(func() error {
		m, err := s.GetLatestMedicines(gctx, userID, false)
		meds = m
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	out := map[string]any{}
	mergeStruct(out, history)
	mergeStruct(out, balance)
	mergeStruct(out, logs)
	out["latestMedicines"] = meds
	out["isBalanceSyncingRequired"] = false // hardcoded in handler.js

	shopFloCoins := 0
	shopFloError := ""
	if s.Shopflo != nil && userCase.PhoneNumber != "" {
		w, err := s.Shopflo.Read(ctx, userCase.PhoneNumber)
		if err != nil {
			shopFloError = err.Error()
		} else if w != nil {
			shopFloCoins = int(math.Round(w.TotalWalletBalance * 10))
		}
	}
	out["shopFloCoins"] = shopFloCoins
	out["shopFloError"] = shopFloError

	authorized := false
	for _, e := range CoinCreditAccessEmailIDs {
		if e == loggedInEmail {
			authorized = true
			break
		}
	}
	out["isUserAuthorizedToGiveCoins"] = authorized
	return out, nil
}

// ExtraRewardResult is POST /extraRewardsToUser/:caseId's response.
type ExtraRewardResult struct {
	RewardRef any    `json:"rewardRef"`
	Message   string `json:"message"`
}

// SaveExtraBonusForUsersForAnyReason ports handler.js saveExtraBonusForUsersForAnyReason.
func (s *Service) SaveExtraBonusForUsersForAnyReason(ctx context.Context, caseID, streakMasterID, reason string, customAmount *int) (*ExtraRewardResult, error) {
	if streakMasterID == "" || reason == "" {
		return nil, common.BadRequest(MsgAllFieldsMandatory)
	}
	oid, err := primitive.ObjectIDFromHex(streakMasterID)
	if err != nil {
		return nil, common.BadRequest(MsgInvalidStreakMaster)
	}
	userCase, err := s.PG.UserCaseByCaseID(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if userCase == nil || userCase.UserID == "" {
		return nil, common.BadRequest("Invalid case id")
	}
	master, err := s.Store.FindStreakMasterByID(ctx, oid, true)
	if err != nil {
		return nil, err
	}
	if master == nil || master.ID.IsZero() {
		return nil, common.BadRequest(MsgInvalidStreakMaster)
	}
	if customAmount != nil && master.Slug != SlugExistingCoinsStreak {
		return nil, common.BadRequest(MsgInvalidStreakMasterCustom)
	}
	res, err := s.SaveRewardTransaction(ctx, SaveRewardInput{
		UserID: userCase.UserID, Master: master, PhoneNumber: userCase.PhoneNumber,
		Reason: reason, CustomAmount: customAmount, CaseID: caseID,
	})
	if err != nil {
		return nil, err
	}
	if res.Ref == nil {
		return &ExtraRewardResult{Message: MsgFailedToCreditCoins}, nil
	}
	return &ExtraRewardResult{RewardRef: res.Ref, Message: fmt.Sprintf(MsgCoinsCreditedSuccess, res.Ref.CreditCoins)}, nil
}

// SyncRewardBalanceWithShopflo ports handler.js syncRewardBalanceWithShopFloCoinBalance.
func (s *Service) SyncRewardBalanceWithShopflo(ctx context.Context, caseID string) (map[string]string, error) {
	userCase, err := s.PG.UserCaseByCaseID(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if userCase == nil || userCase.UserID == "" {
		return nil, common.BadRequest("Invalid case id")
	}
	local, err := s.GetOnlyRewardBalance(ctx, userCase.UserID)
	if err != nil {
		return nil, err
	}
	if s.Shopflo == nil {
		return nil, common.BadRequest(MsgNoShopfloCoins)
	}
	wallet, err := s.Shopflo.Read(ctx, userCase.PhoneNumber)
	if err != nil {
		return nil, err
	}
	if wallet == nil || math.IsNaN(wallet.TotalWalletBalance) {
		return nil, common.BadRequest(MsgNoShopfloCoins)
	}
	shopflo := int(math.Round(wallet.TotalWalletBalance * 10))
	switch {
	case shopflo > local:
		master, err := s.Store.FindStreakMasterBySlug(ctx, SlugExistingCoinsStreak, true)
		if err != nil {
			return nil, err
		}
		if master == nil || master.ID.IsZero() {
			return nil, common.BadRequest(MsgInvalidStreakMaster)
		}
		diff := shopflo - local
		// expire_at here is tomorrow + 1 month (not order-service driven)
		future := common.UTCMidnight(common.ISTShift(s.now()).AddDate(0, 1, 1))
		_, err = s.Store.InsertRewardTransaction(ctx, &models.RewardTransaction{
			UserID: userCase.UserID, StreakMasterID: master.ID, CreditCoins: diff,
			IsCreditTransaction: true, TotalDebitCoins: 0, CreditRemarks: RemarkSyncBalance,
			AllCoinsUsed: false, Status: "success", ExpireAt: &future,
		})
		if err != nil {
			return nil, err
		}
	case local > shopflo:
		diff := local - shopflo
		future := common.UTCMidnight(common.ISTShift(s.now()).AddDate(0, 1, 1))
		if err := s.Shopflo.Credit(ctx, diff, future.UnixMilli(), newUUID(), userCase.PhoneNumber); err != nil {
			return nil, err
		}
	}
	return map[string]string{"message": MsgSyncSuccess}, nil
}

func dayRange(clientDate time.Time) mongoDayRange {
	return mongoDayRange{From: common.StartOfDayUTC(clientDate), To: common.EndOfDayUTC(clientDate), FromInclusive: true, ToInclusive: true}
}
