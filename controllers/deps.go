// Package controllers adapts HTTP requests to the economy services.
package controllers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	"traya-bah-service/auth"
	"traya-bah-service/internal/common"
	"traya-bah-service/internal/habit"
	"traya-bah-service/internal/habit/lifeline"
	"traya-bah-service/internal/legacy"
	"traya-bah-service/internal/logearn"
	"traya-bah-service/internal/orders"
	mongorepo "traya-bah-service/repositories/mongo"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/setup"
	"traya-bah-service/tenant"
)

// Deps are the process-wide collaborators the controllers build per-request services from.
type Deps struct {
	Cfg        *setup.Config
	Registry   *tenant.Registry
	Verifier   *auth.Verifier
	Master     *mongo.Client
	Redis      *redis.Client
	HTTP       *setup.HTTPClients
	TrayaPG    *pgxpool.Pool
	Clock      common.Clock
	Log        *slog.Logger
	Dispatcher *legacy.Dispatcher
	CCD        legacy.CCDPublisher
	Config     *orders.ConfigServiceClient
	TROrders   *orders.TROrderClient
	LogEarnCfg *logearn.ConfigClient
	Shopflo    *legacy.ShopfloClient
}

func (d *Deps) store(c *fiber.Ctx) *mongorepo.Store {
	return mongorepo.NewStore(tenant.From(c).Mongo, d.Clock)
}

func (d *Deps) trayaPG(c *fiber.Ctx) *pgrepo.TrayaStore {
	t := tenant.From(c)
	pool := d.TrayaPG
	if t != nil && t.PG != nil {
		pool = t.PG
	}
	return &pgrepo.TrayaStore{Pool: pool}
}

// legacySvc builds the legacy economy for the request's tenant.
func (d *Deps) legacySvc(c *fiber.Ctx) *legacy.Service {
	t := tenant.From(c)
	return legacy.New(t.ID, legacy.Deps{
		Store: d.store(c), PG: d.trayaPG(c), Redis: d.Redis, Clock: d.Clock, Cfg: d.Cfg, HTTP: d.HTTP,
		ConfigClient: d.Config, Shopflo: d.Shopflo, Habit: d.habitSvc(c), Lifeline: d.lifelineEnqueuer(c),
		CCD: d.CCD, Dispatcher: d.Dispatcher, Log: d.Log,
	})
}

// habitSvc builds the v85 economy for the request's tenant.
func (d *Deps) habitSvc(c *fiber.Ctx) *habit.Service {
	t := tenant.From(c)
	return habit.New(t.ID, habit.Deps{
		Store: d.store(c), PG: d.trayaPG(c), Redis: d.Redis, Clock: d.Clock,
		Cfg: d.Cfg, HTTP: d.HTTP, Shopflo: d.Shopflo, Log: d.Log,
	})
}

// logearnSvc builds the Log & Earn economy for the request's tenant.
func (d *Deps) logearnSvc(c *fiber.Ctx) *logearn.Service {
	t := tenant.From(c)
	return logearn.New(t.ID, logearn.Deps{
		Store:     pgrepo.NewLogEarnStore(t.PG, t.PGRead),
		Customers: &pgrepo.CustomerStore{Pool: t.PG},
		Orders:    d.TROrders, Config: d.LogEarnCfg, Clock: d.Clock, Cfg: d.Cfg, Log: d.Log,
	})
}

// lifelineEnqueuer schedules lifeline checks for the request's tenant.
func (d *Deps) lifelineEnqueuer(c *fiber.Ctx) legacy.LifelineEnqueuer {
	t := tenant.From(c)
	return &lifelineEnqueuer{
		queue: &lifeline.Queue{R: d.Redis, TenantID: t.ID},
		grace: d.Cfg.LifelineGraceHours, testDelay: d.Cfg.LifelineTestDelayMS, clock: d.Clock,
	}
}

type lifelineEnqueuer struct {
	queue     *lifeline.Queue
	grace     int
	testDelay *int
	clock     common.Clock
}

func (l *lifelineEnqueuer) EnqueueForLog(ctx context.Context, userID string, logDate time.Time) error {
	return lifeline.EnqueueForLog(ctx, l.queue, userID, logDate, l.grace, l.testDelay, l.clock.Now())
}

// queryInt reads an int query param with a default.
func queryInt(c *fiber.Ctx, key string, def int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// optionalIntQuery returns nil when the param is absent.
func optionalIntQuery(c *fiber.Ctx, key string) *int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

// identity is the authenticated caller.
func identity(c *fiber.Ctx) *auth.Identity { return auth.From(c) }
