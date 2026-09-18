package legacy

import (
	"context"
	"sort"
	"time"

	"traya-bah-service/internal/common"
)

// Calendar day states.
const (
	CalStateInactive = "inactive"
	CalStateMissed   = "missed"
	CalStateLogged   = "logged"
	CalStateCoin     = "coin"
	CalStateActive   = "active"
)

// CalendarDay is one day cell.
type CalendarDay struct {
	Date   string `json:"date"`
	Status string `json:"status"`
}

// CalendarMonth groups days by "MMMM YYYY".
type CalendarMonth struct {
	Month     string        `json:"month"`
	MonthData []CalendarDay `json:"monthData"`
}

// CalendarResponse is the GET /bah/:userId/calendar response.
type CalendarResponse struct {
	StartDate string          `json:"startDate"`
	EndDate   string          `json:"endDate"`
	Data      []CalendarMonth `json:"data"`
}

func istKey(t time.Time) string { return common.ISTDateString(t) }

// GetBahCalendarLogData ports handler.js getBahCalendarLogData.
// The identity is the caller's; the route's :userId path segment is ignored (spec §6.1.1).
func (s *Service) GetBahCalendarLogData(ctx context.Context, userID, date, mode string) (*CalendarResponse, error) {
	now := s.now()
	if mode == "" {
		mode = "calendar"
	}
	parsed, err := common.ParseYMDIST(date)
	if err != nil {
		if t, err2 := time.Parse(time.RFC3339, date); err2 == nil {
			parsed = common.ISTDate(t)
		} else {
			return nil, common.Internal(MsgInvalidDateFormat)
		}
	}
	inputDate := common.ISTDate(parsed)

	firstDelivered, err := s.PG.FirstDeliveredDate(ctx, userID)
	if err != nil {
		return nil, err
	}
	deliveryDate := common.ISTDate(now)
	if firstDelivered != nil {
		deliveryDate = common.ISTDate(*firstDelivered)
	}

	var start, end time.Time
	var lastLogDate *time.Time
	switch mode {
	case "calendar":
		start = inputDate.AddDate(0, 0, -60)
		if start.Before(deliveryDate) {
			start = deliveryDate
		}
		lastVisible := common.ISTDate(inputDate.AddDate(0, 1, 0).AddDate(0, 0, -inputDate.Day()))
		end = inputDate
		if lastVisible.After(end) {
			end = lastVisible
		}
	case "streak":
		streak, err := s.Store.FindActiveStreak(ctx, userID)
		if err != nil {
			return nil, err
		}
		if streak == nil || streak.ID.IsZero() {
			return nil, common.Internal(MsgNoActiveStreak)
		}
		l := streak.LastDateOfLog
		lastLogDate = &l
		first := common.ISTDate(streak.FirstDateOfLog)
		last := common.ISTDate(streak.LastDateOfLog)
		currentStreakLength := int(last.Sub(first).Hours()/24) + 1
		gap := common.ISTDaysBetween(last, now)
		today := common.ISTDate(now)
		if currentStreakLength >= 21 || gap > 2 {
			start, end = today, today.AddDate(0, 0, 20)
		} else {
			start, end = first, first.AddDate(0, 0, 20)
		}
	default:
		return nil, common.Internal(MsgInvalidMode)
	}

	logs, err := s.Store.FindActivityLogsBetween(ctx, userID, start, end, true, nil)
	if err != nil {
		return nil, err
	}
	rewardDates, err := s.Store.RewardTxnsCreatedBetween(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}

	raw := map[string]string{}
	var order []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := istKey(d)
		order = append(order, key)
		switch {
		case mode == "calendar" && d.Before(deliveryDate):
			raw[key] = CalStateInactive
		case mode == "calendar" && d.After(inputDate):
			raw[key] = CalStateInactive
		default:
			raw[key] = CalStateMissed
		}
	}
	for _, l := range logs {
		if !l.IsValidForStreak {
			continue
		}
		key := istKey(l.CheckInsForDate)
		if _, ok := raw[key]; ok {
			raw[key] = CalStateLogged
		}
	}
	for _, r := range rewardDates {
		key := istKey(r)
		if raw[key] == CalStateLogged {
			raw[key] = CalStateCoin
		}
	}
	todayKey := istKey(now)
	yesterdayKey := istKey(now.AddDate(0, 0, -1))
	if raw[todayKey] == CalStateMissed {
		raw[todayKey] = CalStateActive
	}
	todayLoggedOrCoin := raw[todayKey] == CalStateLogged || raw[todayKey] == CalStateCoin
	if raw[yesterdayKey] == CalStateMissed && !todayLoggedOrCoin {
		raw[yesterdayKey] = CalStateActive
	}
	if mode == "streak" && lastLogDate != nil {
		lastKey := istKey(*lastLogDate)
		for _, key := range order {
			if key > lastKey && raw[key] == CalStateMissed {
				raw[key] = CalStateInactive
			}
		}
	}

	months := map[string][]CalendarDay{}
	var monthOrder []string
	for _, key := range order {
		d, _ := common.ParseYMDIST(key)
		label := common.FormatMoment(d, "MMMM YYYY")
		if _, ok := months[label]; !ok {
			monthOrder = append(monthOrder, label)
		}
		months[label] = append(months[label], CalendarDay{Date: key, Status: raw[key]})
	}
	out := &CalendarResponse{
		StartDate: istKey(start),
		EndDate:   istKey(now), // handler.js always returns today here (inventory §7.7)
	}
	for _, label := range monthOrder {
		out.Data = append(out.Data, CalendarMonth{Month: label, MonthData: months[label]})
	}
	return out, nil
}

func sortByTxnDateDesc(rows []map[string]any) {
	sort.SliceStable(rows, func(i, j int) bool {
		ti, iok := rows[i]["txnDate"].(time.Time)
		tj, jok := rows[j]["txnDate"].(time.Time)
		if !iok || !jok {
			return false
		}
		return ti.After(tj)
	})
}
