// Package lifeline implements the habit-tracker lifeline scheduler, worker and daily reconcile.
// It replaces api-server's BullMQ queue with a Redis sorted set; the decision rules are unchanged.
package lifeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

// Defaults mirroring lifelineQueue.js / habitKitWindow.js.
const (
	LifelinesTotal    = 3
	PauseAfterLogs    = 35
	DefaultGraceHours = 4
)

// CheckDateFor mirrors lifelineSchedule.checkDateFor: the IST day after the last log.
func CheckDateFor(lastLog time.Time) string {
	return common.ISTDateString(common.ISTStartOfDay(lastLog).AddDate(0, 0, 1))
}

// DueAtFor mirrors lifelineSchedule.dueAtFor: IST(checkDate) + 1 day + grace hours.
func DueAtFor(checkDate string, graceHours int) time.Time {
	d, err := common.ParseYMDIST(checkDate)
	if err != nil {
		return time.Time{}
	}
	return d.AddDate(0, 0, 1).Add(time.Duration(graceHours) * time.Hour).UTC()
}

// Decision is lifelineDecision.js' result.
type Decision struct {
	Action       string // "intact" | "apply" | "break"
	ScheduleNext bool
}

// Decide ports lifelineDecision.js, in that exact order.
func Decide(checkDateLogged, streakAlive, checkDateInKit, kitPaused bool, budgetRemaining int) Decision {
	switch {
	case checkDateLogged:
		return Decision{Action: "intact", ScheduleNext: true}
	case !streakAlive:
		return Decision{Action: "intact", ScheduleNext: false}
	case !checkDateInKit:
		return Decision{Action: "intact", ScheduleNext: false}
	case kitPaused:
		return Decision{Action: "break", ScheduleNext: false}
	case budgetRemaining > 0:
		return Decision{Action: "apply", ScheduleNext: true}
	default:
		return Decision{Action: "break", ScheduleNext: false}
	}
}

// StreakState is lifelineState.getStreakState's result.
type StreakState struct {
	StreakAlive     bool
	CheckDateLogged bool
	StreakDay       int
}

// KitContext is lifelineState.getKitContext's result.
type KitContext struct {
	KitStart        *time.Time
	BudgetRemaining int
	KitPaused       bool
}

// Job is one scheduled check.
type Job struct {
	UserID    string `json:"userId"`
	CheckDate string `json:"checkDate"`
}

// Creditor credits the reward for a bridged day.
type Creditor func(ctx context.Context, userID string, streakDay int, checkDate string) error

// OrderSource supplies a user's non-void orders.
type OrderSource func(ctx context.Context, userID string) ([]orders.Order, error)

// Deps are the collaborators the worker needs.
type Deps struct {
	Store       *mongorepo.Store
	Orders      OrderSource
	Clock       common.Clock
	Credit      Creditor
	TenantID    string
	GraceHours  int
	TestDelayMS *int
	Log         *slog.Logger
}

func (d *Deps) now() time.Time {
	if d.Clock == nil {
		return time.Now().UTC()
	}
	return d.Clock.Now()
}

func (d *Deps) grace() int {
	if d.GraceHours <= 0 {
		return DefaultGraceHours
	}
	return d.GraceHours
}

// GetStreakState ports lifelineState.getStreakState.
func (d *Deps) GetStreakState(ctx context.Context, userID, checkDate string) (StreakState, error) {
	streak, err := d.Store.FindActiveStreak(ctx, userID)
	if err != nil {
		return StreakState{}, err
	}
	day0, err := common.ParseYMD(checkDate)
	if err != nil {
		return StreakState{}, err
	}
	logged, err := d.Store.FindActivityLogInRange(ctx, userID, mongorepo.DayRange{
		From: day0, To: day0.AddDate(0, 0, 1), FromInclusive: true}, true)
	if err != nil {
		return StreakState{}, err
	}
	st := StreakState{}
	if streak != nil && !streak.ID.IsZero() {
		st.StreakDay = streak.StreakAchieveDays
		st.StreakAlive = streak.StreakAchieveDays > 0
	}
	// the JS also requires is_valid_for_streak and excludes lifeline docs
	if logged != nil && !logged.ID.IsZero() && logged.IsValidForStreak && !logged.Lifeline() {
		st.CheckDateLogged = true
	}
	return st, nil
}

