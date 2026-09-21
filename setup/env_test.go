package setup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequired(t *testing.T) {
	for k, v := range map[string]string{
		"MONGO_URI": "mongodb://x", "SERVICES_CACHE_HOST": "h", "JWT_SECRET": "s", "V2_FORM_DATA_TOKEN": "v",
		"INTERNAL_SERVICE_TOKEN": "i", "MONGO_DATABASE_SUFFIX": "dev", "POSTGRES_DATABASE_SUFFIX": "user_service",
		"ENVIRONMENT": "development",
	} {
		t.Setenv(k, v)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	setRequired(t)
	t.Setenv("MONGO_URI", "")
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MONGO_URI")
	assert.Contains(t, err.Error(), "JWT_SECRET")
}

func TestLoad_Defaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, 10*time.Second, cfg.HTTPTimeout)
	assert.Equal(t, []string{"legacy", "habit"}, cfg.TenantEconomies["traya"])
	assert.Equal(t, []string{"logearn"}, cfg.TenantEconomies["acne"])
	assert.Equal(t, "h:6379", cfg.RedisAddr)
	assert.True(t, cfg.ScratchCardsEnabled)
	assert.Equal(t, 32, cfg.LegacyWorkers)
	assert.False(t, cfg.IsProduction)
	assert.Nil(t, cfg.TRPGRead)
	assert.Nil(t, cfg.LifelineTestDelayMS)
	assert.Equal(t, 4, cfg.LifelineGraceHours)
	assert.Equal(t, "https://form.traya.health", cfg.FormBaseURL)
	assert.Equal(t, int32(2), cfg.TrayaPG.MinConns)
	assert.Equal(t, int32(25), cfg.TrayaPG.MaxConns)
}

func TestLoad_Overrides(t *testing.T) {
	setRequired(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("MONGO_DB_NAME_TRAYA", "TrayaProd")
	t.Setenv("POSTGRES_READ_HOST", "replica")
	t.Setenv("HABIT_LIFELINE_TEST_DELAY_MS", "0")
	t.Setenv("TENANT_ECONOMIES", `{"kibo":["logearn"]}`)
	t.Setenv("RECOMMENDATION_MAX_SOCKETS", "7")
	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.IsProduction)
	assert.Equal(t, "TrayaProd", cfg.MongoDBNameOverrides["TRAYA"])
	require.NotNil(t, cfg.TRPGRead)
	assert.Equal(t, "replica", cfg.TRPGRead.Host)
	require.NotNil(t, cfg.LifelineTestDelayMS)
	assert.Equal(t, 0, *cfg.LifelineTestDelayMS)
	assert.Equal(t, []string{"logearn"}, cfg.TenantEconomies["kibo"])
	assert.Equal(t, 7, cfg.MaxSockets["recommendation"])
}

func TestLegacyRedisFallbackDefaultsOn(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.LegacyRedisFallback {
		t.Fatal("LEGACY_REDIS_FALLBACK must default to true so the migration does not break")
	}

	t.Setenv("LEGACY_REDIS_FALLBACK", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LegacyRedisFallback {
		t.Fatal("LEGACY_REDIS_FALLBACK=false must turn the fallback off")
	}
}
