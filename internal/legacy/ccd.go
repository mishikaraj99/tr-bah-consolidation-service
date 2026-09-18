package legacy

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// CCDChannel is the Redis pub/sub channel api-server subscribes to (spec §6.4).
const CCDChannel = "ccd_update"

// CCD event types emitted by BAH.
const (
	CCDEventActivityLog  = "ACTIVITY_LOG"
	CCDEventCoinCredited = "COIN_CREDITED"
)

// RedisCCD publishes CCD_UPDATE events. Failures are logged, never fatal.
type RedisCCD struct {
	R   *redis.Client
	Log *slog.Logger
}

// Publish emits {tenantId, eventType, caseId, payload, emittedAt} on CCDChannel.
func (p *RedisCCD) Publish(ctx context.Context, tenantID, eventType, caseID string, payload map[string]any) {
	if p == nil || p.R == nil || caseID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"tenantId": tenantID, "eventType": eventType, "caseId": caseID, "payload": payload,
		"emittedAt": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}
	if err := p.R.Publish(ctx, CCDChannel, body).Err(); err != nil && p.Log != nil {
		p.Log.Warn("ccd publish failed", "eventType", eventType, "error", err.Error())
	}
}
