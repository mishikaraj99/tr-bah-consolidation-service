package routes_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/auth"
	"traya-bah-service/internal/common"
	"traya-bah-service/tenant"
)

// The route table is exercised through the real middleware stack with a stub tenant resolver, so
// auth, tenant gating and the error envelopes are covered without any datastore.
type stubResolver struct{ tenants map[string]*tenant.Tenant }

func (s stubResolver) Resolve(_ context.Context, id string) (*tenant.Tenant, error) {
	if t, ok := s.tenants[id]; ok {
		return t, nil
	}
	return nil, tenant.ErrUnknownTenant
}

func newApp(t *testing.T) (*fiber.App, *miniredis.Miniredis, *auth.Verifier) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	v := &auth.Verifier{Secret: []byte("s3cret"), Redis: rdb, V2Token: "v2tok", InternalToken: "int-tok"}
	res := stubResolver{tenants: map[string]*tenant.Tenant{
		"traya": {ID: "traya", Economies: map[tenant.Economy]bool{tenant.Legacy: true, tenant.Habit: true}, AuthMode: tenant.AuthJWT},
		"mool":  {ID: "mool", Economies: map[tenant.Economy]bool{tenant.LogEarn: true}, AuthMode: tenant.AuthGateway},
	}}
	app := fiber.New()
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })
	app.Use(tenant.Middleware(res, ""))

	jwtMW := v.RequireJWT()
	v2 := v.RequireV2Token()
	internal := v.RequireInternal()
	gateway := v.RequireGateway()
	legacyOnly := tenant.RequireEconomy(tenant.Legacy)
	logEarnOnly := tenant.RequireEconomy(tenant.LogEarn)

	// Representative routes: one per auth strategy and per envelope family.
	app.Get("/streakAndRewardBalance", legacyOnly, jwtMW, func(c *fiber.Ctx) error {
		return common.WriteError(c, common.FamilyLegacyErr, common.Internal("boom"))
	})
	app.Get("/bah/:userId/calendar", legacyOnly, jwtMW, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"identityUser": auth.From(c).UserID, "pathUser": c.Params("userId")})
	})
	app.Post("/activityLogForBAH", legacyOnly, jwtMW, func(c *fiber.Ctx) error {
		var body map[string]any
		_ = c.BodyParser(&body)
		if err := common.ValidateLogActivity(body); err != nil {
			return common.WriteError(c, common.FamilyLegacyMessage, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	app.Get("/rewardBalance/:caseId", legacyOnly, v2, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"rewardBalance": 0})
	})
	app.Post("/bah/scratch-card/habit-tracker-mint", internal, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true})
	})
	app.Get("/coinTransaction", legacyOnly, jwtMW, func(c *fiber.Ctx) error {
		return common.WriteError(c, common.FamilyCoinTxn, common.Internal("kaboom"))
	})
	le := app.Group("/consumers/logearn", logEarnOnly, gateway)
	le.Post("/log", func(c *fiber.Ctx) error {
		return common.WriteJSON(c, fiber.StatusCreated, fiber.Map{"success": true})
	})
	le.Post("/redeem", func(c *fiber.Ctx) error {
		return common.WriteError(c, common.FamilyLogEarn, common.BadRequest("Nothing to redeem"))
	})
	return app, mr, v
}

func token(t *testing.T, userID string) string {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"id": userID, "caseId": "c1"}).SignedString([]byte("s3cret"))
	require.NoError(t, err)
	return tok
}

