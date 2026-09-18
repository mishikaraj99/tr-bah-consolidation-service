package lifeline

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Queue is the Redis sorted-set scheduler that replaces BullMQ.
type Queue struct {
	R        *redis.Client
	TenantID string
}

// Key is the due-set key.
func (q *Queue) Key() string { return "bah:" + q.TenantID + ":lifeline:due" }

// ProcessingKey is the in-flight set key (visibility timeout).
func (q *Queue) ProcessingKey() string { return "bah:" + q.TenantID + ":lifeline:processing" }

func member(j Job) string { return j.UserID + "|" + j.CheckDate }

func parseMember(m string) (Job, bool) {
	userID, checkDate, ok := strings.Cut(m, "|")
	if !ok {
		return Job{}, false
	}
	return Job{UserID: userID, CheckDate: checkDate}, true
}

// Enqueue upserts the user's pending check (one per user, like the BullMQ jobId).
func (q *Queue) Enqueue(ctx context.Context, j Job, dueAt time.Time) error {
	if q == nil || q.R == nil {
		return nil
	}
	existing, err := q.R.ZRange(ctx, q.Key(), 0, -1).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	prefix := j.UserID + "|"
	for _, m := range existing {
		if strings.HasPrefix(m, prefix) {
			if err := q.R.ZRem(ctx, q.Key(), m).Err(); err != nil {
				return err
			}
		}
	}
	return q.R.ZAdd(ctx, q.Key(), redis.Z{Score: float64(dueAt.UnixMilli()), Member: member(j)}).Err()
}

// PendingUserIDs lists users with a scheduled check.
func (q *Queue) PendingUserIDs(ctx context.Context) ([]string, error) {
	if q == nil || q.R == nil {
		return nil, nil
	}
	out := map[string]bool{}
	for _, key := range []string{q.Key(), q.ProcessingKey()} {
		members, err := q.R.ZRange(ctx, key, 0, -1).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		for _, m := range members {
			if j, ok := parseMember(m); ok {
				out[j.UserID] = true
			}
		}
	}
	ids := make([]string, 0, len(out))
	for id := range out {
		ids = append(ids, id)
	}
	sortStrings(ids)
	return ids, nil
}

// popDue atomically moves due members into the processing set with a visibility timeout.
var popDueScript = redis.NewScript(`
local due = KEYS[1]
local processing = KEYS[2]
local now = tonumber(ARGV[1])
local visibility = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local members = redis.call('ZRANGEBYSCORE', due, '-inf', now, 'LIMIT', 0, limit)
for _, m in ipairs(members) do
  redis.call('ZREM', due, m)
  redis.call('ZADD', processing, now + visibility, m)
end
return members
`)

// PopDue claims up to limit due jobs.
func (q *Queue) PopDue(ctx context.Context, now time.Time, limit int, visibility time.Duration) ([]Job, error) {
	if q == nil || q.R == nil {
		return nil, nil
	}
	res, err := popDueScript.Run(ctx, q.R, []string{q.Key(), q.ProcessingKey()},
		now.UnixMilli(), visibility.Milliseconds(), limit).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	raw, _ := res.([]any)
	out := make([]Job, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			if j, ok := parseMember(s); ok {
				out = append(out, j)
			}
		}
	}
	return out, nil
}

// Ack removes a completed job from the processing set.
func (q *Queue) Ack(ctx context.Context, j Job) error {
	if q == nil || q.R == nil {
		return nil
	}
	return q.R.ZRem(ctx, q.ProcessingKey(), member(j)).Err()
}

// RequeueStale returns jobs whose visibility timeout lapsed to the due set.
func (q *Queue) RequeueStale(ctx context.Context, now time.Time) error {
	if q == nil || q.R == nil {
		return nil
	}
	members, err := q.R.ZRangeByScore(ctx, q.ProcessingKey(), &redis.ZRangeBy{
		Min: "-inf", Max: itoa(now.UnixMilli())}).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	for _, m := range members {
		if err := q.R.ZRem(ctx, q.ProcessingKey(), m).Err(); err != nil {
			return err
		}
		if err := q.R.ZAdd(ctx, q.Key(), redis.Z{Score: float64(now.UnixMilli()), Member: m}).Err(); err != nil {
			return err
		}
	}
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// EnqueueForLog mirrors lifelineQueue.enqueueLifelineForLog.
func EnqueueForLog(ctx context.Context, q *Queue, userID string, logDate time.Time, graceHours int, testDelayMS *int, now time.Time) error {
	checkDate := CheckDateFor(logDate)
	dueAt := DueAtFor(checkDate, graceHours)
	if testDelayMS != nil && *testDelayMS >= 0 {
		dueAt = now.Add(time.Duration(*testDelayMS) * time.Millisecond)
	}
	return q.Enqueue(ctx, Job{UserID: userID, CheckDate: checkDate}, dueAt)
}
