package habit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
)

// BadgeView is one badge row.
type BadgeView struct {
	KitNumber int     `json:"kitNumber"`
	Name      string  `json:"name"`
	Image     *string `json:"image"`
	Threshold int     `json:"threshold"`
	Earned    bool    `json:"earned"`
	LogsInKit int     `json:"logsInKit"`
	EarnedAt  string  `json:"earnedAt"`
	Upcoming  *bool   `json:"upcoming,omitempty"`
}

// BadgesResponse is GET /config/cms/kit-tracker-badges.
type BadgesResponse struct {
	EarnedCount int         `json:"earnedCount"`
	Badges      []BadgeView `json:"badges"`
}

// ResolveBadgeImage mirrors resolveHabitTrackerBadgeImage; nil past kit 21.
func ResolveBadgeImage(kitNumber int, earned bool, genderKey string) *string {
	if kitNumber > BadgeImageMaxKit {
		return nil
	}
	key := "disabled"
	if earned {
		key = genderKey
	}
	url := strings.Replace(BadgeImages[key], "{n}", fmt.Sprintf("%d", kitNumber), 1)
	return &url
}

// FormatBadgeEarnedAt mirrors formatHabitTrackerBadgeEarnedAt.
func FormatBadgeEarnedAt(earnedAt *time.Time, earned bool) string {
	if !earned || earnedAt == nil || earnedAt.IsZero() {
		return "Upcoming"
	}
	return common.FormatMoment(*earnedAt, "D MMMM, YYYY")
}

// DeriveBadges ports deriveHabitTrackerBadges.
func DeriveBadges(windows []orders.KitWindow, valid map[string]bool, earnedByKit map[int]time.Time, gender string) (all []BadgeView, earnedCount int, newly []int) {
	genderKey := "M"
	if gender == "F" {
		genderKey = "F"
	}
	byKit := map[int]orders.KitWindow{}
	maxKit := BadgesTotal
	for _, w := range windows {
		byKit[w.KitNumber] = w
		if w.KitNumber > maxKit {
			maxKit = w.KitNumber
		}
	}
	for k := range earnedByKit {
		if k > maxKit {
			maxKit = k
		}
	}
	for kit := 1; kit <= maxKit; kit++ {
		logsInKit := 0
		if w, ok := byKit[kit]; ok {
			for key := range valid {
				d, err := common.ParseYMD(key)
				if err != nil {
					continue
				}
				if !d.Before(common.UTCMidnight(w.Start)) && d.Before(common.UTCMidnight(w.End)) {
					logsInKit++
				}
			}
		}
		derivedEarned := logsInKit >= BadgeThreshold
		earnedAt, already := earnedByKit[kit]
		earned := derivedEarned || already
		if derivedEarned && !already {
			newly = append(newly, kit)
		}
		var at *time.Time
		if already {
			e := earnedAt
			at = &e
		}
		if earned {
			earnedCount++
		}
		all = append(all, BadgeView{
			KitNumber: kit, Name: fmt.Sprintf("Kit %d", kit), Image: ResolveBadgeImage(kit, earned, genderKey),
			Threshold: BadgeThreshold, Earned: earned, LogsInKit: logsInKit, EarnedAt: FormatBadgeEarnedAt(at, earned),
		})
	}
	return all, earnedCount, newly
}

// SelectBadgesForDisplay ports selectBadgesForDisplay (earned + exactly one upcoming, newest first).
func SelectBadgesForDisplay(all []BadgeView) []BadgeView {
	highestEarned := 0
	for _, b := range all {
		if b.Earned && b.KitNumber > highestEarned {
			highestEarned = b.KitNumber
		}
	}
	cutoff := highestEarned
	for _, b := range all {
		if !b.Earned && b.KitNumber > highestEarned {
			cutoff = b.KitNumber
			break
		}
	}
	out := []BadgeView{}
	for _, b := range all {
		if b.KitNumber <= cutoff && (b.Earned || b.KitNumber == cutoff) {
			c := b
			up := !b.Earned
			c.Upcoming = &up
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].KitNumber > out[j].KitNumber })
	return out
}

// GetHabitTrackerBadges ports bahService.getHabitTrackerBadges. Note it WRITES: newly earned badges
// are upserted on read, matching app-backend.
func (s *Service) GetHabitTrackerBadges(ctx context.Context, userID, caseID string, nonVoid []orders.Order, gender string) (*BadgesResponse, error) {
	now := s.now()
	ords := nonVoid
	if ords == nil && caseID != "" {
		o, err := s.PG.NonVoidOrdersByCase(ctx, caseID)
		if err != nil {
			return nil, err
		}
		ords = o
	}
	dates, err := s.Store.FindValidStreakLogDates(ctx, userID)
	if err != nil {
		return nil, err
	}
	earnedRows, err := s.Store.FindEarnedBadges(ctx, userID)
	if err != nil {
		return nil, err
	}
	if gender == "" {
		gender = "M"
		if uc, err := s.PG.UserCaseByUserID(ctx, userID); err == nil && uc != nil && uc.Gender != "" {
			gender = strings.ToUpper(uc.Gender[:1])
		}
	}
	valid := map[string]bool{}
	for _, d := range dates {
		valid[dateKey(d)] = true
	}
	earnedByKit := map[int]time.Time{}
	for _, b := range earnedRows {
		earnedByKit[b.KitNumber] = b.EarnedAt
	}
	all, earnedCount, newly := DeriveBadges(orders.HabitTrackerKitWindows(ords, now), valid, earnedByKit, gender)
	if len(newly) > 0 {
		for _, kit := range newly {
			if err := s.Store.UpsertEarnedBadge(ctx, userID, kit, now, "auto"); err != nil {
				s.Log.Warn("badge upsert failed", "userId", userID, "kit", kit, "error", err.Error())
			}
		}
		for i := range all {
			for _, kit := range newly {
				if all[i].KitNumber == kit {
					all[i].EarnedAt = common.FormatMoment(now, "D MMMM, YYYY")
				}
			}
		}
	}
	return &BadgesResponse{EarnedCount: earnedCount, Badges: SelectBadgesForDisplay(all)}, nil
}
