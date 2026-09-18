package lifeline

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"traya-bah-service/internal/common"
)

// Worker polls the due set and processes checks.
type Worker struct {
	Q           *Queue
	D           *Deps
	Concurrency int
	PollEvery   time.Duration
	Visibility  time.Duration
}

// Run polls until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	if w.Q == nil || w.D == nil {
		return
	}
	if w.Concurrency <= 0 {
		w.Concurrency = 5
	}
	if w.PollEvery <= 0 {
		w.PollEvery = 5 * time.Second
	}
	if w.Visibility <= 0 {
		w.Visibility = 60 * time.Second
	}
	ticker := time.NewTicker(w.PollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	now := w.D.now()
	if err := w.Q.RequeueStale(ctx, now); err != nil {
		w.D.Log.Warn("lifeline requeue stale failed", "error", err.Error())
	}
	jobs, err := w.Q.PopDue(ctx, now, w.Concurrency*4, w.Visibility)
	if err != nil {
		w.D.Log.Warn("lifeline pop due failed", "error", err.Error())
		return
	}
	if len(jobs) == 0 {
		return
	}
	sem := make(chan struct{}, w.Concurrency)
	var wg sync.WaitGroup
	for _, job := range jobs {
		j := job
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.runJob(ctx, j)
		}()
	}
	wg.Wait()
}

func (w *Worker) runJob(ctx context.Context, j Job) {
	defer func() {
		if r := recover(); r != nil {
			w.D.Log.Error("lifeline job panicked", "userId", j.UserID, "panic", r)
		}
	}()
	_, next, err := w.D.ProcessJob(ctx, j)
	if err != nil {
		w.D.Log.Error("lifeline job failed", "userId", j.UserID, "checkDate", j.CheckDate, "error", err.Error())
		return
	}
	if err := w.Q.Ack(ctx, j); err != nil {
		w.D.Log.Warn("lifeline ack failed", "userId", j.UserID, "error", err.Error())
	}
	// scheduling the next check happens after completion, like the BullMQ 'completed' handler
	if next != nil {
		if err := w.Q.Enqueue(ctx, *next, DueAtFor(next.CheckDate, w.D.grace())); err != nil {
			w.D.Log.Warn("lifeline next enqueue failed", "userId", j.UserID, "error", err.Error())
		}
	}
}

// ReconcileLockKey guards the daily reconcile so only one instance runs it.
func (w *Worker) ReconcileLockKey() string { return "bah:" + w.D.TenantID + ":lifeline:reconcile-lock" }

// RunReconcileDaily seeds a check for every active streak without a pending one, at 05:00 IST.
func (w *Worker) RunReconcileDaily(ctx context.Context) {
	if w.Q == nil || w.D == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := w.D.now().In(common.IST)
			if now.Hour() != 5 || now.Minute() != 0 {
				continue
			}
			ok, err := w.Q.R.SetNX(ctx, w.ReconcileLockKey(), "1", 10*time.Minute).Result()
			if err != nil || !ok {
				continue
			}
			if _, _, err := w.Reconcile(ctx); err != nil {
				w.D.Log.Error("lifeline reconcile failed", "error", err.Error())
			}
		}
	}
}

// Reconcile seeds checks for active streaks with none pending; returns (active, seeded).
func (w *Worker) Reconcile(ctx context.Context) (int, int, error) {
	streaks, err := w.D.Store.FindActiveStreaksWithDays(ctx)
	if err != nil {
		return 0, 0, err
	}
	pending, err := w.Q.PendingUserIDs(ctx)
	if err != nil {
		return 0, 0, err
	}
	pendingSet := map[string]bool{}
	for _, id := range pending {
		pendingSet[id] = true
	}
	seeded := 0
	for _, s := range streaks {
		if pendingSet[s.UserID] {
			continue
		}
		last := s.LastDateOfLog
		if last.IsZero() {
			last = w.D.now()
		}
		checkDate := CheckDateFor(last)
		if err := w.Q.Enqueue(ctx, Job{UserID: s.UserID, CheckDate: checkDate}, DueAtFor(checkDate, w.D.grace())); err != nil {
			w.D.Log.Warn("lifeline seed failed", "userId", s.UserID, "error", err.Error())
			continue
		}
		seeded++
	}
	w.D.Log.Info("lifeline reconcile", "active", len(streaks), "pending", len(pending), "seeded", seeded)
	return len(streaks), seeded, nil
}

var _ = redis.Nil
