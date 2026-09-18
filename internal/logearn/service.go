package logearn

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/setup"
)

// RewardsConfig is the tenant's z-tiers and bonuses.
type RewardsConfig struct {
	ZTiers  []ZTier
	Bonuses Bonuses
}

// ConfigClient fetches the rewards config from CMS, cached per tenant.
type ConfigClient struct {
	HTTP    *http.Client
	BaseURL string
	TTL     time.Duration

	mu    sync.RWMutex
	cache map[string]configEntry
}

type configEntry struct {
	cfg     RewardsConfig
	expires time.Time
}

// Get returns the tenant config, falling back to defaults on any failure.
func (c *ConfigClient) Get(ctx context.Context, tenantID string) RewardsConfig {
	def := RewardsConfig{ZTiers: DefaultZTiers, Bonuses: DefaultBonuses}
	if c == nil || c.BaseURL == "" {
		return def
	}
	ttl := c.TTL
	if ttl == 0 {
		ttl = 10 * time.Minute
	}
	c.mu.RLock()
	entry, ok := c.cache[tenantID]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.cfg
	}
	cfg, err := c.fetch(ctx, tenantID)
	if err != nil {
		if ok {
			return entry.cfg
		}
		return def
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]configEntry{}
	}
	c.cache[tenantID] = configEntry{cfg: cfg, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
	return cfg
}

func (c *ConfigClient) fetch(ctx context.Context, tenantID string) (RewardsConfig, error) {
	out := RewardsConfig{ZTiers: DefaultZTiers, Bonuses: DefaultBonuses}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(c.BaseURL, "/")+"/api/carestack/config/"+tenantID, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("x-tenant-id", tenantID)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return out, err
	}
	var payload struct {
		Data struct {
			Rewards *struct {
				ZTiers  []ZTier  `json:"zTiers"`
				Bonuses *Bonuses `json:"bonuses"`
			} `json:"rewards"`
		} `json:"data"`
		Rewards *struct {
			ZTiers  []ZTier  `json:"zTiers"`
			Bonuses *Bonuses `json:"bonuses"`
		} `json:"rewards"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return out, err
	}
	rewards := payload.Data.Rewards
	if rewards == nil {
		rewards = payload.Rewards
	}
	if rewards == nil {
		return out, nil
	}
	if len(rewards.ZTiers) > 0 {
		out.ZTiers = rewards.ZTiers
	}
	if rewards.Bonuses != nil {
		b := DefaultBonuses
		if rewards.Bonuses.FirstEver != 0 {
			b.FirstEver = rewards.Bonuses.FirstEver
		}
		if rewards.Bonuses.DayThree != 0 {
			b.DayThree = rewards.Bonuses.DayThree
		}
		if rewards.Bonuses.ReorderWelcome != 0 {
			b.ReorderWelcome = rewards.Bonuses.ReorderWelcome
		}
		if rewards.Bonuses.MilestoneEvery != 0 {
			b.MilestoneEvery = rewards.Bonuses.MilestoneEvery
		}
		if rewards.Bonuses.MilestoneAmount != 0 {
			b.MilestoneAmount = rewards.Bonuses.MilestoneAmount
		}
		out.Bonuses = b
	}
	return out, nil
}

// Deps are the collaborators a logearn Service needs.
type Deps struct {
	Store     *pgrepo.LogEarnStore
	Customers *pgrepo.CustomerStore
	Orders    *orders.TROrderClient
	Config    *ConfigClient
	Clock     common.Clock
	Cfg       *setup.Config
	Log       *slog.Logger
}

// Service is the per-tenant Log & Earn economy.
type Service struct {
	Deps
	TenantID string
}

// New builds a logearn Service.
func New(tenantID string, d Deps) *Service {
	if d.Clock == nil {
		d.Clock = common.RealClock{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{Deps: d, TenantID: tenantID}
}

func (s *Service) now() time.Time { return s.Clock.Now() }
