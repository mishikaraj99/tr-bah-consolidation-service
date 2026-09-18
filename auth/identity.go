// Package auth implements the per-tenant authentication strategies.
package auth

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Identity is the authenticated caller.
type Identity struct {
	UserID, CaseID, Email, FirstName, Phone, Gender string
	Roles                                           []string
	RawToken                                        string
	AppVersion                                      int
}

const localsKey = "identity"

// From returns the identity set by a Require* middleware (empty identity when none).
func From(c *fiber.Ctx) *Identity {
	if id, ok := c.Locals(localsKey).(*Identity); ok && id != nil {
		return id
	}
	return &Identity{}
}

func set(c *fiber.Ctx, id *Identity) { c.Locals(localsKey, id) }

// AppVersionOf mirrors `query.appVersion || query.version || header['x-app-version']` → Number, 0 when NaN.
func AppVersionOf(c *fiber.Ctx) int {
	for _, raw := range []string{c.Query("appVersion"), c.Query("version"), c.Get("x-app-version")} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return int(f)
		}
		return 0
	}
	return 0
}
