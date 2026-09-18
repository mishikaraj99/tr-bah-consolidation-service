// Package tenant resolves x-tenant-id into per-tenant datastores and enabled economies.
package tenant

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/mongo"
)

// Economy is a BAH implementation a tenant may run.
type Economy string

const (
	Legacy  Economy = "legacy"
	Habit   Economy = "habit"
	LogEarn Economy = "logearn"
)

// AuthMode is how a tenant's callers are authenticated.
type AuthMode string

const (
	AuthJWT     AuthMode = "jwt"
	AuthGateway AuthMode = "gateway"
)

// Header carries the tenant id on every request.
const Header = "x-tenant-id"

// Tenant is the resolved per-request tenant context.
type Tenant struct {
	ID, Name  string
	Economies map[Economy]bool
	AuthMode  AuthMode
	Mongo     *mongo.Database
	PG        *pgxpool.Pool
	PGRead    *pgxpool.Pool
}

// Has reports whether the economy is enabled for the tenant.
func (t *Tenant) Has(e Economy) bool { return t != nil && t.Economies[e] }

const localsKey = "tenant"

// From returns the tenant stored by Middleware (nil before it ran).
func From(c *fiber.Ctx) *Tenant {
	t, _ := c.Locals(localsKey).(*Tenant)
	return t
}