// GetKitContext ports lifelineState.getKitContext.
func (d *Deps) GetKitContext(ctx context.Context, userID string) (KitContext, error) {
	now := d.now()
	ords, err := d.Orders(ctx, userID)
	if err != nil {
		return KitContext{}, err
	}
	kitStart := orders.CurrentKitStart(ords, now, orders.KitCountLegacy(now))
	if kitStart == nil {
		return KitContext{BudgetRemaining: LifelinesTotal}, nil
	}
	used, err := d.Store.CountLifelinesSince(ctx, userID, *kitStart)
	if err != nil {
		return KitContext{}, err
	}
	validLogs, err := d.Store.CountValidStreakLogs(ctx, userID, kitStart)
	if err != nil {
		return KitContext{}, err
	}
	budget := LifelinesTotal - int(used)
	if budget < 0 {
		budget = 0
	}
	return KitContext{KitStart: kitStart, BudgetRemaining: budget, KitPaused: validLogs >= PauseAfterLogs}, nil
}

// ApplyLifeline ports applyLifeline.js: log first, then bridge the activity doc and advance the streak.
func (d *Deps) ApplyLifeline(ctx context.Context, userID, checkDate string, kitStart time.Time, streakDay int) (bool, error) {
	dateCovered, err := common.ParseYMD(checkDate)
	if err != nil {
		return false, err
	}
	now := d.now()
	created, err := d.Store.CreateLifelineLog(ctx, &models.LifelineLog{
		UserID: userID, DateCovered: dateCovered, KitStart: kitStart, AppliedAt: now,
		Source: "auto", StreakDayAtApply: streakDay,
	})
	if err != nil {
		return false, err
	}
	exists, err := d.Store.ExistsActivityLogInRange(ctx, userID, mongorepo.DayRange{
		From: dateCovered, To: dateCovered.AddDate(0, 0, 1), FromInclusive: true}, true)
	if err != nil {
		return created, err
	}
	if !exists {
		prescriptions, err := d.Store.LatestActivityLogPrescriptions(ctx, userID)
		if err != nil {
			return created, err
		}
		if err := d.Store.UpsertLifelineActivityLog(ctx, userID, dateCovered, prescriptions); err != nil {
			return created, err
		}
	}
	if err := d.Store.AdvanceStreakForDate(ctx, userID, dateCovered); err != nil {
		return created, err
	}
	return created, nil
}

// ProcessJob ports lifelineWorker.processLifelineJob; the second return is the next job to schedule.
func (d *Deps) ProcessJob(ctx context.Context, j Job) (Decision, *Job, error) {
	state, err := d.GetStreakState(ctx, j.UserID, j.CheckDate)
	if err != nil {
		return Decision{}, nil, err
	}
	kit, err := d.GetKitContext(ctx, j.UserID)
	if err != nil {
		return Decision{}, nil, err
	}
	checkDateInKit := kit.KitStart != nil && j.CheckDate >= common.FormatMoment(*kit.KitStart, "YYYY-MM-DD")
	decision := Decide(state.CheckDateLogged, state.StreakAlive, checkDateInKit, kit.KitPaused, kit.BudgetRemaining)

	switch decision.Action {
	case "apply":
		created, err := d.ApplyLifeline(ctx, j.UserID, j.CheckDate, *kit.KitStart, state.StreakDay+1)
		if err != nil {
			return decision, nil, err
		}
		if created && d.Credit != nil {
			if err := d.Credit(ctx, j.UserID, state.StreakDay+1, j.CheckDate); err != nil && d.Log != nil {
				d.Log.Warn("lifeline reward credit failed", "userId", j.UserID, "error", err.Error())
			}
		}
	case "break":
		if err := d.Store.BreakStreak(ctx, j.UserID); err != nil {
			return decision, nil, err
		}
	}
	if !decision.ScheduleNext {
		return decision, nil, nil
	}
	cur, err := common.ParseYMD(j.CheckDate)
	if err != nil {
		return decision, nil, err
	}
	next := Job{UserID: j.UserID, CheckDate: common.FormatMoment(cur.AddDate(0, 0, 1), "YYYY-MM-DD")}
	return decision, &next, nil
}

// ReconcileUserLifelines ports lifelineReconcile.reconcileUserLifelines (removal only).
func ReconcileUserLifelines(lifelineDates []string, realValid map[string]bool, budget int) (keep, remove []string) {
	var candidates []string
	redundant := map[string]bool{}
	for _, d := range lifelineDates {
		if realValid[d] {
			redundant[d] = true
			continue
		}
		candidates = append(candidates, d)
	}
	sortStrings(candidates)
	if budget > len(candidates) {
		budget = len(candidates)
	}
	keep = append(keep, candidates[:budget]...)
	kept := map[string]bool{}
	for _, k := range keep {
		kept[k] = true
	}
	for _, d := range lifelineDates {
		if !kept[d] {
			remove = append(remove, d)
		}
	}
	sortStrings(remove)
	return keep, remove
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

var _ = redis.Nil
