package auth

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/common"
	"traya-bah-service/tenant"
)

func call(app *fiber.App, path string, hdr map[string]string) (int, string) {
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, _ := app.Test(req)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRequireJWT(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	v := &Verifier{Secret: []byte("s3cret"), Redis: rdb}
	app := fiber.New()
	app.Get("/", v.RequireJWT(), func(c *fiber.Ctx) error {
		id := From(c)
		return c.JSON(fiber.Map{"u": id.UserID, "case": id.CaseID, "v": id.AppVersion, "roles": id.Roles})
	})
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"id": "u1", "caseId": "c1", "roles": []any{"ADMIN"}}).SignedString([]byte("s3cret"))

	code, body := call(app, "/", nil)
	assert.Equal(t, 401, code)
	assert.JSONEq(t, `{"message":"No authorization header provided."}`, body)

	code, body = call(app, "/", map[string]string{"Authorization": "Bearer " + tok})
	assert.Equal(t, 401, code, "login status missing in redis")
	assert.JSONEq(t, `{"message":"Invalid or expired token."}`, body)

	mr.Set("user!u1login!status", "1")
	code, body = call(app, "/?appVersion=83", map[string]string{"Authorization": "Bearer " + tok})
	assert.Equal(t, 200, code)
	assert.JSONEq(t, `{"u":"u1","case":"c1","v":83,"roles":["ADMIN"]}`, body)

	bad, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"id": "u1"}).SignedString([]byte("wrong"))
	code, _ = call(app, "/", map[string]string{"Authorization": "Bearer " + bad})
	assert.Equal(t, 401, code)
}

func TestV2InternalGatewayAdmin(t *testing.T) {
	v := &Verifier{V2Token: "v2tok", InternalToken: "internal", AdminGuard: true}
	app := fiber.New()
	app.Get("/v2", v.RequireV2Token(), func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/int", v.RequireInternal(), func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/gw", v.RequireGateway(), func(c *fiber.Ctx) error { return c.SendString(From(c).UserID + "|" + From(c).Gender) })
	app.Get("/admin", func(c *fiber.Ctx) error { set(c, &Identity{Roles: []string{"AGENT"}}); return c.Next() }, v.RequireAdmin(), func(c *fiber.Ctx) error { return c.SendString("ok") })

	code, _ := call(app, "/v2", map[string]string{"x-access-token": "Bearer v2tok"})
	assert.Equal(t, 200, code)
	code, _ = call(app, "/v2", map[string]string{"Authorization": "Bearer v2tok"})
	assert.Equal(t, 200, code)
	code, _ = call(app, "/v2", map[string]string{"Authorization": "Bearer nope"})
	assert.Equal(t, 401, code)

	code, _ = call(app, "/int", map[string]string{"x-internal-token": "internal"})
	assert.Equal(t, 200, code)
	code, _ = call(app, "/int", nil)
	assert.Equal(t, 401, code)

	code, body := call(app, "/gw", map[string]string{"x-user-info": `{"customer_id":"cust1","gender":"F"}`})
	assert.Equal(t, 200, code)
	assert.Equal(t, "cust1|F", body)
	code, body = call(app, "/gw?customerId=q1", nil)
	assert.Equal(t, 200, code)
	assert.Equal(t, "q1|", body)
	code, body = call(app, "/gw", nil)
	assert.Equal(t, 400, code)
	assert.Contains(t, body, `"message":"customerId is required"`)
	assert.Contains(t, body, `"statusCode":400`)

	code, body = call(app, "/admin", nil)
	assert.Equal(t, 403, code)
	assert.JSONEq(t, `{"Error":"Only admin and super admin are allowed"}`, body)
	require.True(t, true)
}

// stubTenant puts a resolved tenant on the context the way tenant.Middleware does.
func stubTenant(id string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("tenant", &tenant.Tenant{ID: id})
		return c.Next()
	}
}

func TestLoginGateUsesTenantPrefixedKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	v := &Verifier{Secret: []byte("s3cret"), Redis: rdb}
	app := fiber.New()
	app.Get("/", stubTenant("traya"), v.RequireJWT(), func(c *fiber.Ctx) error { return c.SendString("ok") })
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"id": "u1"}).SignedString([]byte("s3cret"))
	require.NoError(t, err)
	hdr := map[string]string{"Authorization": "Bearer " + tok}

	code, _ := call(app, "/", hdr)
	assert.Equal(t, 401, code, "no login status at all")

	// the tenant-prefixed key alone is enough
	mr.Set("traya:user!u1login!status", "1")
	code, body := call(app, "/", hdr)
	assert.Equal(t, 200, code)
	assert.Equal(t, "ok", body)

	// and so is the legacy key api-server still writes
	mr.Del("traya:user!u1login!status")
	mr.Set("user!u1login!status", "1")
	code, _ = call(app, "/", hdr)
	assert.Equal(t, 200, code, "legacy fallback keeps auth working mid-migration")

	// another tenant's prefixed key must not authorise this one
	mr.Del("user!u1login!status")
	mr.Set("mool:user!u1login!status", "1")
	code, _ = call(app, "/", hdr)
	assert.Equal(t, 401, code, "a different tenant's key must not grant access")

	// with the fallback disabled only the prefixed key works
	common.LegacyRedisFallback = false
	defer func() { common.LegacyRedisFallback = true }()
	mr.Del("mool:user!u1login!status")
	mr.Set("user!u1login!status", "1")
	code, _ = call(app, "/", hdr)
	assert.Equal(t, 401, code, "legacy key ignored once the fallback is off")
}
