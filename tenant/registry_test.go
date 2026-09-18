package tenant

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/models"
	"traya-bah-service/setup"
)

func newTestRegistry() *Registry {
	cfg := &setup.Config{TenantEconomies: map[string][]string{"traya": {"legacy", "habit"}, "mool": {"logearn"}}, MongoDBSuffix: "dev"}
	r := NewRegistry(cfg, nil, nil, nil)
	r.lookup = func(_ context.Context, id string) (*models.Tenant, error) {
		if id == "traya" || id == "mool" {
			return &models.Tenant{TenantID: id, TenantName: id}, nil
		}
		return nil, ErrUnknownTenant
	}
	return r
}

func TestResolve_EconomiesAndAuth(t *testing.T) {
	r := newTestRegistry()
	tr, err := r.Resolve(context.Background(), "traya")
	require.NoError(t, err)
	assert.True(t, tr.Has(Legacy))
	assert.True(t, tr.Has(Habit))
	assert.False(t, tr.Has(LogEarn))
	assert.Equal(t, AuthJWT, tr.AuthMode)
	m, err := r.Resolve(context.Background(), "mool")
	require.NoError(t, err)
	assert.Equal(t, AuthGateway, m.AuthMode)
	_, err = r.Resolve(context.Background(), "acne") // not configured
	assert.ErrorIs(t, err, ErrUnknownTenant)
	again, _ := r.Resolve(context.Background(), "traya")
	assert.Same(t, tr, again, "cached")
}

func do(app *fiber.App, path string, hdr map[string]string) (int, string) {
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, _ := app.Test(req)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestMiddleware(t *testing.T) {
	r := newTestRegistry()
	app := fiber.New()
	app.Use(Middleware(r, ""))
	app.Get("/legacy", RequireEconomy(Legacy), func(c *fiber.Ctx) error { return c.SendString(From(c).ID) })
	app.Get("/logearn", RequireEconomy(LogEarn), func(c *fiber.Ctx) error { return c.SendString(From(c).ID) })

	code, body := do(app, "/legacy", nil)
	assert.Equal(t, 400, code)
	assert.JSONEq(t, `{"message":"Tenant ID header missing"}`, body)
	code, body = do(app, "/legacy", map[string]string{Header: "kibo"})
	assert.Equal(t, 400, code)
	assert.JSONEq(t, `{"message":"Tenant not found: kibo"}`, body)
	code, body = do(app, "/legacy", map[string]string{Header: "TRAYA"})
	assert.Equal(t, 200, code)
	assert.Equal(t, "traya", body)
	code, body = do(app, "/logearn", map[string]string{Header: "traya"})
	assert.Equal(t, 404, code)
	assert.JSONEq(t, `{"message":"Not available for tenant traya"}`, body)

	appDef := fiber.New()
	appDef.Use(Middleware(r, "traya"))
	appDef.Get("/x", func(c *fiber.Ctx) error { return c.SendString(From(c).ID) })
	code, body = do(appDef, "/x", nil)
	assert.Equal(t, 200, code)
	assert.Equal(t, "traya", body)
}
