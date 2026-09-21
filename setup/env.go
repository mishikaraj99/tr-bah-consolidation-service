package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// PGConfig describes one Postgres connection target.
type PGConfig struct {
	Host, Port, User, Password, Database string
	MinConns, MaxConns                   int32
}

// Config is the typed view of the environment. Loaded once at boot.
type Config struct {
	Port          string
	Environment   string
	IsProduction  bool
	DefaultTenant string

	MongoURI             string
	MongoDBSuffix        string
	MongoDBNameOverrides map[string]string // key: upper-cased tenant id

	RedisAddr, RedisUsername, RedisPassword string
	RedisTLS                                bool

	TrayaPG    PGConfig
	TRPGWrite  PGConfig
	TRPGRead   *PGConfig
	TRPGSuffix string

	JWTSecret, V2FormDataToken, InternalServiceToken string
	CRMAdminGuard                                    bool
	TenantEconomies                                  map[string][]string

	RecommendationBaseURL, OrderServiceBaseURL, TROrderServiceBaseURL              string
	ShopfloEndpoint, ShopfloIssuerID, ShopfloMerchantID, ShopfloAPIKey             string
	CommunityBaseURL, CMSServiceBaseURL, TRCMSServiceBaseURL, ConfigServiceBaseURL string
	S3ImageBaseURL, FormBaseURL                                                    string

	HTTPTimeout time.Duration
	MaxSockets  map[string]int

	LegacyWorkers int

	LifelineWorkerEnabled bool
	LifelineGraceHours    int
	LifelineTestDelayMS   *int
	LifelinePollMS        int

	ScratchCardsEnabled bool

	// LegacyRedisFallback lets reads fall back to the unprefixed Redis keys that
	// traya-api-server and traya-app-backend still own. Switch it off once both
	// Node services write the tenant-prefixed names.
	LegacyRedisFallback bool
}

const defaultTenantEconomies = `{"traya":["legacy","habit"],"mool":["logearn"],"acne":["logearn"]}`

var requiredVars = []string{
	"MONGO_URI", "SERVICES_CACHE_HOST", "JWT_SECRET", "V2_FORM_DATA_TOKEN", "INTERNAL_SERVICE_TOKEN",
	"MONGO_DATABASE_SUFFIX", "POSTGRES_DATABASE_SUFFIX", "ENVIRONMENT",
}

