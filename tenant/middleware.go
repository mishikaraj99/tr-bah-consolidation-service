package tenant

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"traya-bah-service/internal/common"
)

// Middleware resolves x-tenant-id (or defaultTenant) into c.Locals("tenant").
func Middleware(r Resolver, defaultTenant string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Get(Header))
		if id == "" {
			id = strings.TrimSpace(c.Query("tenantId"))
		}
		if id == "" {
			id = defaultTenant
		}
		if id == "" {
			return common.WriteError(c, common.FamilyPlain, common.BadRequest("Tenant ID header missing"))
		}
		id = strings.ToLower(id)
		t, err := r.Resolve(c.UserContext(), id)
		if err != nil {
			if errors.Is(err, ErrUnknownTenant) {
				return common.WriteError(c, common.FamilyPlain, common.BadRequest("Tenant not found: "+id))
			}
			return common.WriteError(c, common.FamilyPlain, common.Internal("Failed to connect to tenant database: "+err.Error()))
		}
		c.Locals(localsKey, t)
		return c.Next()
	}
}

// RequireEconomy rejects tenants that do not run the economy.
func RequireEconomy(e Economy) fiber.Handler {
	return func(c *fiber.Ctx) error {
		t := From(c)
		if t == nil || !t.Has(e) {
			id := ""
			if t != nil {
				id = t.ID
			}
			return common.WriteError(c, common.FamilyPlain, common.NotFound("Not available for tenant "+id))
		}
		return c.Next()
	}
}
