package lifeline

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDateForAndDueAt(t *testing.T) {
	// 2026-09-18T18:29:59Z is 23:59:59 IST on the 18th → next check date is the 19th
	last := time.Date(2026, 9, 18, 18, 29, 59, 0, time.UTC)
	assert.Equal(t, "2026-09-19", CheckDateFor(last))
	// one second later it is the 19th IST → check the 20th
	assert.Equal(t, "2026-09-20", CheckDateFor(last.Add(time.Second)))

	due := DueAtFor("2026-09-19", 4)
	// IST midnight on the 19th is 2026-09-18T18:30Z; +1 day +4h
	assert.Equal(t, time.Date(2026, 9, 19, 22, 30, 0, 0, time.UTC), due.UTC())
}

func TestDecide(t *testing.T) {
	cases := []struct {
		name                         string
		logged, alive, inKit, paused bool
		budget                       int
		action                       string
		next                         bool
	}{
		{"logged → intact and reschedule", true, true, true, false, 3, "intact", true},
		{"streak dead → stop", false, false, true, false, 3, "intact", false},
		{"outside kit → stop", false, true, false, false, 3, "intact", false},
		{"kit paused → break", false, true, true, true, 3, "break", false},
		{"budget left → apply", false, true, true, false, 1, "apply", true},
		{"no budget → break", false, true, true, false, 0, "break", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.logged, c.alive, c.inKit, c.paused, c.budget)
			assert.Equal(t, c.action, got.Action)
			assert.Equal(t, c.next, got.ScheduleNext)
		})
	}
}

func TestReconcileUserLifelines(t *testing.T) {
	// a lifeline on a day that also has a real log is redundant and is removed
	keep, remove := ReconcileUserLifelines([]string{"2026-09-03", "2026-09-01", "2026-09-02"},
		map[string]bool{"2026-09-02": true}, 3)
	assert.Equal(t, []string{"2026-09-01", "2026-09-03"}, keep)
	assert.Equal(t, []string{"2026-09-02"}, remove)

	// over budget → keep the oldest, remove the rest
	keep, remove = ReconcileUserLifelines([]string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04"},
		map[string]bool{}, 2)
	assert.Equal(t, []string{"2026-09-01", "2026-09-02"}, keep)
	assert.Equal(t, []string{"2026-09-03", "2026-09-04"}, remove)
}

func testQueue(t *testing.T) *Queue {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	r := redis.NewClient(&redis.Options{Addr: addr})
	q := &Queue{R: r, TenantID: "test" + time.Now().Format("150405.000000")}
	t.Cleanup(func() {
		_ = r.Del(context.Background(), q.Key(), q.ProcessingKey()).Err()
		_ = r.Close()
	})
	return q
}

func TestQueue(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

	require.NoError(t, q.Enqueue(ctx, Job{UserID: "u1", CheckDate: "2026-09-18"}, now.Add(-time.Minute)))
	require.NoError(t, q.Enqueue(ctx, Job{UserID: "u2", CheckDate: "2026-09-18"}, now.Add(time.Hour)))

	// re-enqueueing the same user replaces the pending entry
	require.NoError(t, q.Enqueue(ctx, Job{UserID: "u1", CheckDate: "2026-09-19"}, now.Add(-time.Minute)))
	ids, err := q.PendingUserIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"u1", "u2"}, ids)

	jobs, err := q.PopDue(ctx, now, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, jobs, 1, "only u1 is due")
	assert.Equal(t, "u1", jobs[0].UserID)
	assert.Equal(t, "2026-09-19", jobs[0].CheckDate, "the replacement date won")

	// a popped job is invisible until its visibility timeout
	again, err := q.PopDue(ctx, now, 10, time.Minute)
	require.NoError(t, err)
	assert.Empty(t, again)

	// ack removes it for good
	require.NoError(t, q.Ack(ctx, jobs[0]))
	require.NoError(t, q.RequeueStale(ctx, now.Add(2*time.Minute)))
	after, err := q.PopDue(ctx, now.Add(2*time.Minute), 10, time.Minute)
	require.NoError(t, err)
	assert.Empty(t, after, "acked job does not come back")

	// an unacked job returns after the visibility timeout
	require.NoError(t, q.Enqueue(ctx, Job{UserID: "u3", CheckDate: "2026-09-18"}, now.Add(-time.Minute)))
	popped, err := q.PopDue(ctx, now, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, popped, 1)
	require.NoError(t, q.RequeueStale(ctx, now.Add(5*time.Minute)))
	back, err := q.PopDue(ctx, now.Add(5*time.Minute), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, back, 1)
	assert.Equal(t, "u3", back[0].UserID)
}

func TestEnqueueForLog(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	logDate := time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC)
	require.NoError(t, EnqueueForLog(ctx, q, "u1", logDate, 4, nil, now))
	jobs, err := q.PopDue(ctx, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "2026-09-19", jobs[0].CheckDate)

	// the test delay override makes the job due immediately
	delay := 0
	require.NoError(t, EnqueueForLog(ctx, q, "u2", logDate, 4, &delay, now))
	jobs, err = q.PopDue(ctx, now, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "u2", jobs[0].UserID)
}
