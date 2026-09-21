package habit

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/common"
	"traya-bah-service/setup"
)

func cacheSvc(t *testing.T, tenantID string) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s := &Service{
		TenantID: tenantID,
		Deps: Deps{
			Redis: rdb,
			Cfg:   &setup.Config{IsProduction: true},
			Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
	}
	return s, mr
}

func TestCalendarCacheWritesTenantPrefixedKey(t *testing.T) {
	s, mr := cacheSvc(t, "traya")
	ctx := context.Background()

	s.writeCalendarCache(ctx, "u1", &CalendarResponse{FirstLogDate: strptr("2026-01-01")})

	require.True(t, mr.Exists("traya:kit-tracker-calendar!u1"), "writes the tenant-prefixed key")
	assert.False(t, mr.Exists("kit-tracker-calendar!u1"), "never writes the unprefixed key")

	got := s.readCalendarCache(ctx, "u1")
	require.NotNil(t, got)
	require.NotNil(t, got.FirstLogDate)
	assert.Equal(t, "2026-01-01", *got.FirstLogDate)
}

func TestCalendarCacheReadsLegacyKeyAsFallback(t *testing.T) {
	s, mr := cacheSvc(t, "traya")
	ctx := context.Background()

	// only traya-app-backend's unprefixed key exists
	require.NoError(t, mr.Set("kit-tracker-calendar!u1", `{"firstLogDate":"2026-02-02"}`))

	got := s.readCalendarCache(ctx, "u1")
	require.NotNil(t, got, "falls back to the key the Node service still writes")
	require.NotNil(t, got.FirstLogDate)
	assert.Equal(t, "2026-02-02", *got.FirstLogDate)

	common.LegacyRedisFallback = false
	defer func() { common.LegacyRedisFallback = true }()
	assert.Nil(t, s.readCalendarCache(ctx, "u1"), "legacy key ignored once the fallback is off")
}

func TestCalendarCacheIsolatedBetweenTenants(t *testing.T) {
	s, mr := cacheSvc(t, "mool")
	ctx := context.Background()

	require.NoError(t, mr.Set("traya:kit-tracker-calendar!u1", `{"firstLogDate":"2026-03-03"}`))
	assert.Nil(t, s.readCalendarCache(ctx, "u1"), "mool must not read traya's cached calendar")

	s.writeCalendarCache(ctx, "u1", &CalendarResponse{FirstLogDate: strptr("2026-04-04")})
	assert.True(t, mr.Exists("mool:kit-tracker-calendar!u1"))
	v, err := mr.Get("traya:kit-tracker-calendar!u1")
	require.NoError(t, err)
	assert.Contains(t, v, "2026-03-03", "traya's entry is untouched")
}

func TestInvalidateCalendarCacheDeletesBothKeys(t *testing.T) {
	s, mr := cacheSvc(t, "traya")
	ctx := context.Background()

	require.NoError(t, mr.Set("traya:kit-tracker-calendar!u1", `{"firstLogDate":"a"}`))
	require.NoError(t, mr.Set("kit-tracker-calendar!u1", `{"firstLogDate":"b"}`))

	s.InvalidateCalendarCache(ctx, "u1")

	assert.False(t, mr.Exists("traya:kit-tracker-calendar!u1"))
	assert.False(t, mr.Exists("kit-tracker-calendar!u1"), "app-backend must not serve a stale calendar after a log")
}

func strptr(s string) *string { return &s }