// Load reads .env (if present) and the process environment into a Config.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var missing []string
	for _, k := range requiredVars {
		if strings.TrimSpace(os.Getenv(k)) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	cfg := &Config{
		Port:          getenv("PORT", "3000"),
		Environment:   os.Getenv("ENVIRONMENT"),
		DefaultTenant: os.Getenv("DEFAULT_TENANT"),
		MongoURI:      os.Getenv("MONGO_URI"),
		MongoDBSuffix: os.Getenv("MONGO_DATABASE_SUFFIX"),
		RedisAddr:     os.Getenv("SERVICES_CACHE_HOST") + ":" + getenv("SERVICES_CACHE_PORT", "6379"),
		RedisUsername: os.Getenv("SERVICES_CACHE_USERNAME"),
		RedisPassword: os.Getenv("SERVICES_CACHE_PASSWORD"),
		RedisTLS:      getbool("SERVICES_CACHE_TLS", false),
		TrayaPG: PGConfig{
			Host: getenv("DATABASE_HOST", "127.0.0.1"), Port: getenv("DATABASE_PORT", "5432"),
			User: getenv("DATABASE_USER", "postgres"), Password: getenv("DATABASE_PASSWORD", "postgres"),
			Database: getenv("DATABASE_NAME", "api_server_development"),
			MinConns: int32(getint("DATABASE_MIN_POOL_SIZE", 2)), MaxConns: int32(getint("DATABASE_MAX_POOL_SIZE", 25)),
		},
		TRPGWrite: PGConfig{
			Host: os.Getenv("POSTGRES_WRITE_HOST"), Port: getenv("POSTGRES_WRITE_PORT", "5432"),
			User: os.Getenv("POSTGRES_WRITE_USER"), Password: os.Getenv("POSTGRES_WRITE_PASSWORD"),
			MinConns: 1, MaxConns: int32(getint("POSTGRES_MAX_CONNECTIONS", 20)),
		},
		TRPGSuffix:           os.Getenv("POSTGRES_DATABASE_SUFFIX"),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		V2FormDataToken:      os.Getenv("V2_FORM_DATA_TOKEN"),
		InternalServiceToken: os.Getenv("INTERNAL_SERVICE_TOKEN"),
		CRMAdminGuard:        getbool("CRM_ADMIN_GUARD", false),

		RecommendationBaseURL: os.Getenv("RECOMMENDATION_SERVICE_BASE_URL"),
		OrderServiceBaseURL:   os.Getenv("ORDER_SERVICE_BASE_URL"),
		TROrderServiceBaseURL: os.Getenv("TR_ORDER_SERVICE_BASE_URL"),
		ShopfloEndpoint:       os.Getenv("SHOPFLO_WALLET_API_ENPOINT"),
		ShopfloIssuerID:       os.Getenv("SHOPFLO_WALLET_ISSUER_ID"),
		ShopfloMerchantID:     os.Getenv("SHOPFLO_WALLET_MERCHANT_ID"),
		ShopfloAPIKey:         os.Getenv("SHOPFLO_WALLET_API_KEY"),
		CommunityBaseURL:      os.Getenv("COMMUNITY_BASE_URL"),
		CMSServiceBaseURL:     os.Getenv("CMS_SERVICE_BASE_URL"),
		TRCMSServiceBaseURL:   os.Getenv("TR_CMS_SERVICE_BASE_URL"),
		ConfigServiceBaseURL:  os.Getenv("TR_CONFIG_SERVICE_BASE_URL"),
		S3ImageBaseURL:        os.Getenv("S3_IMAGE_BASE_URL"),
		FormBaseURL:           getenv("FORM_BASE_URL", "https://form.traya.health"),

		HTTPTimeout: time.Duration(getint("HTTP_DEFAULT_TIMEOUT_MS", 10000)) * time.Millisecond,
		MaxSockets: map[string]int{
			"recommendation": getint("RECOMMENDATION_MAX_SOCKETS", 30),
			"order":          getint("ORDER_MAX_SOCKETS", 30),
			"trorder":        getint("TR_ORDER_MAX_SOCKETS", 30),
			"shopflo":        getint("SHOPFLO_MAX_SOCKETS", 10),
			"cms":            getint("CMS_MAX_SOCKETS", 10),
			"trcms":          getint("TR_CMS_MAX_SOCKETS", 10),
			"config":         getint("CONFIG_MAX_SOCKETS", 10),
		},
		LegacyWorkers:         getint("LEGACY_WORKERS", 32),
		LifelineWorkerEnabled: getbool("HABIT_LIFELINE_WORKER_ENABLED", false),
		LifelineGraceHours:    getint("HABIT_LIFELINE_GRACE_HOURS", 4),
		LifelinePollMS:        getint("HABIT_LIFELINE_POLL_MS", 5000),
		ScratchCardsEnabled:   getbool("HABIT_TRACKER_SCRATCH_CARDS_ENABLED", true),
		LegacyRedisFallback:   getbool("LEGACY_REDIS_FALLBACK", true),
	}
	cfg.IsProduction = cfg.Environment == "production"

	if v := strings.TrimSpace(os.Getenv("HABIT_LIFELINE_TEST_DELAY_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.LifelineTestDelayMS = &n
		}
	}
	if h := os.Getenv("POSTGRES_READ_HOST"); h != "" {
		cfg.TRPGRead = &PGConfig{
			Host: h, Port: getenv("POSTGRES_READ_PORT", "5432"),
			User: getenv("POSTGRES_READ_USER", cfg.TRPGWrite.User), Password: getenv("POSTGRES_READ_PASSWORD", cfg.TRPGWrite.Password),
			MinConns: 1, MaxConns: cfg.TRPGWrite.MaxConns,
		}
	}

	cfg.MongoDBNameOverrides = map[string]string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "MONGO_DB_NAME_") {
			k, v, _ := strings.Cut(kv, "=")
			if v != "" {
				cfg.MongoDBNameOverrides[strings.ToUpper(strings.TrimPrefix(k, "MONGO_DB_NAME_"))] = v
			}
		}
	}

	raw := getenv("TENANT_ECONOMIES", defaultTenantEconomies)
	if err := json.Unmarshal([]byte(raw), &cfg.TenantEconomies); err != nil {
		return nil, fmt.Errorf("TENANT_ECONOMIES is not valid JSON: %w", err)
	}
	if len(cfg.TenantEconomies) == 0 {
		return nil, errors.New("TENANT_ECONOMIES must map at least one tenant")
	}
	return cfg, nil
}

// MustLoad is Load that exits the process on error.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return cfg
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getint(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getbool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return def
}
