package logearn

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"traya-bah-service/internal/common"
	pgrepo "traya-bah-service/repositories/pg"
)

// DoseLog aliases the repository row so the pure helpers do not import pgrepo.
type DoseLog = pgrepo.DoseLog

// LogResult is the log/backfill response.
type LogResult struct {
	Success       bool `json:"success"`
	StreakDay     int  `json:"streakDay"`
	INRCredited   int  `json:"inrCredited"`
	EarningCapHit bool `json:"earningCapHit"`
	Balance       int  `json:"balance"`
}

// LifelineResult is the lifeline response.
type LifelineResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	Balance        int    `json:"balance"`
	PriorStreakDay int    `json:"priorStreakDay"`
	DaysBridged    int    `json:"daysBridged"`
}

// RedeemResult is the redeem response.
type RedeemResult struct {
	Success  bool `json:"success"`
	Redeemed int  `json:"redeemed"`
	Balance  int  `json:"balance"`
}

func (s *Service) balance(ctx context.Context, customerID string) int {
	n, err := s.Store.Balance(ctx, customerID)
	if err != nil {
		s.Log.Warn("balance read failed", "customerId", customerID, "error", err.Error())
		return 0
	}
	return n
}

// isLifelineBridgingGap mirrors isLifelineBridgingGap.
func (s *Service) isLifelineBridgingGap(ctx context.Context, customerID string, lastLog *time.Time) bool {
	if lastLog == nil {
		return false
	}
	entry, err := s.Store.LatestLedgerByReason(ctx, customerID, ReasonLifelineRecovered)
	if err != nil || entry == nil {
		return false
	}
	return entry.CreatedAt.After(*lastLog)
}

// orderGuard resolves the order to attribute a log to, mirroring the JS guards.
func (s *Service) orderGuard(cap CapInfo, orderID *string) (string, error) {
	if orderID != nil && *orderID != "" {
		return *orderID, nil
	}
	if cap.OrderID != nil && *cap.OrderID != "" {
		return *cap.OrderID, nil
	}
	if cap.HasInFlightOrder {
		return "", common.BadRequest(MsgKitOnTheWay)
	}
	return "", common.BadRequest(MsgNoActiveOrder)
}