func do(app *fiber.App, method, path string, hdr map[string]string, body string) (int, string) {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, _ := app.Test(req, 5000)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestHealthBypassesTenant(t *testing.T) {
	app, _, _ := newApp(t)
	code, body := do(app, "GET", "/health", nil, "")
	assert.Equal(t, 200, code)
	assert.Equal(t, "OK", body)
}

func TestTenantGating(t *testing.T) {
	app, _, _ := newApp(t)
	code, body := do(app, "GET", "/streakAndRewardBalance", nil, "")
	assert.Equal(t, 400, code)
	assert.JSONEq(t, `{"message":"Tenant ID header missing"}`, body)

	code, body = do(app, "GET", "/streakAndRewardBalance", map[string]string{"x-tenant-id": "kibo"}, "")
	assert.Equal(t, 400, code)
	assert.JSONEq(t, `{"message":"Tenant not found: kibo"}`, body)

	// mool does not run the legacy economy
	code, body = do(app, "GET", "/streakAndRewardBalance", map[string]string{"x-tenant-id": "mool"}, "")
	assert.Equal(t, 404, code)
	assert.JSONEq(t, `{"message":"Not available for tenant mool"}`, body)

	// traya does not run Log & Earn
	code, _ = do(app, "POST", "/consumers/logearn/log?customerId=c1", map[string]string{"x-tenant-id": "traya"}, "")
	assert.Equal(t, 404, code)
}

func TestAuthRequired(t *testing.T) {
	app, mr, _ := newApp(t)
	traya := map[string]string{"x-tenant-id": "traya"}
	code, body := do(app, "GET", "/streakAndRewardBalance", traya, "")
	assert.Equal(t, 401, code)
	assert.JSONEq(t, `{"message":"No authorization header provided."}`, body)

	tok := token(t, "u1")
	code, _ = do(app, "GET", "/streakAndRewardBalance", map[string]string{"x-tenant-id": "traya", "Authorization": "Bearer " + tok}, "")
	assert.Equal(t, 401, code, "redis login status missing")

	mr.Set("user!u1login!status", "1")
	code, body = do(app, "GET", "/streakAndRewardBalance", map[string]string{"x-tenant-id": "traya", "Authorization": "Bearer " + tok}, "")
	assert.Equal(t, 500, code)
	assert.JSONEq(t, `{"err":{"message":"boom"}}`, body, "legacy 500 envelope")
}

func TestCalendarUsesTokenIdentity(t *testing.T) {
	app, mr, _ := newApp(t)
	mr.Set("user!u1login!status", "1")
	hdr := map[string]string{"x-tenant-id": "traya", "Authorization": "Bearer " + token(t, "u1")}
	code, body := do(app, "GET", "/bah/someone-else/calendar?date=2026-09-18", hdr, "")
	assert.Equal(t, 200, code)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	assert.Equal(t, "u1", out["identityUser"], "identity comes from the token, not the path (IDOR fix)")
	assert.Equal(t, "someone-else", out["pathUser"])
}

func TestEnvelopeFamilies(t *testing.T) {
	app, mr, _ := newApp(t)
	mr.Set("user!u1login!status", "1")
	hdr := map[string]string{"x-tenant-id": "traya", "Authorization": "Bearer " + token(t, "u1")}

	// validation → {message} with 400
	code, body := do(app, "POST", "/activityLogForBAH", hdr, `{}`)
	assert.Equal(t, 400, code)
	assert.JSONEq(t, `{"message":"Validation error: \"isLogForToday\" is required"}`, body)

	// coinTransaction → {err: message} with 500
	code, body = do(app, "GET", "/coinTransaction", hdr, "")
	assert.Equal(t, 500, code)
	assert.JSONEq(t, `{"err":"kaboom"}`, body)

	// logearn → {message,statusCode,timestamp,path}
	code, body = do(app, "POST", "/consumers/logearn/redeem?customerId=c1", map[string]string{"x-tenant-id": "mool"}, `{"subtotal":10}`)
	assert.Equal(t, 400, code)
	var le map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &le))
	assert.Equal(t, "Nothing to redeem", le["message"])
	assert.Equal(t, float64(400), le["statusCode"])
	assert.Contains(t, le, "timestamp")
	assert.Contains(t, le["path"], "/consumers/logearn/redeem")
}

func TestV2AndInternalTokens(t *testing.T) {
	app, _, _ := newApp(t)
	traya := map[string]string{"x-tenant-id": "traya"}
	code, _ := do(app, "GET", "/rewardBalance/c1", traya, "")
	assert.Equal(t, 401, code)
	code, _ = do(app, "GET", "/rewardBalance/c1", map[string]string{"x-tenant-id": "traya", "x-access-token": "Bearer v2tok"}, "")
	assert.Equal(t, 200, code)

	code, _ = do(app, "POST", "/bah/scratch-card/habit-tracker-mint", traya, `{}`)
	assert.Equal(t, 401, code)
	code, _ = do(app, "POST", "/bah/scratch-card/habit-tracker-mint",
		map[string]string{"x-tenant-id": "traya", "x-internal-token": "int-tok"}, `{}`)
	assert.Equal(t, 200, code)
}

func TestLogEarnGatewayIdentityAnd201(t *testing.T) {
	app, _, _ := newApp(t)
	mool := map[string]string{"x-tenant-id": "mool"}
	code, body := do(app, "POST", "/consumers/logearn/log", mool, "")
	assert.Equal(t, 400, code)
	assert.Contains(t, body, "customerId is required")

	code, _ = do(app, "POST", "/consumers/logearn/log?customerId=cust1", mool, "")
	assert.Equal(t, 201, code, "logearn POSTs answer 201")

	code, _ = do(app, "POST", "/consumers/logearn/log",
		map[string]string{"x-tenant-id": "mool", "x-user-info": `{"customer_id":"cust1"}`}, "")
	assert.Equal(t, 201, code)
}
