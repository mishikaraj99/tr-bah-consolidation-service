package orders

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ConfigServiceClient reads static content (the old→active variant map) from tr-config-service.
type ConfigServiceClient struct {
	HTTP    *http.Client
	BaseURL string
	TTL     time.Duration

	mu    sync.RWMutex
	cache map[string]variantCacheEntry
}

type variantCacheEntry struct {
	m       map[string]map[string]any
	expires time.Time
}

// OldToActiveVariantKey is the static-content config id.
const OldToActiveVariantKey = "OLD_TO_ACTIVE_VARIANT_IDS_MAP"

// OldToActiveVariantMap fetches (and caches for TTL, default 10m) the variant map.
// Any failure returns the last good value, else an empty map — never an error to the caller.
func (c *ConfigServiceClient) OldToActiveVariantMap(ctx context.Context, tenantID string) map[string]map[string]any {
	ttl := c.TTL
	if ttl == 0 {
		ttl = 10 * time.Minute
	}
	c.mu.RLock()
	entry, ok := c.cache[tenantID]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.m
	}
	fetched, err := c.fetch(ctx, tenantID)
	if err != nil {
		if ok {
			return entry.m
		}
		return map[string]map[string]any{}
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]variantCacheEntry{}
	}
	c.cache[tenantID] = variantCacheEntry{m: fetched, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
	return fetched
}

func (c *ConfigServiceClient) fetch(ctx context.Context, tenantID string) (map[string]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(c.BaseURL, "/")+"/static-content/data/"+OldToActiveVariantKey, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-tenant-id", tenantID)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Data == nil {
		return map[string]map[string]any{}, nil
	}
	return wrapper.Data, nil
}

// ActiveVariantID mirrors getActiveVariantId: map[id].associatedTo || VARIANT_ID_MAPPING[id] || id.
func ActiveVariantID(id int64, m map[string]map[string]any) int64 {
	key := strconv.FormatInt(id, 10)
	if entry, ok := m[key]; ok {
		switch v := entry["associatedTo"].(type) {
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				return n
			}
		case float64:
			if int64(v) > 0 {
				return int64(v)
			}
		}
	}
	if mapped, ok := VariantIDMapping[id]; ok {
		return mapped
	}
	return id
}
