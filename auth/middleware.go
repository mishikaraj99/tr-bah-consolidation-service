package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"traya-bah-service/internal/common"
)

// Verifier holds the secrets each strategy needs.
type Verifier struct {
	Secret        []byte
	Redis         *redis.Client
	V2Token       string
	InternalToken string
	AdminGuard    bool
}

const (
	msgNoAuthHeader = "No authorization header provided."
	msgInvalidToken = "Invalid or expired token."
)

// RequireJWT mirrors api-server authenticateJwt: Bearer HS256 + Redis login-status gate.
func (v *Verifier) RequireJWT() fiber.Handler {
	return func(c *fiber.Ctx) error {
		h := c.Get("Authorization")
		if h == "" {
			return common.WriteError(c, common.FamilyPlain, common.Unauthorized(msgNoAuthHeader))
		}
		scheme, token, _ := strings.Cut(h, " ")
		if !strings.EqualFold(scheme, "Bearer") || token == "" {
			return common.WriteError(c, common.FamilyPlain, common.Unauthorized(msgInvalidToken))
		}
		claims := jwt.MapClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return v.Secret, nil
		})
		if err != nil || !parsed.Valid {
			return common.WriteError(c, common.FamilyPlain, common.Unauthorized(msgInvalidToken))
		}
		id := &Identity{
			UserID: str(claims["id"]), CaseID: str(claims["caseId"]), Email: str(claims["email"]),
			FirstName: str(claims["first_name"]), Phone: str(claims["phone_number"]), Gender: str(claims["gender"]),
			RawToken: token, AppVersion: AppVersionOf(c),
		}
		if roles, ok := claims["roles"].([]any); ok {
			for _, r := range roles {
				id.Roles = append(id.Roles, str(r))
			}
		}
		if id.UserID == "" {
			return common.WriteError(c, common.FamilyPlain, common.Unauthorized(msgInvalidToken))
		}
		if v.Redis != nil {
			ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
			defer cancel()
			status, err := v.Redis.Get(ctx, "user!"+id.UserID+"login!status").Result()
			if err != nil || status == "" {
				return common.WriteError(c, common.FamilyPlain, common.Unauthorized(msgInvalidToken))
			}
		}
		set(c, id)
		return c.Next()
	}
}

// RequireV2Token mirrors checkTokenForV2FormData.
func (v *Verifier) RequireV2Token() fiber.Handler {
	return func(c *fiber.Ctx) error {
		want := "Bearer " + v.V2Token
		for _, got := range []string{c.Get("x-access-token"), c.Get("Authorization")} {
			if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
				set(c, &Identity{AppVersion: AppVersionOf(c)})
				return c.Next()
			}
		}
		return common.WriteError(c, common.FamilyPlain, common.Unauthorized("Unauthorized"))
	}
}

// RequireInternal gates server-to-server routes with x-internal-token.
func (v *Verifier) RequireInternal() fiber.Handler {
	return func(c *fiber.Ctx) error {
		got := c.Get("x-internal-token")
		if v.InternalToken == "" || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(v.InternalToken)) != 1 {
			return common.WriteError(c, common.FamilyPlain, common.Unauthorized("Unauthorized"))
		}
		set(c, &Identity{AppVersion: AppVersionOf(c)})
		return c.Next()
	}
}

// RequireGateway trusts tr-consumer-api-gateway headers (x-user-info) with a customerId query fallback.
func (v *Verifier) RequireGateway() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := &Identity{AppVersion: AppVersionOf(c)}
		if raw := c.Get("x-user-info"); raw != "" {
			var info map[string]any
			if json.Unmarshal([]byte(raw), &info) == nil {
				id.UserID = str(info["customer_id"])
				id.CaseID = str(info["case_id"])
				id.Gender = str(info["gender"])
				id.FirstName = str(info["first_name"])
			}
		}
		if q := strings.TrimSpace(c.Query("customerId")); q != "" {
			id.UserID = q
		}
		if id.UserID == "" {
			return common.WriteError(c, common.FamilyLogEarn, common.BadRequest("customerId is required"))
		}
		set(c, id)
		return c.Next()
	}
}

// RequireAdmin mirrors isUserAdminOrSuperAdmin when AdminGuard is on; otherwise a no-op (api-server does not guard these today).
func (v *Verifier) RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !v.AdminGuard {
			return c.Next()
		}
		id := From(c)
		if len(id.Roles) > 0 {
			switch id.Roles[0] {
			case "ADMIN", "SUPER_ADMIN", "TEAM_LEAD":
				return c.Next()
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"Error": "Only admin and super admin are allowed"})
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
