package tenant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
	"traya-bah-service/setup"
)

// ErrUnknownTenant is returned for ids missing from master.tenant.
var ErrUnknownTenant = errors.New("tenant not found")

// Resolver is what Middleware needs; Registry implements it.
type Resolver interface {
	Resolve(ctx context.Context, id string) (*Tenant, error)
}

// Registry resolves tenants from the master Mongo database and caches datastores.
type Registry struct {
	cfg     *setup.Config
	master  *mongo.Client
	trayaPG *pgxpool.Pool
	log     *slog.Logger

	mu       sync.RWMutex
	tenants  map[string]*Tenant
	negative map[string]time.Time
	indexed  sync.Map // db name → *sync.Once
	// hooks for tests
	lookup func(ctx context.Context, id string) (*models.Tenant, error)
	pgDial func(ctx context.Context, c setup.PGConfig) (*pgxpool.Pool, error)
}

// NewRegistry builds a registry. trayaPG may be nil when the traya tenant is not served.
func NewRegistry(cfg *setup.Config, master *mongo.Client, trayaPG *pgxpool.Pool, log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	r := &Registry{cfg: cfg, master: master, trayaPG: trayaPG, log: log, tenants: map[string]*Tenant{}, negative: map[string]time.Time{}}
	r.lookup = r.lookupMaster
	r.pgDial = setup.ConnectPG
	return r
}

func (r *Registry) lookupMaster(ctx context.Context, id string) (*models.Tenant, error) {
	var t models.Tenant
	err := r.master.Database("master").Collection("tenant").FindOne(ctx, bson.M{"tenant_id": id}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrUnknownTenant
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Resolve returns the cached tenant or builds it from the master registry.
func (r *Registry) Resolve(ctx context.Context, id string) (*Tenant, error) {
	r.mu.RLock()
	t, ok := r.tenants[id]
	neg := r.negative[id]
	r.mu.RUnlock()
	if ok {
		return t, nil
	}
	if !neg.IsZero() && time.Since(neg) < time.Minute {
		return nil, ErrUnknownTenant
	}
	econ, configured := r.cfg.TenantEconomies[id]
	if !configured {
		r.remember(id, nil)
		return nil, ErrUnknownTenant
	}
	rec, err := r.lookup(ctx, id)
	if err != nil {
		if errors.Is(err, ErrUnknownTenant) {
			r.remember(id, nil)
		}
		return nil, err
	}
	t = &Tenant{ID: rec.TenantID, Name: rec.TenantName, Economies: map[Economy]bool{}, AuthMode: AuthGateway}
	for _, e := range econ {
		t.Economies[Economy(e)] = true
	}
	if t.Has(Legacy) || t.Has(Habit) {
		t.AuthMode = AuthJWT
	}
	if r.master != nil {
		t.Mongo = r.master.Database(setup.TenantMongoDBName(r.cfg, id))
		r.ensureIndexes(t.Mongo)
	}
	if id == "traya" {
		t.PG = r.trayaPG
	} else if r.cfg.TRPGWrite.Host != "" {
		wc := r.cfg.TRPGWrite
		wc.Database = id + "_" + r.cfg.TRPGSuffix
		pool, err := r.pgDial(ctx, wc)
		if err != nil {
			return nil, fmt.Errorf("tenant %s postgres: %w", id, err)
		}
		t.PG = pool
		if r.cfg.TRPGRead != nil {
			rc := *r.cfg.TRPGRead
			rc.Database = wc.Database
			if rp, err := r.pgDial(ctx, rc); err == nil {
				t.PGRead = rp
			} else {
				r.log.Warn("tenant read replica unavailable, using writer", "tenant", id, "error", err.Error())
			}
		}
	}
	if t.PGRead == nil {
		t.PGRead = t.PG
	}
	r.remember(id, t)
	return t, nil
}

func (r *Registry) remember(id string, t *Tenant) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t == nil {
		r.negative[id] = time.Now()
		return
	}
	if existing, ok := r.tenants[id]; ok {
		// another goroutine won the race; keep the first
		_ = existing
		return
	}
	r.tenants[id] = t
}

// ensureIndexes runs the index bootstrap once per database, off the request path.
func (r *Registry) ensureIndexes(db *mongo.Database) {
	once, _ := r.indexed.LoadOrStore(db.Name(), &sync.Once{})
	once.(*sync.Once).Do(func() {
		go func() {
			if err := mongorepo.EnsureIndexes(context.Background(), db); err != nil {
				r.log.Error("index bootstrap failed", "db", db.Name(), "error", err.Error())
			}
		}()
	})
}

// Close releases pooled tenant connections.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.tenants {
		if id != "traya" && t.PG != nil {
			t.PG.Close()
			if t.PGRead != nil && t.PGRead != t.PG {
				t.PGRead.Close()
			}
		}
	}
}
