package legacy

import (
	"context"
	"strings"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
)

func firstCharIn(s string, set []string) bool {
	if s == "" {
		return false
	}
	c := strings.ToLower(s[:1])
	for _, x := range set {
		if x == c {
			return true
		}
	}
	return false
}

// CheckO8PlusEligibility ports handler.js checkO8PlusEligibility.
func CheckO8PlusEligibility(ords []orders.Order, now time.Time) bool {
	if len(ords) < O8PlusMinOrderCount {
		return false
	}
	caseID := ords[0].CaseID
	if caseID == "" || !firstCharIn(caseID, O8PlusVariationGroup) {
		return false
	}
	goLive, err := common.ParseYMD(O8PlusGoLiveDate)
	if err != nil {
		return false
	}
	end := common.EndOfDayUTC(goLive.AddDate(0, 0, O8PlusExperimentWindow))
	for _, o := range ords { // list is created_at DESC; first with a delivery_date
		if o.DeliveryDate == nil {
			continue
		}
		d := common.UTCMidnight(*o.DeliveryDate)
		return !d.Before(goLive) && !d.After(end)
	}
	return false
}

// IsMaleStreakRestartCohort ports isMaleStreakRestartCohort.
func IsMaleStreakRestartCohort(caseID, gender string) bool {
	return gender == "M" && firstCharIn(caseID, StreakRestartVariationGroup)
}

// IsFemaleStreakRestartCohort ports isFemaleStreakRestartCohort.
func IsFemaleStreakRestartCohort(caseID, gender string, orderCount int) bool {
	if gender != "F" || !firstCharIn(caseID, StreakRestartVariationGroup) {
		return false
	}
	for _, n := range StreakRestartFemaleOrderCounts {
		if n == orderCount {
			return true
		}
	}
	return false
}

// CheckStreakRestartBonusEligibility is the male predicate used by the read path.
func CheckStreakRestartBonusEligibility(caseID, gender string) bool {
	return StreakRestartBonusEnabled && IsMaleStreakRestartCohort(caseID, gender)
}

// ResolveStreakRestartBonusAmount ports resolveStreakRestartBonusAmount (log-time only).
func (s *Service) ResolveStreakRestartBonusAmount(ctx context.Context, userID, caseID, gender string) (*int, error) {
	if CheckStreakRestartBonusEligibility(caseID, gender) {
		v := StreakRestartBonusCoins
		return &v, nil
	}
	if gender == "F" && StreakRestartBonusFemaleEnabled {
		ords, err := s.PG.NonVoidOrdersByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if IsFemaleStreakRestartCohort(caseID, gender, len(ords)) {
			v := StreakRestartBonusFemaleCoins
			return &v, nil
		}
	}
	return nil, nil
}

// FifteenDayEligibility is checkFifteenDayCheckinCallEligibility's result.
type FifteenDayEligibility struct {
	W1NotCompleted, W3NotCompleted, NoRecentCall bool
}

// Eligible reports all three conditions.
func (e FifteenDayEligibility) Eligible() bool {
	return e.W1NotCompleted && e.W3NotCompleted && e.NoRecentCall
}

// CheckFifteenDayCheckinCallEligibility ports the user_order_reminders query.
func (s *Service) CheckFifteenDayCheckinCallEligibility(ctx context.Context, userID string) (FifteenDayEligibility, error) {
	out := FifteenDayEligibility{W1NotCompleted: true, W3NotCompleted: true, NoRecentCall: true}
	since := s.now().AddDate(0, 0, -3)
	rows, err := s.PG.FinishedReminders(ctx, userID, since)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		switch r.Tag {
		case "Week #1":
			out.W1NotCompleted = false
		case "Week #3":
			out.W3NotCompleted = false
		}
		if r.ActualDate != nil && !r.ActualDate.Before(since) {
			out.NoRecentCall = false
		}
	}
	return out, nil
}

// FeedbackUIDomain picks the environment's feedback-ui host.
func (s *Service) FeedbackUIDomain() string {
	if s.Cfg != nil && s.Cfg.IsProduction {
		return FeedbackUIDomainProd
	}
	return FeedbackUIDomainDev
}
