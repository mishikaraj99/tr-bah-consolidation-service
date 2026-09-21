package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/swagger"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"traya-bah-service/auth"
	"traya-bah-service/controllers"
	_ "traya-bah-service/docs"
	"traya-bah-service/internal/common"
	"traya-bah-service/internal/habit/lifeline"
	"traya-bah-service/internal/legacy"
	"traya-bah-service/internal/logearn"
	"traya-bah-service/internal/orders"
	mongorepo "traya-bah-service/repositories/mongo"
	pgrepo "traya-bah-service/repositories/pg"
	"traya-bah-service/routes"
	"traya-bah-service/setup"
	"traya-bah-service/tenant"
)

// @title			Traya BAH Service
// @version		1.0
// @description	Multi-tenant Build-A-Habit service (traya legacy + v85, mool/acne Log & Earn)
func main() {
	time.Local = time.UTC
	cfg := setup.MustLoad()
	log := setup.NewLogger(cfg.Environment)
	common.LegacyRedisFallback = cfg.LegacyRedisFallback
	ctx := context.Background()

	master, err := setup.ConnectMongo(ctx, cfg.MongoURI)
	if err != nil {
		log.Error("mongo connect failed", "error", err.Error())
		os.Exit(1)
	}
	redisClient := setup.ConnectRedis(cfg)
	httpClients := setup.NewHTTPClients(cfg)

	var trayaPG *pgxpool.Pool
	if pool, err := setup.ConnectPG(ctx, cfg.TrayaPG); err != nil {
		log.Warn("traya postgres unavailable at boot; legacy routes will fail until it recovers", "error", err.Error())
	} else {
		trayaPG = pool
	}

	registry := tenant.NewRegistry(cfg, master, trayaPG, log)
	verifier := &auth.Verifier{Secret: []byte(cfg.JWTSecret), Redis: redisClient,
		V2Token: cfg.V2FormDataToken, InternalToken: cfg.InternalServiceToken, AdminGuard: cfg.CRMAdminGuard}
	dispatcher := legacy.NewDispatcher(cfg.LegacyWorkers, 1024, log)

	deps := &controllers.Deps{
		Cfg: cfg, Registry: registry, Verifier: verifier, Master: master, Redis: redisClient,
		HTTP: httpClients, TrayaPG: trayaPG, Clock: common.RealClock{}, Log: log,
		Dispatcher: dispatcher, CCD: &legacy.RedisCCD{R: redisClient, Log: log},
		Config:     &orders.ConfigServiceClient{HTTP: httpClients.ConfigService, BaseURL: cfg.ConfigServiceBaseURL},
		TROrders:   &orders.TROrderClient{HTTP: httpClients.TROrderService, BaseURL: cfg.TROrderServiceBaseURL},
		LogEarnCfg: &logearn.ConfigClient{HTTP: httpClients.TRCMS, BaseURL: cfg.TRCMSServiceBaseURL},
		Shopflo: &legacy.ShopfloClient{HTTP: httpClients.Shopflo, Endpoint: cfg.ShopfloEndpoint,
			IssuerID: cfg.ShopfloIssuerID, MerchantID: cfg.ShopfloMerchantID, APIKey: cfg.ShopfloAPIKey, Log: log},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second})
	app.Use(recover.New())
	app.Use(requestid.New(requestid.Config{Header: "x-trace-id"}))
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${ip} | ${reqHeader:x-trace-id} | ${reqHeader:x-tenant-id} | ${status} | ${method} | ${path} | ${latency} | ${error}\n",
		TimeFormat: time.RFC3339,
	}))
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })
	app.Get("/api/docs/*", swagger.HandlerDefault)
	routes.Setup(app, deps)

	workerCtx, stopWorkers := context.WithCancel(ctx)
	if cfg.LifelineWorkerEnabled {
		startLifelineWorkers(workerCtx, cfg, registry, deps, redisClient, log)
	}

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Error("listen failed", "error", err.Error())
			os.Exit(1)
		}
	}()
	log.Info("tr-bah-service listening", "port", cfg.Port, "environment", cfg.Environment)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	stopWorkers()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(shutdownCtx)
	dispatcher.Shutdown(shutdownCtx)
	registry.Close()
	if trayaPG != nil {
		trayaPG.Close()
	}
	_ = redisClient.Close()
	_ = master.Disconnect(shutdownCtx)
}

// startLifelineWorkers runs the lifeline poller and daily reconcile for every tenant that has the habit economy.
func startLifelineWorkers(ctx context.Context, cfg *setup.Config, registry *tenant.Registry,
	deps *controllers.Deps, redisClient *redis.Client, log *slog.Logger) {
	for tenantID, economies := range cfg.TenantEconomies {
		hasHabit := false
		for _, e := range economies {
			if tenant.Economy(e) == tenant.Habit {
				hasHabit = true
			}
		}
		if !hasHabit {
			continue
		}
		t, err := registry.Resolve(ctx, tenantID)
		if err != nil {
			log.Error("lifeline worker skipped: tenant unresolved", "tenant", tenantID, "error", err.Error())
			continue
		}
		store := mongorepo.NewStore(t.Mongo, deps.Clock)
		pg := &pgrepo.TrayaStore{Pool: t.PG}
		habitDeps := &lifeline.Deps{
			Store: store, Clock: deps.Clock, TenantID: tenantID, GraceHours: cfg.LifelineGraceHours,
			TestDelayMS: cfg.LifelineTestDelayMS, Log: log,
			Orders: func(ctx context.Context, userID string) ([]orders.Order, error) {
				return pg.NonVoidOrdersByUser(ctx, userID)
			},
		}
		worker := &lifeline.Worker{
			Q: &lifeline.Queue{R: redisClient, TenantID: tenantID}, D: habitDeps,
			Concurrency: 5, PollEvery: time.Duration(cfg.LifelinePollMS) * time.Millisecond,
		}
		go worker.Run(ctx)
		go worker.RunReconcileDaily(ctx)
		log.Info("lifeline worker started", "tenant", tenantID)
	}
}
