package legacy

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
)

// StreakResult is createStreakLogForUser's outcome.
type StreakResult struct {
	Streak          *models.StreakLog
	IsStreakBreaked bool
	AlreadyLogged   bool
}

// CreateStreakLogForUser ports handler.js createStreakLogForUser.
// Fix (spec §6.1.3): the update filter includes is_active:true.
func (s *Service) CreateStreakLogForUser(ctx context.Context, userID string, checkInDate time.Time, isValidForStreak bool, caseID string) (StreakResult, error) {
	if !isValidForStreak {
		return StreakResult{}, common.BadRequest(MsgNotEligibleToCreateStreak)
	}
	existing, err := s.Store.FindActiveStreak(ctx, userID)
	if err != nil {
		return StreakResult{}, err
	}
	var updated *models.StreakLog
	breaked := false
	if existing != nil && !existing.ID.IsZero() {
		daysDiff := common.CalendarDaysDifference(existing.LastDateOfLog, checkInDate)
		if daysDiff == 0 {
			return StreakResult{Streak: existing, AlreadyLogged: true}, nil
		}
		breaked = daysDiff > 1 || existing.StreakAchieveDays == 21
		set := bson.M{"streak_achieve_days": existing.StreakAchieveDays + 1, "last_date_of_log": checkInDate}
		if breaked {
			set["streak_achieve_days"] = 1
			set["first_date_of_log"] = checkInDate
		}
		updated, err = s.Store.UpdateActiveStreak(ctx, userID, set)
		if err != nil {
			return StreakResult{}, err
		}
	} else {
		active := true
		updated, err = s.Store.CreateStreak(ctx, &models.StreakLog{
			UserID: userID, StreakAchieveDays: 1, LongestStreakDays: 1,
			FirstDateOfLog: checkInDate, LastDateOfLog: checkInDate, IsActive: &active,
		})
		if err != nil {
			return StreakResult{}, err
		}
	}
	if updated == nil {
		return StreakResult{IsStreakBreaked: breaked}, nil
	}
	if updated.LongestStreakDays < updated.StreakAchieveDays {
		if u2, err := s.Store.UpdateActiveStreak(ctx, userID, bson.M{"longest_streak_days": updated.StreakAchieveDays}); err == nil && u2 != nil {
			updated = u2
		}
	}
	if s.CCD != nil && caseID != "" {
		s.CCD.Publish(ctx, s.TenantID, "ACTIVITY_LOG", caseID, map[string]any{
			"logDate": checkInDate, "streakDate": updated.LastDateOfLog, "streakCount": updated.StreakAchieveDays,
		})
	}
	return StreakResult{Streak: updated, IsStreakBreaked: breaked}, nil
}

// RunningLogDay is getBahRunningLogDay's response ({logRunningDay, userHasUsedBAH}).
type RunningLogDay struct {
	LogRunningDay  any  `json:"logRunningDay"`
	UserHasUsedBAH bool `json:"userHasUsedBAH"`
}

// GetBahRunningLogDay ports handler.js getBahRunningLogDay.
func (s *Service) GetBahRunningLogDay(ctx context.Context, userID string) (RunningLogDay, error) {
	streak, err := s.Store.FindActiveStreak(ctx, userID)
	if err != nil {
		return RunningLogDay{}, err
	}
	out := RunningLogDay{LogRunningDay: 0}
	if streak == nil || streak.ID.IsZero() {
		return out, nil
	}
	out.UserHasUsedBAH = true
	if common.CalendarDaysDifference(streak.LastDateOfLog, common.ISTShift(s.now())) > 2 {
		out.LogRunningDay = "regular_day"
		return out, nil
	}
	switch streak.StreakAchieveDays {
	case 2, 6, 20:
		out.LogRunningDay = streak.StreakAchieveDays
	default:
		out.LogRunningDay = "regular_day"
	}
	return out, nil
}

// StreakBrokenRecompute applies the version>=71 rule: a gap over two days, or day 21 plus one, resets to 0.
func StreakBrokenRecompute(current int, lastLog *time.Time, now time.Time) int {
	if lastLog == nil || lastLog.IsZero() {
		return current
	}
	diff := common.CalendarDaysDifference(*lastLog, now)
	if diff > 2 || (current == 21 && diff == 1) {
		return 0
	}
	return current
}

// IsLoggedToday mirrors isLoggedToday: same Y/M/D as now (process timezone UTC).
func IsLoggedToday(lastLog *time.Time, now time.Time) bool {
	if lastLog == nil || lastLog.IsZero() {
		return false
	}
	a, b := lastLog.UTC(), now.UTC()
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// GenderOf mirrors getGenderOfUser: upper-cased gender, default "M".
func GenderOf(g string) string {
	switch g {
	case "f", "F":
		return "F"
	case "m", "M":
		return "M"
	case "":
		return "M"
	}
	if len(g) > 0 {
		u := []rune(g)
		if u[0] >= 'a' && u[0] <= 'z' {
			u[0] -= 32
		}
		return string(u)
	}
	return "M"
}
