package logearn

import (
	"context"
	"strings"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
)

// ZAmountForDay mirrors zAmountForDay: the first tier covering the streak day.
func ZAmountForDay(streakDay int, tiers []ZTier) (tier, amount int) {
	if len(tiers) == 0 {
		tiers = DefaultZTiers
	}
	for i, t := range tiers {
		if t.UpTo == nil || streakDay <= *t.UpTo {
			return i, t.Amount
		}
	}
	last := len(tiers) - 1
	return last, tiers[last].Amount
}

// BonusForDay mirrors bonusForDay.
func BonusForDay(streakDay int, isFirstEver bool, b Bonuses) int {
	bonus := 0
	if isFirstEver {
		bonus += b.FirstEver
	}
	if streakDay == 3 {
		bonus += b.DayThree
	}
	if b.MilestoneEvery > 0 && streakDay > 0 && streakDay%b.MilestoneEvery == 0 {
		bonus += b.MilestoneAmount
	}
	return bonus
}

// ComputeStreakDay mirrors computeStreakDay. previousLogs must be newest-first.
func ComputeStreakDay(previousLogs []DoseLog, target time.Time, lifelineBridged bool) (streakDay int, isFirstEver bool) {
	if len(previousLogs) == 0 {
		return 1, true
	}
	last := previousLogs[0]
	for _, l := range previousLogs {
		if l.LogDate.After(last.LogDate) {
			last = l
		}
	}
	gap := common.ISTDaysBetween(last.LogDate, target)
	if gap <= 2 || lifelineBridged {
		return last.StreakDay + 1, false
	}
	return 1, false
}

// CapInfo is getEarningCapInfo's result.
type CapInfo struct {
	BulkX                int        `json:"bulkX"`
	TreatmentDay         int        `json:"treatmentDay"`
	EarningCapDays       int        `json:"earningCapDays"`
	TotalWindowDays      int        `json:"totalWindowDays"`
	EarningCapHit        bool       `json:"earningCapHit"`
	WindowExpired        bool       `json:"windowExpired"`
	EarningDaysConsumed  int        `json:"earningDaysConsumed"`
	EarningDaysRemaining int        `json:"earningDaysRemaining"`
	InGracePeriod        bool       `json:"inGracePeriod"`
	DeliveryDate         *time.Time `json:"deliveryDate"`
	OrderID              *string    `json:"orderId"`
	HasInFlightOrder     bool       `json:"-"`
	OrderServiceFailed   bool       `json:"-"`
}

func defaultCapInfo() CapInfo {
	return CapInfo{BulkX: DefaultBulkX, EarningCapDays: DefaultEarningCapDays, TotalWindowDays: DefaultTotalWindow,
		EarningDaysRemaining: DefaultEarningCapDays}
}

func isDeadStatus(status string) bool {
	s := strings.ToLower(strings.ReplaceAll(status, "-", "_"))
	for _, d := range DeadOrderStatuses {
		if s == d {
			return true
		}
	}
	return false
}

// GetEarningCapInfo ports logearn.service getEarningCapInfo.
func (s *Service) GetEarningCapInfo(ctx context.Context, customerID string) CapInfo {
	now := s.now()
	out := defaultCapInfo()
	rows, err := s.Orders.NonVoidOrdersByCustomer(ctx, s.TenantID, customerID)
	if err != nil {
		s.Log.Warn("order service unavailable for cap info", "customerId", customerID, "error", err.Error())
		out.OrderServiceFailed = true
		return out
	}
	var latest *orders.Order
	for i := range rows {
		o := rows[i].AsOrder()
		status := strings.ToLower(strings.ReplaceAll(o.Status, "-", "_"))
		if status != "" && status != "delivered" && !isDeadStatus(status) {
			out.HasInFlightOrder = true
		}
		if status != "delivered" {
			continue
		}
		if latest == nil || orders.AnchorDate(o).After(orders.AnchorDate(*latest)) {
			oc := o
			latest = &oc
		}
	}
	if latest == nil {
		return out
	}
	bulkX := latest.BulkOrderDuration
	if bulkX < 1 {
		bulkX = 1
	}
	if details, err := s.Orders.OrderDetails(ctx, s.TenantID, latest.ID); err == nil {
		if n := bulkDurationFrom(details); n > 0 {
			bulkX = n
		}
	}
	delivery := orders.AnchorDate(*latest)
	treatmentDay := int(now.Sub(delivery).Hours() / 24)
	if treatmentDay < 0 {
		treatmentDay = 0
	}
	consumed, err := s.Store.CountDoseLogsSince(ctx, customerID, delivery)
	if err != nil {
		s.Log.Warn("dose log count failed", "customerId", customerID, "error", err.Error())
	}
	capDays := bulkX * KitDaysPerBulkUnit
	total := capDays + GraceWindowDays
	remaining := capDays - consumed
	if remaining < 0 {
		remaining = 0
	}
	out.BulkX = bulkX
	out.TreatmentDay = treatmentDay
	out.EarningCapDays = capDays
	out.TotalWindowDays = total
	out.EarningDaysConsumed = consumed
	out.EarningDaysRemaining = remaining
	out.EarningCapHit = consumed >= capDays
	out.WindowExpired = treatmentDay > total
	out.InGracePeriod = out.EarningCapHit && !out.WindowExpired
	out.DeliveryDate = &delivery
	id := latest.ID
	out.OrderID = &id
	return out
}

// bulkDurationFrom walks the order-details payload for orders.bulkOrderDuration.
func bulkDurationFrom(payload map[string]any) int {
	var walk func(any) int
	walk = func(v any) int {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if k == "bulkOrderDuration" || k == "bulk_order_duration" {
					switch n := val.(type) {
					case float64:
						return int(n)
					case string:
						if p, err := parseInt(n); err == nil {
							return p
						}
					}
				}
				if got := walk(val); got > 0 {
					return got
				}
			}
		case []any:
			for _, item := range x {
				if got := walk(item); got > 0 {
					return got
				}
			}
		}
		return 0
	}
	return walk(payload)
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNotNumber
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var errNotNumber = errorString("not a number")

type errorString string

func (e errorString) Error() string { return string(e) }
