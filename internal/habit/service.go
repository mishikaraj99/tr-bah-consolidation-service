package habit

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/legacy"
	"traya-bah-service/internal/orders"
	mongorepo "traya-bah-service/repositories/mongo"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/setup"
)

// OrderSource supplies orders and identity for the habit economy.
type OrderSource interface {
	NonVoidOrdersByUser(ctx context.Context, userID string) ([]orders.Order, error)
	NonVoidOrdersByCase(ctx context.Context, caseID string) ([]orders.Order, error)
	UserCaseByUserID(ctx context.Context, userID string) (*pgrepo.UserCase, error)
	UserCaseByCaseID(ctx context.Context, caseID string) (*pgrepo.UserCase, error)
}

// Deps are the collaborators a habit Service needs.
type Deps struct {
	Store   *mongorepo.Store
	PG      OrderSource
	Redis   *redis.Client
	Clock   common.Clock
	Cfg     *setup.Config
	HTTP    *setup.HTTPClients
	Shopflo *legacy.ShopfloClient
	Log     *slog.Logger
}

// Service is the per-tenant v85 economy.
type Service struct {
	Deps
	TenantID string
}

// New builds a habit Service.
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

func (s *Service) httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}

func (s *Service) scratchCardsEnabled() bool {
	return s.Cfg == nil || s.Cfg.ScratchCardsEnabled
}