// LogToday ports logearn.service logToday.
func (s *Service) LogToday(ctx context.Context, customerID string, orderID *string) (*LogResult, error) {
	now := s.now()
	previous, err := s.Store.RecentDoseLogs(ctx, customerID, 14)
	if err != nil {
		return nil, err
	}
	todayIST := common.ISTDateString(now)
	for _, l := range previous {
		if common.ISTDateString(l.LogDate) == todayIST {
			return nil, common.BadRequest(MsgAlreadyLoggedToday)
		}
	}
	cap := s.GetEarningCapInfo(ctx, customerID)
	resolvedOrder, err := s.orderGuard(cap, orderID)
	if err != nil {
		return nil, err
	}
	if cap.WindowExpired {
		return nil, common.BadRequest(MsgWindowExpired)
	}
	var lastLogDate *time.Time
	if len(previous) > 0 {
		d := previous[0].LogDate
		lastLogDate = &d
	}
	bridged := s.isLifelineBridgingGap(ctx, customerID, lastLogDate)
	streakDay, isFirstEver := ComputeStreakDay(previous, now, bridged)

	cfg := s.Config.Get(ctx, s.TenantID)
	tier, z := ZAmountForDay(streakDay, cfg.ZTiers)
	bonus := BonusForDay(streakDay, isFirstEver, cfg.Bonuses)
	reorderBonus := 0
	if streakDay == 1 && !isFirstEver {
		existing, err := s.Store.LatestLedgerByReason(ctx, customerID, ReasonReorderWelcome)
		if err != nil {
			return nil, err
		}
		if existing == nil || (cap.DeliveryDate != nil && existing.CreatedAt.Before(*cap.DeliveryDate)) {
			reorderBonus = cfg.Bonuses.ReorderWelcome
		}
	}
	if cap.EarningCapHit {
		z, bonus, reorderBonus = 0, 0, 0
	}
	total := z + bonus + reorderBonus

	bonusReason := ReasonMilestoneBonus
	if isFirstEver {
		bonusReason = ReasonFirstEverBonus
	} else if streakDay == 3 {
		bonusReason = ReasonDay3Bonus
	}

	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.Store.InsertDoseLog(ctx, tx, &pgrepo.DoseLog{
			CustomerID: customerID, OrderID: resolvedOrder, LogDate: common.ISTDayAnchor(now),
			StreakDay: streakDay, ZTier: tier, INRCredited: total, IsBackfill: false,
		}); err != nil {
			return err
		}
		if z > 0 {
			if _, err := s.Store.AppendLedger(ctx, tx, customerID, &resolvedOrder, z, ReasonDoseLog); err != nil {
				return err
			}
		}
		if bonus > 0 {
			if _, err := s.Store.AppendLedger(ctx, tx, customerID, &resolvedOrder, bonus, bonusReason); err != nil {
				return err
			}
		}
		if reorderBonus > 0 {
			if _, err := s.Store.AppendLedger(ctx, tx, customerID, &resolvedOrder, reorderBonus, ReasonReorderWelcome); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &LogResult{Success: true, StreakDay: streakDay, INRCredited: total,
		EarningCapHit: cap.EarningCapHit, Balance: s.balance(ctx, customerID)}, nil
}

// BackfillYesterday ports logearn.service backfillYesterday.
func (s *Service) BackfillYesterday(ctx context.Context, customerID string, orderID *string) (*LogResult, error) {
	now := s.now()
	yesterday := now.AddDate(0, 0, -1)
	previous, err := s.Store.RecentDoseLogs(ctx, customerID, 14)
	if err != nil {
		return nil, err
	}
	if len(previous) == 0 {
		return nil, common.BadRequest(MsgLogTodayFirst)
	}
	yKey := common.ISTDateString(yesterday)
	for _, l := range previous {
		if common.ISTDateString(l.LogDate) == yKey {
			return nil, common.BadRequest(MsgYesterdayLogged)
		}
	}
	cap := s.GetEarningCapInfo(ctx, customerID)
	resolvedOrder, err := s.orderGuard(cap, orderID)
	if err != nil {
		return nil, err
	}
	if cap.WindowExpired {
		return nil, common.BadRequest(MsgWindowExpired)
	}
	if common.ISTDaysBetween(previous[0].LogDate, now) > 2 {
		return nil, common.BadRequest(MsgStreakBrokenBackfill)
	}
	var before []DoseLog
	var todayLog *DoseLog
	todayKey := common.ISTDateString(now)
	for i := range previous {
		key := common.ISTDateString(previous[i].LogDate)
		if key < yKey {
			before = append(before, previous[i])
		}
		if key == todayKey {
			todayLog = &previous[i]
		}
	}
	var lastLogDate *time.Time
	if len(previous) > 0 {
		d := previous[0].LogDate
		lastLogDate = &d
	}
	bridged := s.isLifelineBridgingGap(ctx, customerID, lastLogDate)
	streakDay, _ := ComputeStreakDay(before, yesterday, bridged)

	cfg := s.Config.Get(ctx, s.TenantID)
	tier, z := ZAmountForDay(streakDay, cfg.ZTiers)
	bonus := BonusForDay(streakDay, false, cfg.Bonuses) // backfill never grants the first-ever bonus
	if cap.EarningCapHit {
		z, bonus = 0, 0
	}
	total := z + bonus
	bonusReason := ReasonMilestoneBonus
	if streakDay == 3 {
		bonusReason = ReasonDay3Bonus
	}

	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.Store.InsertDoseLog(ctx, tx, &pgrepo.DoseLog{
			CustomerID: customerID, OrderID: resolvedOrder, LogDate: common.ISTDayAnchor(yesterday),
			StreakDay: streakDay, ZTier: tier, INRCredited: total, IsBackfill: true,
		}); err != nil {
			return err
		}
		if todayLog != nil {
			if err := s.Store.UpdateDoseLogStreakDay(ctx, tx, todayLog.ID, streakDay+1); err != nil {
				return err
			}
		}
		if z > 0 {
			if _, err := s.Store.AppendLedger(ctx, tx, customerID, &resolvedOrder, z, ReasonBackfill); err != nil {
				return err
			}
		}
		if bonus > 0 {
			if _, err := s.Store.AppendLedger(ctx, tx, customerID, &resolvedOrder, bonus, bonusReason); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &LogResult{Success: true, StreakDay: streakDay, INRCredited: total,
		EarningCapHit: cap.EarningCapHit, Balance: s.balance(ctx, customerID)}, nil
}

// UseLifeline ports logearn.service useLifeline.
func (s *Service) UseLifeline(ctx context.Context, customerID string) (*LifelineResult, error) {
	now := s.now()
	logs, err := s.Store.RecentDoseLogs(ctx, customerID, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, common.BadRequest(MsgNoStreakToRecover)
	}
	latest := logs[0]
	gap := common.ISTDaysBetween(latest.LogDate, now)
	if gap <= 2 {
		return nil, common.BadRequest(MsgStreakNotBroken)
	}
	cap := s.GetEarningCapInfo(ctx, customerID)
	if cap.OrderID == nil || *cap.OrderID == "" {
		if cap.HasInFlightOrder {
			return nil, common.BadRequest(MsgKitOnTheWay)
		}
		return nil, common.BadRequest(MsgNoActiveOrder)
	}
	if cap.DeliveryDate != nil {
		used, err := s.Store.ExistsLedgerReasonSince(ctx, customerID, ReasonLifelineRecovered, *cap.DeliveryDate)
		if err != nil {
			return nil, err
		}
		if used {
			return nil, common.BadRequest(MsgLifelineAlreadyUsed)
		}
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := s.Store.AppendLedger(ctx, tx, customerID, cap.OrderID, 0, ReasonLifelineRecovered)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &LifelineResult{Success: true, Message: MsgLifelineApplied, Balance: s.balance(ctx, customerID),
		PriorStreakDay: latest.StreakDay, DaysBridged: gap}, nil
}

// Redeem ports logearn.service redeem, with a per-order idempotency guard (spec §6.1.11).
func (s *Service) Redeem(ctx context.Context, customerID string, subtotal int, amount *int, orderID *string) (*RedeemResult, error) {
	if orderID != nil && *orderID != "" {
		done, err := s.Store.ExistsRedeemForOrder(ctx, customerID, *orderID)
		if err != nil {
			return nil, err
		}
		if done {
			return nil, common.BadRequest(MsgOrderAlreadyRedeemed)
		}
	}
	balance := s.balance(ctx, customerID)
	maxRedeem := int(float64(subtotal) * RedeemSubtotalShare)
	redeemAmount := balance
	if amount != nil && *amount < redeemAmount {
		redeemAmount = *amount
	}
	if maxRedeem < redeemAmount {
		redeemAmount = maxRedeem
	}
	if redeemAmount <= 0 {
		return nil, common.BadRequest(MsgNothingToRedeem)
	}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := s.Store.AppendLedger(ctx, tx, customerID, orderID, -redeemAmount, ReasonRedeem)
		return err
	})
	if err != nil {
		if err == pgrepo.ErrInsufficientBalance {
			return nil, common.BadRequest(pgrepo.ErrInsufficientBalance.Error())
		}
		return nil, err
	}
	return &RedeemResult{Success: true, Redeemed: redeemAmount, Balance: s.balance(ctx, customerID)}, nil
}
