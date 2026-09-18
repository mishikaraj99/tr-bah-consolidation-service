# tr-bah-service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `tr-bah-service`, a multi-tenant Go service that serves every surviving Build-A-Habit endpoint for the `traya` (legacy 3/7/21 + v85 Habit Tracker) and `mool`/`acne` (Log & Earn) tenants, with response shapes identical to today's Node services.

**Architecture:** Fiber HTTP layer → per-tenant context (Mongo DB, Postgres pool, auth strategy, enabled economies) → three economy packages (`internal/legacy`, `internal/habit`, `internal/logearn`) built on shared `internal/orders` kit math and typed repositories. Behaviour is ported function-by-function from the pseudo-code in `docs/reference/inventory-*.md`, which are the ground truth for copy strings, constants and field names.

**Tech Stack:** Go 1.25, `github.com/gofiber/fiber/v2`, `go.mongodb.org/mongo-driver` v1, `github.com/jackc/pgx/v5` + `pgxpool`, `github.com/redis/go-redis/v9`, `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`, `github.com/joho/godotenv`, `github.com/stretchr/testify`, `github.com/gofiber/swagger` + `swaggo/swag`.

## Global Constraints

- Module name `traya-bah-service`; Go `1.25.0`; repo `/Users/mishika/Desktop/traya/tr/tr-bah-service`.
- Every JSON field name, copy string, image URL, constant and error message must match `docs/reference/inventory-*.md` byte for byte. When the inventory and the source JS disagree, the JS wins — read the referenced `handler.js` lines.
- Error envelopes per route family: legacy `{ "err": ... }` / `{ "message": ... }` as tabulated in `inventory-api-server-legacy-bah.md` §5; v85 `{ "message": ... }`; logearn `{ "message", "statusCode", "timestamp", "path" }`.
- Fixes allowed ONLY as listed in spec §6.1. Quirks in spec §6.2 are preserved.
- Time: `time.Local = time.UTC` at startup. Use `common.ISTShift`/`common.UTCMidnight` where the api-server used offset math, `common.ISTDay*` where the source used `Asia/Kolkata` (lifelines, logearn, `getRewardsCount`).
- Identity for public Traya routes comes only from the verified JWT. `GET /bah/:userId/calendar` ignores the path param.
- Never log secrets or full phone numbers (`common.MaskPhone`).
- `gofmt`, `go vet ./...` and `go test ./...` must pass before every commit. Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Integration tests run only when `TEST_MONGO_URI`, `TEST_PG_URI`, `TEST_REDIS_ADDR` are set (else `t.Skip`). `docker-compose.test.yml` provides them. (This replaces the spec's testcontainers mention — same intent, fewer moving parts.)

---

## File Structure (created by this plan)

```
main.go                                  Task 1
go.mod / go.sum                          Task 1
Dockerfile .air.toml .gitignore README.md CLAUDE.md .env.example docker-compose.test.yml   Task 1, 33
setup/env.go                             Task 2
setup/logger.go                          Task 2
setup/mongo.go                           Task 3
setup/postgres.go                        Task 3
setup/redis.go                           Task 3
setup/httpclient.go                      Task 3
internal/common/istdate.go (+_test)      Task 4
internal/common/errors.go, response.go   Task 5
internal/common/validate.go (+_test)     Task 5
internal/common/clock.go                 Task 4
models/*.go                              Task 6
repositories/mongo/*.go                  Task 6, 7
repositories/pg/*.go                     Task 8
tenant/registry.go, context.go, middleware.go (+_test)   Task 9
auth/*.go (+_test)                       Task 10
internal/orders/*.go (+_test)            Task 11, 12
internal/legacy/*.go (+_test)            Tasks 13–21
internal/habit/*.go (+_test)             Tasks 22–27
internal/habit/lifeline/*.go (+_test)    Task 28
internal/logearn/*.go (+_test)           Tasks 29–30
controllers/*.go, routes/routes.go       Task 31
migrations/**                            Task 32
cmd/parity/main.go, cmd/migrate/main.go  Task 32
docs/swagger.*                           Task 33
```

Dependency rule: `controllers → internal/* → repositories → models/setup`. `internal/*` never imports Fiber. Only `routes`, `controllers`, `main` import `tenant` and `auth`.

---

## Task 1: Repository skeleton and boot

**Files:**
- Create: `go.mod`, `main.go`, `Dockerfile`, `.air.toml`, `README.md`, `CLAUDE.md`, `.env.example`, `docker-compose.test.yml`
- Modify: `.gitignore`

**Interfaces:**
- Produces: `main()` boots Fiber on `PORT` (default 3000) with `/health` → `OK`. Later tasks add `setup.Load()`, `routes.Setup(app, deps)`.

- [ ] **Step 1: Initialise the module and pin dependencies**

```bash
cd /Users/mishika/Desktop/traya/tr/tr-bah-service
go mod init traya-bah-service
go get github.com/gofiber/fiber/v2@v2.52.13 go.mongodb.org/mongo-driver@v1.17.7 github.com/redis/go-redis/v9@v9.7.3 \
  github.com/jackc/pgx/v5@latest github.com/golang-jwt/jwt/v5@latest github.com/google/uuid@v1.6.0 \
  github.com/joho/godotenv@v1.5.1 github.com/stretchr/testify@latest github.com/gofiber/swagger@v1.1.1 github.com/swaggo/swag@v1.16.4
```

- [ ] **Step 2: Write `main.go`**

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// @title       Traya BAH Service
// @version     1.0
// @description Multi-tenant Build-A-Habit service (traya legacy + v85, mool/acne Log & Earn)
func main() {
	time.Local = time.UTC
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())
	app.Use(requestid.New(requestid.Config{Header: "x-trace-id"}))
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("OK") })

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	go func() {
		if err := app.Listen(":" + port); err != nil {
			slog.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(ctx)
}
```

- [ ] **Step 3: Dockerfile, air, env example, compose, README, CLAUDE.md**

`Dockerfile` (same shape as tr-config-service):
```dockerfile
FROM golang:1.25-alpine AS build_base
RUN apk add --no-cache git
WORKDIR /tmp/traya-bah-service
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o ./out/traya-bah-service .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build_base /tmp/traya-bah-service/out/traya-bah-service /app/traya-bah-service
EXPOSE 3000
CMD ["/app/traya-bah-service"]
```
Copy `.air.toml` from `/Users/mishika/Desktop/traya/tr/tr-config-service/.air.toml` unchanged.

`.env.example` — one line per variable in spec §6.6/§7 with a comment, values blank:
```
PORT=3000
ENVIRONMENT=development            # production enables Mongo transactions and TrayaProd db name
MONGO_URI=
MONGO_DATABASE_SUFFIX=dev          # tenant db = <tenant>_<suffix>
MONGO_DB_NAME_TRAYA=               # optional override
SERVICES_CACHE_HOST=127.0.0.1
SERVICES_CACHE_PORT=6379
SERVICES_CACHE_PASSWORD=
SERVICES_CACHE_TLS=false
DATABASE_HOST=127.0.0.1            # traya postgres (api-server db)
DATABASE_PORT=5432
DATABASE_USER=postgres
DATABASE_PASSWORD=postgres
DATABASE_NAME=api_server_development
DATABASE_MIN_POOL_SIZE=2
DATABASE_MAX_POOL_SIZE=25
POSTGRES_WRITE_HOST=               # mool/acne tenant postgres
POSTGRES_WRITE_PORT=5432
POSTGRES_WRITE_USER=
POSTGRES_WRITE_PASSWORD=
POSTGRES_READ_HOST=                # optional replica
POSTGRES_READ_PORT=
POSTGRES_READ_USER=
POSTGRES_READ_PASSWORD=
POSTGRES_DATABASE_SUFFIX=user_service
POSTGRES_MASTER_DATABASE_NAME=
JWT_SECRET=
V2_FORM_DATA_TOKEN=
INTERNAL_SERVICE_TOKEN=
CRM_ADMIN_GUARD=false
TENANT_ECONOMIES={"traya":["legacy","habit"],"mool":["logearn"],"acne":["logearn"]}
RECOMMENDATION_SERVICE_BASE_URL=
RECOMMENDATION_MAX_SOCKETS=30
ORDER_SERVICE_BASE_URL=
TR_ORDER_SERVICE_BASE_URL=
SHOPFLO_WALLET_API_ENPOINT=        # sic, must end with /
SHOPFLO_WALLET_ISSUER_ID=
SHOPFLO_WALLET_MERCHANT_ID=
SHOPFLO_WALLET_API_KEY=
COMMUNITY_BASE_URL=                # must end with /
CMS_SERVICE_BASE_URL=
TR_CMS_SERVICE_BASE_URL=
TR_CONFIG_SERVICE_BASE_URL=
S3_IMAGE_BASE_URL=
FORM_BASE_URL=https://form.traya.health
HTTP_DEFAULT_TIMEOUT_MS=10000
LEGACY_WORKERS=32
HABIT_LIFELINE_WORKER_ENABLED=false
HABIT_LIFELINE_GRACE_HOURS=4
HABIT_LIFELINE_TEST_DELAY_MS=
HABIT_LIFELINE_POLL_MS=5000
HABIT_TRACKER_SCRATCH_CARDS_ENABLED=true
DEFAULT_TENANT=                    # optional; only for the ALB-direct rollout phase
```

`docker-compose.test.yml`:
```yaml
services:
  mongo: { image: mongo:7, ports: ["27117:27017"] }
  postgres: { image: postgres:16, environment: { POSTGRES_PASSWORD: test, POSTGRES_DB: bah_test }, ports: ["55432:5432"] }
  redis: { image: redis:7, ports: ["63790:6379"] }
```
README: how to run (`cp .env.example .env`, `go run .`), test (`docker compose -f docker-compose.test.yml up -d && TEST_MONGO_URI=mongodb://localhost:27117 TEST_PG_URI=postgres://postgres:test@localhost:55432/bah_test TEST_REDIS_ADDR=localhost:63790 go test ./...`), swagger (`swag init`). CLAUDE.md: copy the `/ship` block from tr-config-service/CLAUDE.md plus "Ground truth for behaviour is docs/reference/inventory-*.md; never invent copy strings."

`.gitignore` add: `/tmp`, `.env`, `/.idea`, `/bin`, `/out`.

- [ ] **Step 4: Verify boot**

Run: `go build ./... && (PORT=3999 go run . & sleep 2; curl -s localhost:3999/health; kill %1)`
Expected: `OK`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "chore: bootstrap tr-bah-service module, Fiber app, Dockerfile, env example

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Task 2: Typed configuration and logger

**Files:**
- Create: `setup/env.go`, `setup/env_test.go`, `setup/logger.go`

**Interfaces:**
- Produces:
```go
package setup
type PGConfig struct{ Host, Port, User, Password, Database string; MinConns, MaxConns int32 }
type Config struct {
  Port, Environment string; IsProduction bool
  MongoURI, MongoDBSuffix string; MongoDBNameOverrides map[string]string // key upper tenant id
  RedisAddr, RedisUsername, RedisPassword string; RedisTLS bool
  TrayaPG PGConfig; TRPGWrite PGConfig; TRPGRead *PGConfig; TRPGSuffix string
  JWTSecret, V2FormDataToken, InternalServiceToken string; CRMAdminGuard bool
  TenantEconomies map[string][]string; DefaultTenant string // DEFAULT_TENANT, optional
  RecommendationBaseURL, OrderServiceBaseURL, TROrderServiceBaseURL string
  ShopfloEndpoint, ShopfloIssuerID, ShopfloMerchantID, ShopfloAPIKey string
  CommunityBaseURL, CMSServiceBaseURL, TRCMSServiceBaseURL, ConfigServiceBaseURL, S3ImageBaseURL, FormBaseURL string
  HTTPTimeout time.Duration; MaxSockets map[string]int // "recommendation","order","appbackend" etc
  LegacyWorkers int
  LifelineWorkerEnabled bool; LifelineGraceHours int; LifelineTestDelayMS *int; LifelinePollMS int
  ScratchCardsEnabled bool
}
func Load() (*Config, error)          // godotenv.Load() then os.Getenv; validates required
func MustLoad() *Config
func NewLogger(env string) *slog.Logger   // JSON handler, level INFO (DEBUG when env != production)
```
- Required at boot (error lists all missing): `MONGO_URI`, `SERVICES_CACHE_HOST`, `JWT_SECRET`, `V2_FORM_DATA_TOKEN`, `INTERNAL_SERVICE_TOKEN`, `MONGO_DATABASE_SUFFIX`, `POSTGRES_DATABASE_SUFFIX`, `ENVIRONMENT`. `TENANT_ECONOMIES` default `{"traya":["legacy","habit"],"mool":["logearn"],"acne":["logearn"]}`.

- [ ] **Step 1: Failing test**

```go
func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("MONGO_URI", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MONGO_URI")
}
func TestLoad_Defaults(t *testing.T) {
	for k, v := range map[string]string{"MONGO_URI": "mongodb://x", "SERVICES_CACHE_HOST": "h", "JWT_SECRET": "s", "V2_FORM_DATA_TOKEN": "v", "INTERNAL_SERVICE_TOKEN": "i", "MONGO_DATABASE_SUFFIX": "dev", "POSTGRES_DATABASE_SUFFIX": "user_service", "ENVIRONMENT": "development"} { t.Setenv(k, v) }
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, 10*time.Second, cfg.HTTPTimeout)
	assert.Equal(t, []string{"legacy", "habit"}, cfg.TenantEconomies["traya"])
	assert.Equal(t, "h:6379", cfg.RedisAddr)
	assert.True(t, cfg.ScratchCardsEnabled)
	assert.Equal(t, 32, cfg.LegacyWorkers)
	assert.False(t, cfg.IsProduction)
}
```

- [ ] **Step 2: Run** `go test ./setup/ -run TestLoad -v` → FAIL (undefined Load).
- [ ] **Step 3: Implement** `Load()` reading each variable with helpers `getenv(key, def string)`, `getint(key string, def int)`, `getbool`, `getdur(msKey, defMS)`. `RedisAddr = host + ":" + port(default 6379)`. `IsProduction = Environment == "production"`. Parse `TENANT_ECONOMIES` with `encoding/json`. `MongoDBNameOverrides` from any `MONGO_DB_NAME_<X>` env. `TRPGRead` nil when `POSTGRES_READ_HOST` empty. `LifelineTestDelayMS` nil unless set to a non-negative int.
- [ ] **Step 4: Run** → PASS. `go vet ./...` clean.
- [ ] **Step 5: Commit** `feat(setup): typed config loader and slog logger`

---

## Task 3: Datastore and HTTP client factories

**Files:**
- Create: `setup/mongo.go`, `setup/postgres.go`, `setup/redis.go`, `setup/httpclient.go`, `setup/mongo_test.go` (integration, skip w/o env)

**Interfaces:**
- Produces:
```go
func ConnectMongo(ctx context.Context, uri string) (*mongo.Client, error)   // 10s timeout + ping
func TenantMongoDBName(cfg *Config, tenantID string) string  // override > (production && traya → "TrayaProd") > tenant+"_"+suffix
func ConnectPG(ctx context.Context, c PGConfig) (*pgxpool.Pool, error)      // pgxpool with MinConns/MaxConns, ping
func ConnectRedis(cfg *Config) *redis.Client                                // Addr/Username/Password/TLS
func NewHTTPClient(maxSockets int, timeout time.Duration) *http.Client      // Transport MaxIdleConnsPerHost=maxSockets, IdleConnTimeout 90s
type HTTPClients struct{ Recommendation, OrderService, TROrderService, Shopflo, CMS, TRCMS, ConfigService *http.Client }
func NewHTTPClients(cfg *Config) *HTTPClients
```

- [ ] **Step 1: Failing test** for `TenantMongoDBName`:
```go
func TestTenantMongoDBName(t *testing.T) {
	cfg := &Config{MongoDBSuffix: "dev", MongoDBNameOverrides: map[string]string{"ACNE": "acne_custom"}}
	assert.Equal(t, "mool_dev", TenantMongoDBName(cfg, "mool"))
	assert.Equal(t, "acne_custom", TenantMongoDBName(cfg, "acne"))
	cfg.IsProduction = true
	assert.Equal(t, "TrayaProd", TenantMongoDBName(cfg, "traya"))
	assert.Equal(t, "mool_dev", TenantMongoDBName(cfg, "mool"))
}
```
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** the four files. `ConnectMongo` uses `options.Client().ApplyURI(uri).SetMaxPoolSize(100)`. **Step 4:** PASS. Integration test (`TEST_MONGO_URI`) pings.
- [ ] **Step 5: Commit** `feat(setup): mongo, postgres, redis and pooled http client factories`

---

## Task 4: Time helpers and Clock

**Files:**
- Create: `internal/common/clock.go`, `internal/common/istdate.go`, `internal/common/istdate_test.go`

**Interfaces:**
```go
package common
type Clock interface{ Now() time.Time }
type RealClock struct{}; func (RealClock) Now() time.Time { return time.Now().UTC() }
type FixedClock struct{ T time.Time }; func (f FixedClock) Now() time.Time { return f.T }

var IST = time.FixedZone("IST", 330*60)
func ISTShift(t time.Time) time.Time            // t + 330 minutes, still UTC location (api-server getClientTimeFromUtcTime / getIndianTime)
func UTCMidnight(t time.Time) time.Time         // setUTCHours(0,0,0,0)
func StartOfDayUTC(t time.Time) time.Time       // same as UTCMidnight (api-server setTimeStartOfDay with process in UTC)
func EndOfDayUTC(t time.Time) time.Time         // 23:59:59.999
func CalendarDaysDifference(a, b time.Time) int // abs((UTCMidnight(b)-UTCMidnight(a))/24h)  (api-server calculateDaysDifference)
func ISTDate(t time.Time) time.Time             // midnight IST of t's IST calendar day, in IST zone
func ISTDateString(t time.Time) string          // YYYY-MM-DD in IST
func ISTDaysBetween(a, b time.Time) int         // max(0, round((ISTDate(b)-ISTDate(a))/24h))  (logearn istDaysBetween)
func ISTDayAnchor(t time.Time) time.Time        // UTC instant of 00:00 IST that day (logearn istDayAnchor)
func ISTStartOfDay(t time.Time) time.Time       // == ISTDayAnchor (lifeline schedule IST startOf('day'))
func ParseYMD(s string) (time.Time, error)      // "YYYY-MM-DD" → UTC midnight
func FormatMoment(t time.Time, layout string) string // supports tokens: "DD MMM YYYY","D MMMM, YYYY","D MMM, YYYY","D MMM","MMMM YYYY","YYYY-MM-DD","Do MMMM"
func JSDateString(t time.Time) string           // "Thu Sep 18 2026 18:30:00 GMT+0000 (Coordinated Universal Time)"
func DayText(n int) string                      // n==1 ? "day" : "days"
func Plural(n int, one, many string) string
```

- [ ] **Step 1: Tests** (table-driven):
```go
func TestISTBoundaries(t *testing.T) {
	// 2026-09-18T18:29:59Z is still 2026-09-18 in IST? 18:29:59Z = 23:59:59 IST → same day
	tm := time.Date(2026, 9, 18, 18, 29, 59, 0, time.UTC)
	assert.Equal(t, "2026-09-18", ISTDateString(tm))
	assert.Equal(t, "2026-09-19", ISTDateString(tm.Add(time.Second)))
	assert.Equal(t, time.Date(2026, 9, 17, 18, 30, 0, 0, time.UTC), ISTDayAnchor(tm))
	assert.Equal(t, 1, ISTDaysBetween(tm, tm.Add(time.Second)))
	assert.Equal(t, 0, ISTDaysBetween(tm.Add(time.Second), tm)) // max(0,…)
	assert.Equal(t, time.Date(2026, 9, 18, 23, 59, 59, 0, time.UTC), ISTShift(tm))
	assert.Equal(t, 1, CalendarDaysDifference(tm, tm.Add(time.Second)))     // UTC midnight math: 18th vs 18th → 0? No: 18:29:59+1s is still 18th UTC → 0
}
```
Fix the last assertion to `0` and add a case crossing UTC midnight → `1`. Add `FormatMoment` cases: `FormatMoment(time.Date(2026,9,5,0,0,0,0,time.UTC),"D MMMM, YYYY") == "5 September, 2026"`, `"DD MMM YYYY" == "05 Sep 2026"`, `"Do MMMM" == "5th September"`. `JSDateString` exact string for `2026-09-18T18:30:00Z`.
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5: Commit** `feat(common): IST/UTC date helpers, moment-style formatting, clock`

---

## Task 5: Errors, envelopes, validators

**Files:**
- Create: `internal/common/errors.go`, `internal/common/response.go`, `internal/common/validate.go`, `internal/common/validate_test.go`, `internal/common/mask.go`

**Interfaces:**
```go
type HTTPError struct{ Status int; Message string; Body any } // Body overrides Message when non-nil (proxy pass-through)
func (e *HTTPError) Error() string
func BadRequest(msg string) *HTTPError            // 400
func NotFound400(msg string) *HTTPError           // 400 (v85 factory quirk)
func NotFound(msg string) *HTTPError              // 404
func Gone(msg string) *HTTPError                  // 410
func Forbidden(msg string) *HTTPError             // 403
func Unauthorized(msg string) *HTTPError          // 401
func Internal(msg string) *HTTPError              // 500
func StatusOf(err error) int                      // HTTPError.Status else 500
func MessageOf(err error) string                  // HTTPError.Message else "Unexpected error occurred"

type Family int; const ( FamilyLegacyErr Family = iota /* {err: e} 500 */; FamilyLegacyMessage /* {message} status */; FamilyV85 /* {message} status */; FamilyCalendar /* {error} / {error,details} */; FamilyCoinTxn /* {err: message} 500 */; FamilyProxy /* status + {err: body} */; FamilyLogEarn /* {message,statusCode,timestamp,path} */ )
func WriteError(c *fiber.Ctx, fam Family, err error) error
func WriteJSON(c *fiber.Ctx, status int, v any) error   // status 200 default; logearn POST 201

func MaskPhone(p string) string                   // "+91******1234"; "" stays ""

// Joi-equivalent validation producing Joi's message text
type FieldError struct{ Path, Msg string }
func ValidateLogActivity(body map[string]any) error   // rules inventory legacy §3.19 logActivityObject → BadRequest("Validation error: \"isLogForToday\" is required") etc.
func ValidateMultipleActivityLog(body map[string]any) error
```
Joi message formats to reproduce: `"<key>" is required`, `"<key>" must be a boolean`, `"<key>" must be a string`, `"<key>" must be a number`, `"<key>" must be an array`, `"<key>" is not allowed` (unknown key), `"<key>" must be of type object`. The first error only (`error.details[0].message`), nested paths like `"productPrescriptions[0].name" is required`.

- [ ] **Step 1: Tests**: missing `isLogForToday` → `Validation error: "isLogForToday" is required`; wrong type → `must be a boolean`; unknown key `foo` at root → `"foo" is not allowed`; `productPrescriptions` item missing `name` → `"productPrescriptions[0].name" is required`; multiple-log `productId` string → `"logProductDetail[0].productId" must be a number`; `MaskPhone("+919876543210") == "+91******3210"`.
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5: Commit** `feat(common): http errors, envelope writers per family, Joi-compatible validators`

---

## Task 6: Mongo models and store

**Files:**
- Create: `models/activity_log.go`, `models/streak_log.go`, `models/streak_master.go`, `models/reward_transaction.go`, `models/redeem_transaction.go`, `models/archived_products.go`, `models/lifeline_log.go`, `models/scratch_card.go`, `models/badge.go`, `models/customer_activity_log.go`, `models/task.go`, `models/tenant.go`
- Create: `repositories/mongo/store.go`, `repositories/mongo/indexes.go`

**Interfaces:** every struct uses `bson` tags equal to the inventory field names and `json` tags where the struct is returned raw. Key ones:
```go
type ActivityLog struct {
  ID primitive.ObjectID `bson:"_id,omitempty"`; UserID string `bson:"user_id"`; CheckInsForDate time.Time `bson:"check_ins_for_date"`
  ProductPrescriptions []map[string]any `bson:"product_prescriptions"`   // free-form, stored verbatim
  IsValidForStreak bool `bson:"is_valid_for_streak"`; IsActive bool `bson:"is_active"`
  IsLifeline *bool `bson:"is_lifeline,omitempty"`; LogSource string `bson:"log_source,omitempty"`
  CreatedAt time.Time `bson:"createdAt"`; UpdatedAt time.Time `bson:"updatedAt"` }
type StreakLog struct { ID primitive.ObjectID `bson:"_id,omitempty"`; UserID string `bson:"user_id"`; StreakAchieveDays int `bson:"streak_achieve_days"`; LongestStreakDays int `bson:"longest_streak_days"`; FirstDateOfLog time.Time `bson:"first_date_of_log"`; LastDateOfLog time.Time `bson:"last_date_of_log"`; IsActive *bool `bson:"is_active,omitempty"`; LogSource string `bson:"log_source,omitempty"`; CreatedAt, UpdatedAt time.Time }
type StreakMaster struct { ID primitive.ObjectID `bson:"_id,omitempty"`; DisplayName string `bson:"display_name"`; Days int `bson:"days"`; Slug string `bson:"slug"`; IsActive bool `bson:"is_active"`; IsForSuperadmin bool `bson:"is_for_superadmin"`; RewardCoins int `bson:"reward_coins"` }
type DebitTransaction struct { DebitTransactionID primitive.ObjectID `bson:"debit_transaction_id"`; DebitCoins int `bson:"debit_coins"`; TransactionStatus string `bson:"transaction_status"`; DebitRemarks string `bson:"debit_remarks"`; DebitTransactionDate time.Time `bson:"debit_transaction_date"`; DebitTransactionUpdatedDate *time.Time `bson:"debit_transaction_updated_date,omitempty"` }
type RewardTransaction struct { ID primitive.ObjectID `bson:"_id,omitempty"`; UserID string `bson:"user_id"`; StreakMasterID primitive.ObjectID `bson:"streak_master_id"`; CreditCoins int `bson:"credit_coins"`; IsCreditTransaction bool `bson:"is_credit_transaction"`; TotalDebitCoins int `bson:"total_debit_coins"`; IsDebitTransaction bool `bson:"is_debit_transaction"`; DebitTransactions []DebitTransaction `bson:"debit_transactions"`; CreditRemarks string `bson:"credit_remarks"`; AllCoinsUsed bool `bson:"all_coins_used"`; Status string `bson:"status"`; ExpireAt *time.Time `bson:"expire_at,omitempty"`; CreatedAt, UpdatedAt time.Time }
type RedeemTransaction struct { ID primitive.ObjectID; UserID string `bson:"user_id"`; RedeemedCoins int `bson:"redeemed_coins"`; RedeemedAmount float64 `bson:"redeemed_amount"`; OrderID *string `bson:"order_id"`; OrderDisplayID *string `bson:"order_display_id"`; ShopFloTxnID *string `bson:"shop_flo_txn_id,omitempty"`; Currency string `bson:"currency"`; Status string `bson:"status"`; Remarks string `bson:"remarks"`; Meta map[string]any `bson:"meta,omitempty"`; CreatedAt, UpdatedAt time.Time }
type ArchivedProducts struct { UserID string `bson:"user_id"`; ArchivedProductIDs []string `bson:"archived_product_ids"`; UpdatedAt time.Time `bson:"updated_at"` }
type LifelineLog struct { UserID string `bson:"user_id"`; DateCovered time.Time `bson:"date_covered"`; KitStart time.Time `bson:"kit_start"`; AppliedAt time.Time `bson:"applied_at"`; Source string `bson:"source"`; StreakDayAtApply int `bson:"streak_day_at_apply"` }
type ScratchCard struct { ID primitive.ObjectID `bson:"_id,omitempty"`; UserID string `bson:"user_id"`; Source string `bson:"source"`; RewardType string `bson:"reward_type"`; RewardValue int `bson:"reward_value"`; RewardMeta struct{ Slug string `bson:"slug"`; StreakDay int `bson:"streak_day"`; Tier string `bson:"tier"` } `bson:"reward_meta"`; RewardDayDate string `bson:"reward_day_date"`; PhoneNumber string `bson:"phone_number"`; Status string `bson:"status"`; ExpiresAt time.Time `bson:"expires_at"`; ClaimedAt *time.Time `bson:"claimed_at"`; RewardTransactionRef *primitive.ObjectID `bson:"reward_transaction_ref"`; CreditRemarks string `bson:"credit_remarks"`; CreatedAt, UpdatedAt time.Time }
type Badge struct { UserID string `bson:"user_id"`; KitNumber int `bson:"kit_number"`; BadgeID string `bson:"badge_id,omitempty"`; Source string `bson:"source"`; EarnedAt time.Time `bson:"earned_at"` }
type CustomerActivityLog struct { CaseID string `bson:"case_id"`; ActionDate time.Time `bson:"action_date"`; Event string `bson:"event"`; OrderID *string `bson:"order_id"`; ReminderDays []int `bson:"reminder_days"`; CreatedAt time.Time `bson:"createdAt"` }
type Tenant struct { TenantID string `bson:"tenant_id"`; TenantName string `bson:"tenant_name"` }

package mongorepo // repositories/mongo
const ( CollActivityLogs="user_activity_logs_for_bah"; CollStreakLogs="streak_logs"; CollStreakMasters="streak_masters"; CollRewardTransactions="reward_transactions"; CollRedeemTransactions="redeem_reward_transactions"; CollArchivedProducts="user_bah_archived_products"; CollLifelineLogs="habit_tracker_lifeline_logs"; CollScratchCards="scratch_cards"; CollBadges="user_habit_tracker_badges"; CollCustomerActivityLogs="customeractivitylogs"; CollTaskMaster="task_master"; CollUserTaskDetails="user_task_details" )
type Store struct{ DB *mongo.Database; Clock common.Clock }
func NewStore(db *mongo.Database, clk common.Clock) *Store
func (s *Store) C(name string) *mongo.Collection
func EnsureIndexes(ctx context.Context, db *mongo.Database) error   // CreateMany per collection per spec §4; idempotent
func IsDuplicateKey(err error) bool
```
`EnsureIndexes` partial unique on reward_transactions: `Keys {user_id:1, credit_remarks:1}`, `PartialFilterExpression {is_credit_transaction:true, credit_remarks:{$regex:"^(bah-legacy-|habit-tracker-)"}}`, `Unique(true)`.

- [ ] **Step 1: Integration test** (skip without `TEST_MONGO_URI`): `EnsureIndexes` twice is a no-op; inserting two reward txns with same `(user_id, credit_remarks:"bah-legacy-first-log")` → second `IsDuplicateKey`; two with `credit_remarks:"Manual grant"` → both succeed.
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(models): mongo document models, store and index bootstrap`

---

## Task 7: Mongo repositories

**Files:**
- Create: `repositories/mongo/activity_logs.go`, `streak_logs.go`, `streak_masters.go`, `reward_transactions.go`, `redeem_transactions.go`, `archived_products.go`, `lifeline_logs.go`, `scratch_cards.go`, `badges.go`, `customer_activity_logs.go`, `tasks.go`, and `*_test.go` (integration)

**Interfaces** (all methods take `ctx context.Context` first; errors returned; `mongo.ErrNoDocuments` mapped to `(nil, nil)` for FindOne helpers):
```go
// activity logs — day-range helpers take explicit bounds so call sites keep their $gt/$gte semantics
type DayRange struct{ From, To time.Time; FromInclusive, ToInclusive bool }
func (s *Store) FindActivityLogInRange(ctx, userID string, r DayRange, activeOnly bool) (*models.ActivityLog, error)
func (s *Store) ExistsActivityLogInRange(ctx, userID string, r DayRange, activeOnly bool) (bool, error)
func (s *Store) CreateActivityLog(ctx, doc *models.ActivityLog) (*models.ActivityLog, error)            // sets createdAt/updatedAt
func (s *Store) FindActivityLogsBetween(ctx, userID string, from, to time.Time, activeOnly bool, projection bson.M) ([]models.ActivityLog, error)   // $gte/$lte
func (s *Store) FindAllActivityLogs(ctx, userID string, projection bson.M) ([]models.ActivityLog, error)  // is_active:true, sort check_ins_for_date asc
func (s *Store) LatestActivityLogPrescriptions(ctx, userID string) ([]map[string]any, error)              // {user_id,is_active:true} sort check_ins desc, projection product_prescriptions
func (s *Store) FirstRealActivityLogOnOrAfter(ctx, userID string, from time.Time) (*models.ActivityLog, error) // is_active, is_lifeline $ne true
func (s *Store) CountValidStreakLogs(ctx, userID string, since *time.Time) (int64, error)
func (s *Store) FindValidStreakLogDates(ctx, userID string) ([]time.Time, error)
func (s *Store) SetProductCheckIns(ctx, logID primitive.ObjectID, productID string, morning, evening bool) error   // positional $ update
func (s *Store) BulkSetProductCheckIns(ctx, logID primitive.ObjectID, updates map[string][2]bool) error
func (s *Store) UpsertLifelineActivityLog(ctx, userID string, dateCovered time.Time, prescriptions []map[string]any) error
// streak logs
func (s *Store) FindActiveStreak(ctx, userID string) (*models.StreakLog, error)
func (s *Store) CreateStreak(ctx, doc *models.StreakLog) (*models.StreakLog, error)
func (s *Store) UpdateActiveStreak(ctx, userID string, set bson.M) (*models.StreakLog, error)   // filter {user_id, is_active:true}, returns after
func (s *Store) BreakStreak(ctx, userID string) error                                            // $set streak_achieve_days:0
func (s *Store) AdvanceStreakForDate(ctx, userID string, dateCovered time.Time) error            // pipeline update per inventory applyLifeline
func (s *Store) FindActiveStreaksWithDays(ctx) ([]models.StreakLog, error)                        // is_active, streak_achieve_days>0, proj user_id,last_date_of_log
// streak masters
func (s *Store) FindStreakMasterBySlug(ctx, slug string, activeOnly bool) (*models.StreakMaster, error)
func (s *Store) FindStreakMasterByDays(ctx, days int) (*models.StreakMaster, error)              // is_active:true
func (s *Store) FindStreakMasterByID(ctx, id primitive.ObjectID, superadminOnly bool) (*models.StreakMaster, error)
func (s *Store) FindSuperadminStreakMasters(ctx) ([]models.StreakMaster, error)
func (s *Store) UpsertStreakMasterBySlug(ctx, slug string, setOnInsert *models.StreakMaster) (*models.StreakMaster, error)
// reward transactions
type Balance struct{ BalanceCoins float64; EarliestExpiry *time.Time }
func (s *Store) ActiveCoinBalance(ctx, userID string, now time.Time) (Balance, error)             // aggregation §3.1 q1 (+ $min expire_at)
func (s *Store) InsertRewardTransaction(ctx, doc *models.RewardTransaction) (*models.RewardTransaction, error)   // returns doc; caller checks IsDuplicateKey
func (s *Store) ExistsCreditTransactionWithRemarks(ctx, userID, remarks string) (bool, error)
func (s *Store) FindCreditTransactionByRemarks(ctx, userID, remarks string) (*models.RewardTransaction, error)
func (s *Store) FindRewardTransactionsByUser(ctx, userID string, sortNewestFirst bool, projection bson.M) ([]models.RewardTransaction, error)
func (s *Store) ExistsRewardTxnForMasters(ctx, userID string, masterIDs []primitive.ObjectID) (bool, error)
func (s *Store) EarliestExpiringUnusedCredit(ctx, userID string, dayStart time.Time) (*models.RewardTransaction, error)
func (s *Store) RewardTxnsCreatedBetween(ctx, userID string, from, to time.Time) ([]time.Time, error)   // status success, proj createdAt
func (s *Store) FIFOOpenCredits(ctx, userID string, now time.Time, sortByExpiry bool) ([]models.RewardTransaction, error) // status success, expire_at>now, all_coins_used false; sort createdAt asc (app-backend) or expire_at asc
func (s *Store) PushDebit(ctx, txnID primitive.ObjectID, d models.DebitTransaction, allUsed bool, totalDebit int) error
func (s *Store) RewardTxnExpiriesByIDs(ctx, ids []primitive.ObjectID) (map[primitive.ObjectID]*time.Time, error)
type CoinTxnRow struct{ TxnCoinAmount int `json:"txnCoinAmount"`; TxnType string `json:"txnType"`; TxnStatus string `json:"txnStatus"`; Remarks string `json:"remarks"`; ExpiryDate any `json:"expiryDate"`; TxnDate time.Time `json:"txnDate"`; ShowCoins bool `json:"showCoins"`; Text string `json:"text"`; StreakName string `json:"streakName"` }
func (s *Store) CoinTransactionPage(ctx, userID string, now time.Time, page, limit int) (rows []CoinTxnRow, total int, err error)   // $unionWith aggregation §3.8
func (s *Store) StreakMasterNames(ctx, ids []primitive.ObjectID) (map[primitive.ObjectID]string, error)
// redeem
func (s *Store) InsertRedeemTransaction(ctx, doc *models.RedeemTransaction) (*models.RedeemTransaction, error)
func (s *Store) FindRedeemTransactionsByUser(ctx, userID string) ([]models.RedeemTransaction, error)
func (s *Store) ExistsRedemptionForOrder(ctx, userID, orderID string) (bool, error)
func (s *Store) ExistsRedemptionForShopfloTxn(ctx, userID, txn string) (bool, error)
// archived
func (s *Store) ArchivedProductIDs(ctx, userID string) (map[string]bool, error)
func (s *Store) AddArchivedProduct(ctx, userID, productID string, now time.Time) ([]string, error)     // $addToSet upsert, returns after
func (s *Store) RemoveArchivedProduct(ctx, userID, productID string, now time.Time) ([]string, error)  // $pull no upsert; nil doc → []
// lifeline logs
func (s *Store) CreateLifelineLog(ctx, doc *models.LifelineLog) (created bool, err error)   // 11000 → false,nil
func (s *Store) LifelineDates(ctx, userID string) ([]time.Time, error)
func (s *Store) CountLifelinesSince(ctx, userID string, since time.Time) (int64, error)
// scratch cards
func (s *Store) CreateScratchCard(ctx, doc *models.ScratchCard) (*models.ScratchCard, error)
func (s *Store) FindMintedCardForRewardDay(ctx, userID, source, day string) (*models.ScratchCard, error)
func (s *Store) FindScratchCardsForUser(ctx, userID string) ([]models.ScratchCard, error)      // sort createdAt desc
func (s *Store) FindCardByIDForUser(ctx, id primitive.ObjectID, userID string) (*models.ScratchCard, error)
func (s *Store) ClaimActiveCard(ctx, id primitive.ObjectID, userID string, now time.Time) (*models.ScratchCard, error)  // findOneAndUpdate; nil when not matched
func (s *Store) LinkRewardTransaction(ctx, cardID, txnID primitive.ObjectID) error
func (s *Store) ReopenClaimedCard(ctx, cardID primitive.ObjectID) error
// badges
func (s *Store) FindEarnedBadges(ctx, userID string) ([]models.Badge, error)
func (s *Store) UpsertEarnedBadge(ctx, userID string, kit int, now time.Time, source string) error
// customer activity logs
func (s *Store) FindCustomerActivityLog(ctx, caseID, event string, createdBetween *[2]time.Time) (*models.CustomerActivityLog, error)
func (s *Store) LatestReminderInfo(ctx, caseID string) (*models.CustomerActivityLog, error)   // reminder_days exists & non-empty, newest
// tasks
func (s *Store) CompleteUserTask(ctx, userID, caseID, taskName, completedBy string, now time.Time) error  // port from api-server server/components/user_task/handler.js updateTaskForUserToDisplay — read it; task_master lookup by name, user_task_details set is_active:false, completed_date, completed_by, $inc click_count
```

- [ ] **Step 1: Integration tests** for the non-trivial ones: `ActiveCoinBalance` (expired/used excluded), `CoinTransactionPage` (Credit + Debit + synthesised Expired row, sort, pagination, missing master → `streakName:""`), `FIFOOpenCredits` order, `ClaimActiveCard` race (two goroutines → one non-nil), `CreateLifelineLog` duplicate → `false,nil`, `AddArchivedProduct` upsert then `RemoveArchivedProduct`.
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(repositories): mongo repositories for BAH collections`

`CoinTransactionPage` aggregation sketch:
```
[ {$match:{user_id}}, {$lookup:{from:"streak_masters", localField:"streak_master_id", foreignField:"_id", as:"sm"}},
  {$addFields:{streakName:{$ifNull:[{$arrayElemAt:["$sm.display_name",0]},""]}}},
  {$project: credit row fields with literals txnType:"Credit", showCoins:true, text:"", remarks:{$ifNull:["$credit_remarks",""]}, expiryDate:{$ifNull:["$expire_at",""]}, txnDate:"$createdAt", txnStatus:{$ifNull:["$status",""]}, txnCoinAmount:"$credit_coins", plus expiredRow flag: expire_at<now && !all_coins_used && status!="failure", remaining: credit-debit}},
  {$unionWith:{coll:"redeem_reward_transactions", pipeline:[{$match:{user_id}}, {$project debit row: txnType:"Debit", expiryDate:"", showCoins:{$ne:["$status","failure"]}, text:{$cond:[{$eq:["$status","failure"]},{$concat:[{$toString:"$redeemed_coins"}," coins redeemed on this order added back to wallet"]},""]}, streakName:""}]}},
  {$facet:{ rows:[{$sort:{txnDate:-1}}], total:[{$count:"n"}] }} ]
```
Synthesised Expired rows: emit them in Go from the credit rows carrying the `expiredRow` flag (`txnType:"Expired"`, `txnStatus:"expired"`, `remarks:"Coins Expired"`, `expiryDate: expire_at`, `txnDate: expire_at`), then sort and slice in Go — the dataset per user is small; the DB does the lookup work. Sort must be stable by `txnDate` desc.

---

## Task 8: Postgres repositories (Traya) and logearn tables

**Files:**
- Create: `repositories/pg/traya_orders.go`, `repositories/pg/traya_users_cases.go`, `repositories/pg/traya_products.go`, `repositories/pg/traya_reminders.go`, `repositories/pg/traya_forms.go`, `repositories/pg/logearn.go`, `repositories/pg/tenant_customers.go`, `*_test.go`

**Interfaces:**
```go
package pgrepo
type TrayaStore struct{ Pool *pgxpool.Pool }
type Order struct{ ID string; UserID string; CaseID string; Status string; CreatedAt time.Time; DeliveryDate *time.Time; OrderMeta map[string]any; IsBulkOrder bool; BulkOrderDuration int; OrderDisplayID string }
func (s *TrayaStore) NonVoidOrdersByUser(ctx, userID string) ([]Order, error)       // status != 'void' ORDER BY created_at DESC; cols id,status,created_at,delivery_date,order_meta,case_id
func (s *TrayaStore) NonVoidOrdersByCase(ctx, caseID string) ([]Order, error)
func (s *TrayaStore) LatestNonVoidUnknownOrders(ctx, userID string) ([]Order, error) // status NOT IN ('void','unknown','ghost'), created_at DESC, + is_bulk_order,bulk_order_duration,order_display_id
func (s *TrayaStore) FirstDeliveredDate(ctx, userID string) (*time.Time, error)      // status='delivered' ORDER BY delivery_date ASC LIMIT 1
func (s *TrayaStore) LatestOrderAndCount(ctx, userID string) (*Order, int, error)     // status!='void' newest (created_at, case_id, delivery_date) + count(*) all orders
type UserCase struct{ UserID, CaseID, PhoneNumber, Gender, Email, FirstName string }
func (s *TrayaStore) UserCaseByUserID(ctx, userID string) (*UserCase, error)          // cases JOIN users WHERE cases.user_id = $1 (first)
func (s *TrayaStore) UserCaseByCaseID(ctx, caseID string) (*UserCase, error)
func (s *TrayaStore) UserIDFromCaseID(ctx, caseID string) (string, error)
type ProductDesc struct{ ProductPrincipalID string; ProductPrice float64; ImageCDNPath, CartCDNImages, MobileImagePath, SingleHalfImages, SingleImages string; MedicineDisplayName, MedicineType, MedicineDescription, MedicineDetailedDisplayName, MedicineDosage, MedicineDosageCode, MedicineInfo, MedicineComposition string }
func (s *TrayaStore) ProductsByPrincipalIDs(ctx, ids []string) ([]ProductDesc, error) // product_sku_mapping JOIN medicine_master ON medicine_id = medicine_master.id
type ReminderRow struct{ Tag string; ActualDate *time.Time }
func (s *TrayaStore) FinishedReminders(ctx, userID string, since time.Time) ([]ReminderRow, error) // is_finished AND (tag IN ('Week #1','Week #3') OR actual_date >= since)
func (s *TrayaStore) FifteenDayCheckinSession(...)  // port buildFifteenDayCheckinFeedbackUrl DB parts — read api-server handler.js for buildFifteenDayCheckinFeedbackUrl before implementing; expose: HasCompletedFormSession(ctx, caseID, formID string) (bool, error); CreateFormSession(ctx, ...) (sessionID, syntheticID string, err error)

type LogEarnStore struct{ Write, Read *pgxpool.Pool }   // Read == Write when no replica
type DoseLog struct{ ID, CustomerID, OrderID string; LogDate time.Time; StreakDay, ZTier, INRCredited int; IsBackfill bool; CreatedAt time.Time }
type LedgerEntry struct{ ID, CustomerID string; OrderID *string; Amount int; Reason string; BalanceAfter int; CreatedAt time.Time }
func (s *LogEarnStore) RecentDoseLogs(ctx, customerID string, limit int) ([]DoseLog, error)     // ORDER BY log_date DESC
func (s *LogEarnStore) CountDoseLogsSince(ctx, customerID string, since time.Time) (int, error)
func (s *LogEarnStore) InsertDoseLog(ctx, tx pgx.Tx, d *DoseLog) error
func (s *LogEarnStore) UpdateDoseLogStreakDay(ctx, tx pgx.Tx, id string, streakDay int) error
func (s *LogEarnStore) LatestLedger(ctx, customerID string) (*LedgerEntry, error)
func (s *LogEarnStore) LatestLedgerByReason(ctx, customerID, reason string) (*LedgerEntry, error)
func (s *LogEarnStore) ExistsLedgerReasonSince(ctx, customerID, reason string, since time.Time) (bool, error)
func (s *LogEarnStore) ExistsRedeemForOrder(ctx, customerID, orderID string) (bool, error)
func (s *LogEarnStore) WithTx(ctx, fn func(pgx.Tx) error) error
func (s *LogEarnStore) AppendLedger(ctx, tx pgx.Tx, customerID string, orderID *string, amount int, reason string) (balanceAfter int, err error)  // SELECT balance_after ... ORDER BY created_at DESC LIMIT 1 FOR UPDATE; insert; return
type CustomerStore struct{ Pool *pgxpool.Pool }
func (s *CustomerStore) Gender(ctx, customerID string) (string, error)    // customer.gender default 'M'
```

- [ ] **Step 1: Integration tests** (`TEST_PG_URI`): apply `migrations/pg/logearn/001_dose_logs_cash_ledger.sql` (write it now — DDL from inventory logearn §2 plus the redeem unique index), test `AppendLedger` under 20 concurrent credits sums correctly; `InsertDoseLog` twice same `(customer, log_date)` → unique violation.
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(repositories): traya postgres reads and logearn ledger store`

---

## Task 9: Tenant registry and middleware

**Files:**
- Create: `tenant/registry.go`, `tenant/context.go`, `tenant/middleware.go`, `tenant/registry_test.go`

**Interfaces:**
```go
package tenant
type Economy string
const ( Legacy Economy = "legacy"; Habit Economy = "habit"; LogEarn Economy = "logearn" )
type AuthMode string
const ( AuthJWT AuthMode = "jwt"; AuthGateway AuthMode = "gateway" )
type Tenant struct { ID, Name string; Economies map[Economy]bool; AuthMode AuthMode; Mongo *mongo.Database; PG *pgxpool.Pool; PGRead *pgxpool.Pool }
func (t *Tenant) Has(e Economy) bool
type Registry struct { /* cfg, master client, trayaPG pool, caches: map[string]*Tenant, sync.RWMutex, negative cache with time */ }
func NewRegistry(cfg *setup.Config, master *mongo.Client, trayaPG *pgxpool.Pool) *Registry
func (r *Registry) Resolve(ctx context.Context, id string) (*Tenant, error)   // master.tenant lookup; builds Tenant; EnsureIndexes in background once; ErrUnknownTenant
var ErrUnknownTenant = errors.New("tenant not found")
const Header = "x-tenant-id"
func Middleware(r *Registry, defaultTenant string) fiber.Handler   // header missing → defaultTenant when non-empty else 400 {"message":"Tenant ID header missing"}; unknown → 400 {"message":"Tenant not found: <id>"}; stores in c.Locals("tenant")
func From(c *fiber.Ctx) *Tenant
func RequireEconomy(e Economy) fiber.Handler // 404 {"message":"Not available for tenant <id>"}
```
Economies come from `cfg.TenantEconomies[id]`; AuthMode = `jwt` when economies include legacy or habit, else `gateway`. PG: `traya` → `trayaPG`; others → `setup.ConnectPG` on `cfg.TRPGWrite` with `Database = id + "_" + cfg.TRPGSuffix` (and read replica when configured), cached.

- [ ] **Step 1: Test** with a fake resolver: `Middleware` sets Locals and `RequireEconomy(LogEarn)` on a legacy-only tenant → 404 with exact message; missing header → 400 exact message. Use `app.Test`.
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(tenant): master registry, per-tenant datastores, middleware`

---

## Task 10: Auth strategies

**Files:**
- Create: `auth/identity.go`, `auth/jwt_traya.go`, `auth/v2token.go`, `auth/internal_token.go`, `auth/gateway.go`, `auth/middleware.go`, `auth/auth_test.go`

**Interfaces:**
```go
package auth
type Identity struct { UserID, CaseID, Email, FirstName, Phone, Gender string; Roles []string; RawToken string; AppVersion int }
func From(c *fiber.Ctx) *Identity              // c.Locals("identity"); never nil after a Require* middleware
type Verifier struct { Secret []byte; Redis *redis.Client; V2Token, InternalToken string; AdminGuard bool }
func (v *Verifier) RequireJWT() fiber.Handler       // see spec §5; sets Identity from claims id,caseId,email,roles,first_name,phone_number; RawToken = token; AppVersion from header x-app-version / query appVersion / query version (first non-empty, Number, 0 on NaN)
func (v *Verifier) RequireV2Token() fiber.Handler   // Identity with only AppVersion; caseId comes from path in controller
func (v *Verifier) RequireInternal() fiber.Handler  // x-internal-token; Identity.UserID from query userId / body userId (parsed by controller)
func (v *Verifier) RequireGateway() fiber.Handler   // x-user-info JSON {customer_id, case_id, gender, first_name} else query customerId → UserID; missing → 400 {"message":"customerId is required","statusCode":400,"timestamp":...,"path":...}
func (v *Verifier) RequireAdmin() fiber.Handler     // no-op unless AdminGuard; roles[0] in {ADMIN,SUPER_ADMIN,TEAM_LEAD} else 403 {"Error":"Only admin and super admin are allowed"}
```
JWT errors: no `Authorization` header → 401 `{"message":"No authorization header provided."}`; not `Bearer`, bad signature, expired, or Redis `user!<id>login!status` empty → 401 `{"message":"Invalid or expired token."}`. Redis errors (connection) → 401 as well but logged at error level.

- [ ] **Step 1: Tests**: sign a token with `jwt.SigningMethodHS256` claims `{id:"u1", caseId:"c1", email:"e"}`; with a `miniredis` (add `github.com/alicebob/miniredis/v2` test dep) key set → 200 and identity populated; key missing → 401 message; wrong secret → 401; no header → exact message; V2 token match/mismatch; gateway header parse; admin guard on/off.
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(auth): per-tenant auth strategies (traya JWT, V2 token, internal token, gateway)`

---

## Task 11: Kit calculator (shared order math)

**Files:**
- Create: `internal/orders/order.go`, `internal/orders/variants.go`, `internal/orders/kit_calculator.go`, `internal/orders/kit_calculator_test.go`

**Interfaces:**
```go
package orders
type Order = pgrepo.Order   // alias; mool/acne orders are mapped into the same struct by tr_orderservice
type LineItem struct{ VariantID int64; ProductID string; Quantity int; Name string }
func LineItems(o Order) []LineItem   // from OrderMeta["line_items"]; quantity default 1; name = name || title
var VariantIDsArray []int64          // copy VARIANT_IDS_ARRAY verbatim from /Users/mishika/Desktop/traya/traya-api-server/server/utils/config.js
var VariantIDMapping map[int64]int64 // copy VARIANT_ID_MAPPING verbatim
const HairVitaminVariant int64 = 44396552913074; const DiscontinuedVitaminVariant int64 = 37547237310642
type KitDetail struct{ KitCount int; DiffDays int; IsOrderDelivered bool; OrderStatus string; ProductDetails []LineItem }
func GetKitDetail(o Order, now time.Time) KitDetail        // inventory legacy §(kit_calculator.getKitDetail): deliveryDate = delivery_date||created_at; diff = CalendarDaysDifference(deliveryDate, now); kitCount = max qty among line items whose variant ∈ VariantIDsArray
type NonVoidDetails struct{ TotalKitCount, TotalDaysForUsingKit, RunningMonthForHairKit, KitExpireDays, MinDaysAfterOrderDelivered, RunningWeek, RunningWeekinMonthForHairKit int; IsOrderPlaced, IsAnyOrderDelivered bool; LatestOrderWithKitCount int; LatestKitOrderDeliveryDate, FirstKitOrderDeliveryDate, LatestBulkKitDeliveryDate *time.Time; LatestBulkKitCount int; IsLatestOrderBulkKit bool }
func GetAllDetailsRelatedToNonVoidOrders(orders []Order, now time.Time) NonVoidDetails   // port ENTIRE function from /Users/mishika/Desktop/traya/traya-api-server/server/components/customer_computed_data/kit_calculator.js (read to the end; the inventory shows only the first 90 lines) — MinDaysAfterOrderDelivered defaults -1
func CurrentKitStart(orders []Order, today time.Time, kitCount func(Order) int) *time.Time  // habitKitWindow.currentKitStart
type KitWindow struct{ KitNumber int; Start, End time.Time }
func HabitTrackerKitWindows(orders []Order, now time.Time) []KitWindow      // app-backend orderDataService.getHabitTrackerKitWindows (read lines 295-340)
func CurrentRunningKitStartDate(orders []Order, now time.Time) *time.Time   // orderDataService.getCurrentRunningKitStartDate (read line 337+)
func LastDelivered(orders []Order) *Order                                   // status delivered, max(delivery_date||created_at)
```

- [ ] **Step 1: Tests**: port every case from `/Users/mishika/Desktop/traya/traya-api-server/server/components/BAH/test/habitKitWindow.test.js` and any `kit_calculator` tests found under `server/components/customer_computed_data/test` (grep). Add: single delivered kit 10 days ago → `MinDaysAfterOrderDelivered 10, KitExpireDays 30, RunningWeek 2`; 2-kit bulk order → `KitExpireDays 60`; undelivered newest order → `IsOrderPlaced true`.
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(orders): kit calculator, kit windows and variant tables`

---

## Task 12: Order sources (Traya PG, tr order-service, config-service map)

**Files:**
- Create: `internal/orders/traya_source.go`, `internal/orders/tr_orderservice.go`, `internal/orders/config_service.go`, `internal/orders/tr_orderservice_test.go` (httptest)

**Interfaces:**
```go
type TRAppOrder struct { ID string `json:"id"`; Status string `json:"status"`; DeliveryDate *string `json:"delivery_date"`; CreatedAt string `json:"created_at"`; BulkOrderDuration any `json:"bulk_order_duration"` }
type TROrderClient struct{ HTTP *http.Client; BaseURL string }
func (c *TROrderClient) NonVoidOrdersByCustomer(ctx, tenantID, customerID string) ([]TRAppOrder, error)   // GET /orders/orders/get-all-orders-for-app/{customerId}?voidType=nonVoid, header x-tenant-id
func (c *TROrderClient) OrderDetails(ctx, tenantID, orderID string) (map[string]any, error)           // the long columns/joins URL from inventory logearn §3
type ConfigServiceClient struct{ HTTP *http.Client; BaseURL string; cache sync.Map /* key tenant → {map, expiry} */ }
func (c *ConfigServiceClient) OldToActiveVariantMap(ctx, tenantID string) (map[string]map[string]any, error)  // GET /static-content/data/OLD_TO_ACTIVE_VARIANT_IDS_MAP; response {message, data:<content>}; cache 10 min; on error return last good or empty
func ActiveVariantID(id int64, m map[string]map[string]any) int64     // m[id].associatedTo || VariantIDMapping[id] || id
```
Order-service responses are untrusted input: decode defensively; unknown shapes → error, never panic.

- [ ] **Step 1: Tests** with `httptest.Server`: header `x-tenant-id` forwarded; path exact; malformed JSON → error; config map cache hit within 10 min (count server hits).
- [ ] **Step 2–4:** implement; PASS.
- [ ] **Step 5: Commit** `feat(orders): tr order-service client and config-service variant map`

---

## Task 13: Legacy constants and copy

**Files:**
- Create: `internal/legacy/constants.go`, `internal/legacy/constants_test.go`

**Interfaces:** every constant from `inventory-api-server-legacy-bah.md` §6 and copy from §3.1 (`popupText`, modal media URLs, banner copy), §3.3, §3.4, §3.11 texts, §3.16 texts. Names:
```go
var StreakRewards = []struct{Streak, Coin int}{{3,100},{7,400},{21,2000}}
var O8PlusStreakRewards = map[int]int{3:200,7:600,21:2500}
const O8PlusGoLive = "2026-04-10"; O8PlusWindowDays = 15; O8PlusMinOrders = 8
var O8PlusVariationGroup = []string{"2","3","4","5","6","7","8","9"}
const StreakRestartBonusEnabled = false; StreakRestartBonusFemaleEnabled = true; StreakRestartBonusCoins = 100
var StreakRestartGroup = []string{"0","1","a","b","c","d","e","f"}; var StreakRestartFemaleOrderCounts = []int{1}
const CoinDiscountCapValue = 25; CoinConversionRatio = "0.1"; AutoApplyCoins = true
const Coins800StreakRewardsID = "reorder_800_coin_experiment"; FeatureUpdateCutoff = "2025-05-20"
var CoinCreditAccessEmailIDs = []string{"vipinchauhan@traya.health","agarwalsandip@traya.health","sultantippu@traya.health","bharathputta@traya.health"}
const FifteenDayCheckinSubtext = "You are staying consistent. Let your coach know if you are facing any issues with treatment"
const FifteenDayFormID = "component-1782457562767-VdwcN0"; FifteenDayFormName = "15_Day_Checkin"
const CDNBase = "https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/"
const BahVideoFemale = "https://cdn.shopify.com/videos/c/o/v/639cdb2a63a245c2a46de9aa67bef697.mp4"; BahVideoMale = "https://cdn.shopify.com/videos/c/o/v/856c2a9f4dfb433b8dd99d3738621fc7.mp4"
var PopupText = map[string]any{ "_1_day": map[string]string{"h1":"YOU WON 100 COINS! 🎊","h2":"Redeem the coins before making final payment for next order. 10 coins = ₹1"}, ... "ctaText":"HOW TO REDEEM COINS?", "dismissText":"DISMISS" }   // use an ordered struct instead of map for stable JSON
const TaskBuildAHabbitSticky = "build_a_habbit_sticky"; TaskBuildAHabit = "build_a_habit"; TaskMaleKitStarted = "male_kit_started"; TaskFemaleKitStarted = "female_kit_started"
// dosage codes
const DosageOnceAWeek="ONCE_A_WEEK"; DosageTwiceAWeek="TWICE_A_WEEK"; DosageThriceAWeek="THRICE_A_WEEK"; DosageTwiceOrThriceAWeek="TWICE_OR_THRICE_A_WEEK"; Dosage100="1-0-0"; Dosage200="2-0-0"; Dosage010="0-1-0"; Dosage001="0-0-1"; Dosage002="0-0-2"; Dosage101="1-0-1"; Dosage202="2-0-2"; Dosage111="1-1-1"; Dosage1ml00="1ml-0-0"; Dosage001ml="0-0-1ml"; Dosage1ml01ml="1ml-0-1ml"; DosageAsDirected="AS_DIRECTED"
// idempotency keys (spec §4)
const RemarkFirstLog = "bah-legacy-first-log"
func RemarkMilestone(slug, istDate string) string  // "bah-legacy-<slug>-<date>"
func RemarkRestart(istDate string) string          // "bah-legacy-restart-<date>"
```
Struct types with ordered JSON for `PopupText`, modal payloads (Task 16) live here too.

- [ ] **Step 1: Test** asserts a handful of strings verbatim (popup h1s, `FifteenDayCheckinSubtext`, `RemarkMilestone("existing_coins_streak","2026-09-18") == "bah-legacy-existing_coins_streak-2026-09-18"`).
- [ ] **Step 2–4:** write; PASS. **Step 5: Commit** `feat(legacy): constants, copy strings and idempotency keys`

---

## Task 14: Legacy streak engine

**Files:**
- Create: `internal/legacy/service.go` (Service struct + constructor), `internal/legacy/streak.go`, `internal/legacy/streak_test.go`

**Interfaces:**
```go
package legacy
type Deps struct { Store *mongorepo.Store; PG *pgrepo.TrayaStore; Redis *redis.Client; Clock common.Clock; Cfg *setup.Config; HTTP *setup.HTTPClients; Config *orders.ConfigServiceClient; Habit HabitCreditor; CCD CCDPublisher; Log *slog.Logger }
type HabitCreditor interface{ MintOrCredit(ctx context.Context, in HabitCreditInput) (HabitCreditResult, error) }   // implemented by internal/habit (Task 25); avoids import cycle
type HabitCreditInput struct{ UserID string; StreakDay int; PhoneNumber string; CheckInDate time.Time }
type HabitCreditResult struct{ Success bool `json:"success"`; Minted *bool `json:"minted,omitempty"`; Credited *bool `json:"credited,omitempty"`; Paused *bool `json:"paused,omitempty"`; AlreadyCredited *bool `json:"alreadyCredited,omitempty"`; AlreadyExists *bool `json:"alreadyExists,omitempty"`; CardID string `json:"cardId,omitempty"`; Coins *int `json:"coins,omitempty"`; Type string `json:"type,omitempty"`; Card any `json:"card,omitempty"`; Message string `json:"message,omitempty"` }
type CCDPublisher interface{ Publish(ctx context.Context, tenantID, eventType, caseID string, payload map[string]any) }
type Service struct{ Deps; TenantID string }
func New(t *tenant.Tenant, d Deps) *Service   // wires Store from t.Mongo, PG from t.PG

type StreakResult struct{ Streak *models.StreakLog; IsStreakBreaked bool; AlreadyLogged bool }
func (s *Service) CreateStreakLogForUser(ctx, userID string, checkInDate time.Time, isValidForStreak bool, caseID string) (StreakResult, error)  // inventory §3.5 createStreakLogForUser; filter adds is_active:true; emits CCD ACTIVITY_LOG {logDate, streakDate, streakCount}
type RunningLogDay struct{ LogRunningDay any `json:"logRunningDay"`; UserHasUsedBAH bool `json:"userHasUsedBAH"` }   // number or "regular_day" or 0
func (s *Service) GetBahRunningLogDay(ctx, userID string) (RunningLogDay, error)
func StreakBrokenRecompute(current int, lastLog *time.Time, now time.Time) int   // version>=71 rule: diff>2 || (current==21 && diff==1) → 0
func IsLoggedToday(lastLog *time.Time, now time.Time) bool                       // UTC Y/M/D equality (JS getDate/Month/FullYear with process TZ UTC)
```

- [ ] **Step 1: Tests** (integration for Create*, unit for pure): no streak → create with days 1; same-day → `AlreadyLogged`; gap 1 → +1; gap 2 → reset to 1 with `first_date_of_log` = checkInDate and `IsStreakBreaked`; at 21 → next log resets to 1; `longest_streak_days` tracks max; CCD publisher fake receives `ACTIVITY_LOG` with `streakCount`.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): streak engine with CCD emit`

---

## Task 15: Legacy rewards, Shopflo, CCD, tasks

**Files:**
- Create: `internal/legacy/rewards.go`, `internal/legacy/shopflo.go`, `internal/legacy/ccd.go`, `internal/legacy/tasks.go`, `internal/legacy/cohorts.go`, tests

**Interfaces:**
```go
type ShopfloClient struct{ HTTP *http.Client; Endpoint, IssuerID, MerchantID, APIKey string; Log *slog.Logger }
func (c *ShopfloClient) EnsureWallet(ctx, phone string) error                       // GET user-wallet?phone-number=; 404 → POST create {oid, initial_balance:0, wallet_type:"REWARDS"}
func (c *ShopfloClient) Credit(ctx, coins int, expiryEpochMS int64, referenceID, phone string) error   // body per inventory §4.1; 404/400 swallowed
func (c *ShopfloClient) Debit(ctx, coins int, referenceID, phone, reference string) error              // 404 → BadRequest("No record found for this user"); 400 → BadRequest("User have less coin balance to debit")
type Wallet struct{ TotalWalletBalance float64 `json:"total_wallet_balance"` /* + raw map */ ; Raw map[string]any }
func (c *ShopfloClient) Read(ctx, phone string) (*Wallet, error)                    // prefixes +91 if missing; 404 → BadRequest("No record found for this user")
func NormalizePhone(p string) string                                               // +91 prefix rule

type RedisCCD struct{ R *redis.Client; Log *slog.Logger }
func (p *RedisCCD) Publish(ctx, tenantID, eventType, caseID string, payload map[string]any)   // channel "ccd_update", JSON {tenantId,eventType,caseId,payload,emittedAt}; errors logged

type SaveRewardInput struct{ UserID string; Master *models.StreakMaster; PhoneNumber string; Reason string; CustomAmount *int; CaseID string; ExpiryDaysOverride *int }
type SaveRewardResult struct{ Ref *models.RewardTransaction; Duplicate bool }
func (s *Service) SaveRewardTransaction(ctx, in SaveRewardInput) (SaveRewardResult, error)   // §3.14: expiry days from order-service (2 for reorder_800…; ExpiryDaysOverride for sync); expire_at = UTCMidnight(ISTShift(now)+1d+days); insert; duplicate key → Duplicate:true (no CCD/Shopflo); else CCD COIN_CREDITED {amount}; EnsureWallet + Credit best-effort (logged)
func (s *Service) CoinExpiryDays(ctx, userID string) (int, error)     // GET ORDER_SERVICE_BASE_URL/coin/expiry/month/<userId>
func (s *Service) CreditRewardCoinsToUser(ctx, userID string, continuousDays int, phone string, isHabitTracker bool, checkInDate time.Time) (*HabitCreditResult, error)   // §3.5a; legacy path uses RemarkFirstLog / RemarkMilestone(master.Slug, ISTDateString(checkInDate))
func (s *Service) CreditStreakRestartBonus(ctx, userID, phone string, amount int, checkInDate time.Time) error   // master existing_coins_streak, reason "Streak restart bonus" — NOTE: idempotency key: use Reason "Streak restart bonus" for credit_remarks parity? DECISION: keep credit_remarks "Streak restart bonus" (display parity) and guard with an explicit ExistsCreditTransactionWithRemarks check on RemarkRestart stored in a NEW field? No — simpler: credit_remarks = "Streak restart bonus" and pre-check `RewardTxnsCreatedBetween` same IST day for that master; document in code.
// cohorts
func CheckO8PlusEligibility(orders []orders.Order, now time.Time) bool
func IsMaleStreakRestartCohort(caseID, gender string) bool
func IsFemaleStreakRestartCohort(caseID, gender string, orderCount int) bool
func (s *Service) ResolveStreakRestartBonusAmount(ctx, userID, caseID, gender string) (*int, error)
type FifteenDayEligibility struct{ W1NotCompleted, W3NotCompleted, NoRecentCall bool }
func (s *Service) CheckFifteenDayCheckinCallEligibility(ctx, userID string) (FifteenDayEligibility, error)
func (s *Service) BuildFifteenDayCheckinFeedbackURL(ctx, caseID, userID, gender, authToken string) (string, error)   // port from api-server handler.js buildFifteenDayCheckinFeedbackUrl (read it); FEEDBACK_UI_DOMAIN by IsProduction
// tasks
func (s *Service) UpdateTaskForUserToDisplay(ctx, userID, caseID, taskName string) error   // Store.CompleteUserTask(..., "CUSTOMER")
func GenderOf(g string) string   // upper(g) if "M"/"F" else "M"
```

- [ ] **Step 1: Tests**: Shopflo client against `httptest` (URL template, raw Authorization header, amount /10, 404 handling); `SaveRewardTransaction` integration: expiry = UTC midnight of IST(now)+1d+N; duplicate remark → `Duplicate:true` and no second CCD publish (fake publisher counter); `CheckO8PlusEligibility` table (7 orders → false; caseId first char '1' → false; delivery outside window → false; inside → true); cohorts table.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): reward credits with idempotency, shopflo, ccd publisher, cohorts, tasks`

---

## Task 16: Legacy banners, modals and copy engines

**Files:**
- Create: `internal/legacy/banner.go`, `internal/legacy/banner_test.go`

**Interfaces:**
```go
type BannerWidgetData struct { Title string `json:"title"`; SubTitle string `json:"subTitle,omitempty"`; SubTitleIcon string `json:"subTitleIcon,omitempty"`; CtaLabel string `json:"ctaLabel"`; ShowBlueBar *bool `json:"showBlueBar,omitempty"`; CoinBalance string `json:"coinBalance,omitempty"`; StreakDays string `json:"streakDays,omitempty"`; ShowStreakTimeline *bool `json:"showStreakTimeline,omitempty"`; Icon string `json:"icon,omitempty"` }
func GetBannerWidgetData(in BannerInput) *BannerWidgetData   // read handler.js:4169-4240 to confirm EXACT key set per branch (omitempty must match which keys the JS sets; if JS sets `subTitle: undefined`, key is absent in JSON — use omitempty; if JS sets null, use a nullable field)
type BannerInput struct{ HasLoggedEver, IsBahLocked, LoggedToday, IsEligibleForStreakRestartBonus bool; CurrentStreak, RewardBalance int; LastLogDate *time.Time; Now time.Time; CDN string }
type ChallengeBanner struct{ Title string `json:"title"`; Subtitle string `json:"subtitle"`; Cta string `json:"cta"`; BgImg string `json:"bgImg"` }
func GetBahChallengeEntryPointBanner(streak int, hasLoggedEver, isLoggedToday, isMissedYesterday bool, s3 string) ChallengeBanner   // §3.3; read handler.js:3598-3850 for exact JSON keys (may include more, e.g. `ctaAction`) 
type PostLogContent struct{ Title, Description, Cta, CtaAction string }
func GetPostLoggingModalContent(currentStreak int, hasLoggedEver, isMissedYesterday, isStreakRestartBonus bool) PostLogContent   // §3.4 including streak 0 fallthrough
type ModalButton struct{ Label string `json:"label"`; Action string `json:"action"`; Variant string `json:"variant"`; URL string `json:"url,omitempty"` }
type Modals struct { LogDoneModal any `json:"logDoneModal"`; RewardsModal any `json:"rewardsModal"`; StreakRestartBonusModal any `json:"streakRestartBonusModal"`; ErrorModal any `json:"errorModal"`; StreakBrokeModal any `json:"streakBrokeModal"`; FeatureUpdateModal any `json:"featureUpdateModal"`; NewOrderModal any `json:"newOrderModal"`; MissedLogModal any `json:"missedLogModal"` }
func BuildModals(in ModalsInput) Modals   // §3.1 modals; use ordered structs per modal so key order matches JS insertion order
```

- [ ] **Step 1: Tests**: full copy tables from inventory §3.3 and §3.4 as table tests (every row); banner branches A–E with exact strings; `BuildModals` snapshot JSON for the eligible branch with `currentMilestone 7`, community button present, feedback button replacing primary.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): banner widget, challenge banner, post-log copy and modals`

---

## Task 17: `getStreakAndRewardBalance` and balance reads

**Files:**
- Create: `internal/legacy/balance.go`, `internal/legacy/balance_test.go`

**Interfaces:**
```go
type BahBanner struct{ Title string `json:"title"`; SubTitle string `json:"subTitle"`; CtaText string `json:"ctaText"`; Coins int `json:"coins"`; UnlockText string `json:"unlockText"` }
type CoinInfo struct{ Title string `json:"title"`; SubTitle string `json:"subTitle"` }
type StreakAndRewardBalance struct {
  RewardBalance int `json:"rewardBalance"`; CurrentDaysStreakCount int `json:"currentDaysStreakCount"`; LongestDaysStreakCount int `json:"longestDaysStreakCount"`
  ThreeDaysStreakCount int `json:"threeDaysStreakCount"`; SevenDaysStreakCount int `json:"sevenDaysStreakCount"`; TwentyOneDaysStreakCount int `json:"twentyOneDaysStreakCount"`
  LastLogDate *time.Time `json:"lastLogDate"`; FirstLogDate *time.Time `json:"firstLogDate"`; IsUserEligibleForNewBahFlow bool `json:"isUserEligibleForNewBahFlow"`
  BahBanner any `json:"bahBanner"`; BahTitle string `json:"bahTitle"`; PopupText PopupTextT `json:"popupText"`
  CoinDiscountCap struct{ Value int `json:"value"`; Type string `json:"type"` } `json:"coinDiscountCap"`; CoinConversionRatio string `json:"coinConversionRatio"`
  CoinNotApplied CoinInfo `json:"coinNotApplied"`; CoinApplied CoinInfo `json:"coinApplied"`; AutoApplyCoins bool `json:"autoApplyCoins"`
  BahbannerNewTitle string `json:"bahbannerNewTitle"`; StreakRewardMessage string `json:"streakRewardMessage"`; HasUserSeenBahUpdatedModalResult bool `json:"hasUserSeenBahUpdatedModalResult"`
  BannerWidgetData *BannerWidgetData `json:"bannerWidgetData"`; ShowBAHLogMissedToUser bool `json:"showBAHLogMissedToUser"`; Modals Modals `json:"modals"`; ReminderOnBahPage string `json:"reminderOnBahPage"` }
func (s *Service) GetStreakAndRewardBalance(ctx, userID string, version int, authToken string) (*StreakAndRewardBalance, error)   // §3.1 end to end; read handler.js:1381-1700 alongside; lastLogDate/firstLogDate are nil when no streak (JS: undefined → key absent? check; if absent use omitempty)
func (s *Service) GetOnlyRewardBalance(ctx, userID string) (int, error)
func (s *Service) GetUserRewardBalance(ctx, caseOrUserID string) (int, error)   // caseId → userId; if lookup fails and the value looks like a userId, use directly (internal-service /rewardBalance/:customerId)
func (s *Service) GetEarliestExpiringUnusedCoins(ctx, userID string) (*ExpiringReward, error)
type ExpiringReward struct{ RemainingCoins int; ExpiringOn string }
func (s *Service) HasUserSeenBahUpdatedModal(ctx, userID string, hasLoggedEver bool, lastLog *time.Time) (showMissed bool, err error)
```

- [ ] **Step 1: Tests** (integration with seeded Mongo + fake PG store interface — introduce `TrayaPG` interface in `service.go` so tests can stub orders/users): never logged → banner A, `bahBanner.coins 100`, `modals.rewardsModal.description.firstLog`; version 70 → `bannerWidgetData` null and no recompute; version 71 + lastLog 3 days ago → streak 0 and banner "Your streak broke…"; milestone 7 with authToken → community button URL exact; JSON snapshot key order equals inventory response shape.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): streakAndRewardBalance, balance helpers`

---

## Task 18: Medicines, how-to-use and recommendation proxies

**Files:**
- Create: `internal/legacy/medicines.go`, `internal/legacy/howtouse.go`, tests

**Interfaces:**
```go
type Medicine struct { ProductID string `json:"product_id"`; Name string `json:"name"`; Type string `json:"type"`; Dosage string `json:"Dosage"`; DosageCode string `json:"dosageCode"`; Info string `json:"info"`; Description string `json:"description"`; Composition string `json:"composition"`; Price float64 `json:"price"`; ItemCount int `json:"itemCount"`; ImageURL ImageURL `json:"image_url"`; CartDisplayName string `json:"cartDisplayName"`; NewlyAdded bool `json:"newlyAdded"` }
type ImageURL struct{ ProductURL string `json:"productUrl"`; CartImgURL string `json:"cartImgUrl"`; MobileImgURL string `json:"mobileImgUrl"`; SingleHalfImages string `json:"singleHalfImages"` }
type BahV3ReorderText struct{ H1 string `json:"h1,omitempty"`; H2 string `json:"h2,omitempty"`; IsBahLocked *bool `json:"isBahLocked,omitempty"` }   // {} when empty
type LatestMedicines struct { Medicines []Medicine `json:"medicines"`; LogDaysLeft int `json:"logDaysLeft"`; Text1 string `json:"text1"`; Text2 string `json:"text2"`; IsReorderRequired bool `json:"isReorderRequired"`; ShowText bool `json:"showText"`; CtaText string `json:"ctaText"`; IsMedicineLocked *bool `json:"isMedicineLocked,omitempty"`; BahV3ReorderText *BahV3ReorderText `json:"bahV3ReorderText,omitempty"` }
func (s *Service) GetLatestMedicines(ctx, userID string, showBahV3 bool) (*LatestMedicines, error)     // §3.11
func (s *Service) GetPrescriptionForMedicinesByOrders(ctx, ords []orders.Order) ([]Medicine, error)  // getMedicineIds + getProductDesc; newlyAdded first
type HowToUse struct { MedicinesPrescription []Medicine `json:"medicinesPrescription"`; IsPrescriptionLocked bool `json:"isPrescriptionLocked"`; LatestOrderID string `json:"latestOrderId"`; LatestOrderDisplayID string `json:"latestOrderDisplayId"`; LatestOrderStatus string `json:"latestOrderStatus"`; LatestOrderDate *time.Time `json:"latestOrderDate"`; HowToUseText string `json:"howToUseText"`; ShowNew bool `json:"showNew"` }
func (s *Service) GetMedicinesForHowToUsePurpose(ctx, userID string) (*HowToUse, error)               // §3.12; read handler.js:947-1025 for the exact initial values of latest* fields when no orders
type ProxyResult struct{ Status int; Body []byte }
func (s *Service) RecommendationGet(ctx, path string, query url.Values) (ProxyResult, error)          // headers Content-Type, Authorization Bearer V2 token, x-tenant-id traya; returns upstream status+body verbatim
func (s *Service) LatestOrderHowToUseV2(ctx, caseID string, query url.Values) (ProxyResult, error)    // how-to-use/{caseId}; on 2xx parse JSON object and set reminderInfo from Store.LatestReminderInfo (read index.js route 8 for exact key names), re-marshal
func (s *Service) LatestRoutineV2(ctx, caseID string, query url.Values) (ProxyResult, error)
```

- [ ] **Step 1: Tests**: `GetLatestMedicines` scenarios from §3.11 (no delivered → "Your Order is on its way"; delivered 25 days ago, 1 kit → reorder texts with `logDaysLeft 20`; locked case texts with `ctaText "Save My Streak!"`); `GetPrescriptionForMedicinesByOrders` vitamin dedupe and newlyAdded ordering; proxies via httptest (headers, status pass-through, reminderInfo merge).
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): latest medicines, how-to-use, recommendation proxies`

---

## Task 19: `bahLogForGivenDate` and the writes

**Files:**
- Create: `internal/legacy/logs.go`, `internal/legacy/logs_test.go`, `internal/legacy/workers.go`

**Interfaces:**
```go
type DosageText struct{ Text string `json:"text"`; IsMedicineLogged bool `json:"isMedicineLogged"`; LogTimeInDay string `json:"logTimeInDay"` }
// dosage rows are the prescription map plus subHeading and dosageDisplayText — use map[string]any copy to keep unknown keys
type BahLogForDate struct { DailyDosageTitle string `json:"dailyDosageTitle"`; DailyDosage []map[string]any `json:"dailyDosage"`; WeeklyDosageTitle string `json:"weeklyDosageTitle"`; WeeklyDosage []map[string]any `json:"weeklyDosage"`; IsPostApiNeeded bool `json:"isPostApiNeeded"`; IsPutApiNeeded bool `json:"isPutApiNeeded"`; OrderEligibleForLog bool `json:"orderEligibleForLog"`; MedicineReordertext any `json:"medicineReordertext"`; CoinExpiryText string `json:"coinExpiryText"`; BahChallengeEntryPointConfig ChallengeBanner `json:"bahChallengeEntryPointConfig"`; BahVideo struct{ Female string `json:"female"`; Male string `json:"male"` } `json:"bahVideo"`; ArchivedProduct []map[string]any `json:"archivedProduct"` }
func (s *Service) GetBahLogForGivenDate(ctx, date *time.Time, userID string, showBahV3 bool, caseID string, appVersion int) (*BahLogForDate, error)   // §3.2
func SplitDosage(items []map[string]any) (daily, weekly []map[string]any)   // pure; §3.2 tables
func IsOrderEligibleForLog(caseID string, now time.Time) bool

type LogActivityInput struct{ UserID, CaseID, Phone string; IsLogForToday bool; ProductPrescriptions []map[string]any; IsHabitTracker bool }
type LogActivityResult struct{ Message string `json:"message"`; ScratchCard any `json:"scratchCard,omitempty"` }   // scratchCard key present (null) on success path, absent on "cannot log" path — model with a custom MarshalJSON
func (s *Service) SaveActivityLogsAndCreateStreakAndGiveRewards(ctx, in LogActivityInput) (*LogActivityResult, error)   // §3.5 with fixes: IST date compare; habit path sync; legacy path via s.Dispatch
func (s *Service) SaveActivityLogs(ctx, userID string, checkInDate time.Time, pp []map[string]any) (*models.ActivityLog, error)   // nil,nil when duplicate; busts kit-tracker-calendar!<userId>
func (s *Service) ProcessStreakAndRewards(ctx, userID string, checkInDate time.Time, log *models.ActivityLog, isHabitTracker bool) (scratchCard any, err error)
type MultipleLogInput struct{ UserID string; IsLogForToday bool; LogProductDetail []map[string]any }
func (s *Service) UpdateMultipleMedicineLogForUser(ctx, in MultipleLogInput) (map[string]string, error)   // §3.7 forgiving semantics; returns {"message":"Medicine logged successfully for your products"}

// workers.go — bounded pool for the fire-and-forget legacy path
type Dispatcher struct{ /* chan func(), size, workers */ }
func NewDispatcher(workers, queue int, log *slog.Logger) *Dispatcher
func (d *Dispatcher) Go(fn func(ctx context.Context))   // enqueue; if full run inline; each job gets context.WithTimeout(background, 30s) and recover
func (d *Dispatcher) Shutdown(ctx context.Context)
```

- [ ] **Step 1: Tests**: `SplitDosage` table for all 16 dosage codes (weekly bucket, subHeading, text arrays); `GetBahLogForGivenDate` today with no log → `isPostApiNeeded true`, yesterday with today logged → false; appVersion 85 with archived ids → filtered + `archivedProduct` populated; POST flow integration: first log creates activity+streak+first-log credit (idempotent on replay: replay returns `User cannot log…`/duplicate silently and reward count stays 1); `isHabitTracker` true uses fake `HabitCreditor` and returns its card; yesterday log when today exists → message `User cannot log for date <JSDateString>` with no `scratchCard` key; multiple-log no-op cases return 200 message.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): bahLogForGivenDate, activity log write path, multiple-log update, dispatcher`

---

## Task 20: Coin transactions and calendar

**Files:**
- Create: `internal/legacy/cointxn.go`, `internal/legacy/calendar.go`, tests

**Interfaces:**
```go
type Pagination struct{ Page int `json:"page"`; Limit int `json:"limit"`; Total int `json:"total"`; TotalPages int `json:"totalPages"`; HasNextPage bool `json:"hasNextPage"`; HasPrevPage bool `json:"hasPrevPage"` }
type CoinTransactionPage struct{ RewardHistory []mongorepo.CoinTxnRow `json:"rewardHistory"`; Pagination Pagination `json:"pagination"` }
func (s *Service) GetRewardCoinHistoryPaginated(ctx, userID string, page, limit int) (*CoinTransactionPage, error)
type RewardCoinHistory struct{ RewardHistory []map[string]any `json:"rewardHistory"`; Text1 string `json:"text1"`; Text2 string `json:"text2"` }
func (s *Service) GetRewardCoinHistory(ctx, userID string, isCredit, isDebit bool) (*RewardCoinHistory, error)   // §3.16 (no streakName, no Expired rows)
type CalendarDay struct{ Date string `json:"date"`; Status string `json:"status"` }
type CalendarMonth struct{ Month string `json:"month"`; MonthData []CalendarDay `json:"monthData"` }
type CalendarResponse struct{ StartDate string `json:"startDate"`; EndDate string `json:"endDate"`; Data []CalendarMonth `json:"data"` }
func (s *Service) GetBahCalendarLogData(ctx, userID, date, mode string) (*CalendarResponse, error)   // §3.9; errors: BadRequest("Invalid date format."), BadRequest("No active streak found for this user."), BadRequest(`Invalid mode. Use "calendar" or "streak".`) — confirm status codes in handler.js:3401-3560 (the JS throws → 500 {error:'Internal Server Error', details}); mirror that: these become HTTPError{500, details}
func PaintCalendar(in PaintInput) map[string]string   // pure painting over [start..end] for tests
```

- [ ] **Step 1: Tests**: paginated page 2 of 8 rows limit 6 → 2 rows, `hasPrevPage true`; Expired synthesis; calendar painting table: inactive before delivery, `active` today when missed, yesterday `active` only when today not logged/coin, streak mode `inactive` after last log; month grouping label `"June 2025"`; `endDate` equals today.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): coinTransaction pagination and calendar`

---

## Task 21: CRM handlers

**Files:**
- Create: `internal/legacy/crm.go`, `internal/legacy/crm_test.go`

**Interfaces:**
```go
type StreakMasterOption struct{ StreakMasterID string `json:"streakMasterId"`; SlugName string `json:"slugName"`; DisplayName string `json:"displayName"`; RewardCoins int `json:"rewardCoins"` }
func (s *Service) GetMasterStreakDataForExtraBonusStreak(ctx) ([]StreakMasterOption, error)
type ActivityLogsMonth struct{ ActivityLogs map[string][]map[string]any `json:"activityLogs"`; ActivityLogTime map[string]*time.Time `json:"activityLogTime"`; IsYesterdayLogExist bool `json:"isYesterdayLogExist"` }
func (s *Service) GetActivityLogs(ctx, userID string, year, month *int) (*ActivityLogsMonth, error)   // read handler.js getActivityLogs for exact range + IST time via ISTShift
func (s *Service) GetBahHistoryOfUser(ctx, caseID string, isCredit, isDebit bool, year, month *int, loggedInEmail string) (map[string]any, error)   // §3.16: spread-merge of 4 results + latestMedicines, isBalanceSyncingRequired:false, shopFloCoins, isUserAuthorizedToGiveCoins, shopFloError
type ExtraRewardResult struct{ RewardRef any `json:"rewardRef"`; Message string `json:"message"` }
func (s *Service) SaveExtraBonusForUsersForAnyReason(ctx, caseID, streakMasterID, reason string, customAmount *int) (*ExtraRewardResult, error)
func (s *Service) SyncRewardBalanceWithShopflo(ctx, caseID string) (map[string]string, error)   // §3.16
```

- [ ] **Step 1: Tests**: extraRewards validation messages (`All fields are mandatory`, `Invalid streak master id`, `Invalid streak master id for custom amount`); bahHistory merge keys present; sync with fake Shopflo wallet balance 50 (=500 coins) vs local 300 → local credit 200 with remarks `Credit reward to sync balance`.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(legacy): CRM handlers (bahHistory, extra rewards, shopflo sync, streak masters)`

---

## Task 22: Habit constants, reward math, streak masters

**Files:**
- Create: `internal/habit/constants.go`, `internal/habit/reward.go`, `internal/habit/service.go`, tests

**Interfaces:**
```go
package habit
type Deps struct{ Store *mongorepo.Store; PG *pgrepo.TrayaStore; Redis *redis.Client; Clock common.Clock; Cfg *setup.Config; HTTP *setup.HTTPClients; Shopflo *legacy.ShopfloClient; Log *slog.Logger }
type Service struct{ Deps; TenantID string }
func New(t *tenant.Tenant, d Deps) *Service
// constants.go: everything in inventory-app-backend §3.1 (tiers, ladder, assets map Assets with the 21 keys, header/cta/intro copy, badge images, reorder constants)
type Reward struct{ Type string `json:"type"`; Slug string `json:"slug"`; Coins int `json:"coins"` }
func DailyCoins(streakDay int) int
func WeekForDay(streakDay int) int
func ResolveReward(streakDay int) *Reward                 // nil when <1 or >35
func (s *Service) EnsureStreakMaster(ctx, r Reward) (*models.StreakMaster, error)
func (s *Service) SaveRewardTransaction90(ctx, userID string, master *models.StreakMaster, phone, reason string, coins int) (*models.RewardTransaction, error)  // expire_at = UTCMidnight(now+1d+90d); NO shopflo
func (s *Service) CreditHabitTrackerCoins(ctx, in legacy.HabitCreditInput) (legacy.HabitCreditResult, error)   // §3.3 daily path
func (s *Service) MintOrCredit(ctx, in legacy.HabitCreditInput) (legacy.HabitCreditResult, error)   // controller/mint semantics: reward nil → paused; ladder → Mint (Task 25) else Credit; also validates streakDay>=1
```

- [ ] **Step 1: Tests**: `ResolveReward` for days 1,7,8,14,35,36 (`36 → nil`); `DailyCoins` 1→30, 8→40, 35→70; `EnsureStreakMaster` display names `Habit Tracker Ladder - Day 7` / `Habit Tracker Daily - week 1`; credit idempotency by `habit-tracker-daily-<date>`.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): constants, reward tiers and habit credit`

---

## Task 23: State resolver and bottom sheets

**Files:**
- Create: `internal/habit/state_resolver.go`, `internal/habit/bottomsheets.go`, tests

**Interfaces:**
```go
type StateInput struct{ LoggedToday, HasLoggedEver, RewardToday, DayBeforeReward, StreakBroken, BrokeOnRewardDay, KitArrivingIntro, NewOrderPlacedNotDelivered, NewKitDeliveredNotLogged, EarningPaused bool; EffectiveDay, LifelinesUsed, LifelinesTotal, CoinBalance int }
type Resolved struct{ State, Heading, CtaLabel, CoinsSubtitle, StreakSubtitle, LifelinesSubtitle string }
func ResolveState(in StateInput) Resolved
func Rupees(coins int) string; func LifelineLabel(used, total int) string; func DaysToNextReward(d int) *int
func BuildCoinsBottomSheet(coinBalance int, expiry *time.Time, ords []orders.Order, now time.Time) map[string]any   // ordered structs preferred; keys header, expiry, weeklyCoins, bonus, note
func BuildStreakBottomSheet(currentStreak int) map[string]any
func BuildLifelineBottomSheet(used, total int) map[string]any
```

- [ ] **Step 1: Tests**: all 16 resolver rows (inventory §3.2) as a table with exact headings including `\n`; bottom sheets snapshot for `coinBalance 250, expiry 2026-10-01` (`"250 coins expire on 01 Oct 2026"`), streak 0 and 10, lifelines 0/1/3.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): 16-state resolver and bottom sheets`

---

## Task 24: `getHabitTrackerData`, calendar, badges

**Files:**
- Create: `internal/habit/data.go`, `internal/habit/calendar.go`, `internal/habit/badges.go`, tests

**Interfaces:**
```go
type DayCell struct{ Date string `json:"date"`; DayLabel string `json:"dayLabel"`; DayOfMonth int `json:"dayOfMonth"`; IsToday bool `json:"isToday"`; IsRewardDay bool `json:"isRewardDay"`; Coins int `json:"coins"`; Logged bool `json:"logged"`; LifelineUsed bool `json:"lifelineUsed"`; IsStreakBreak bool `json:"isStreakBreak"` }
type Stat struct{ Value int `json:"value"`; Label string `json:"label"`; Subtitle string `json:"subtitle"`; Action string `json:"action"`; Icon string `json:"icon"` }
type LifelineStat struct{ Total int `json:"total"`; Used int `json:"used"`; Label string `json:"label"`; Subtitle string `json:"subtitle"`; Action string `json:"action"`; Icon string `json:"icon"`; IconDisabled string `json:"iconDisabled"` }
type LogAndEarn struct { State string `json:"state,omitempty"`; HabitTrackerDisabled *bool `json:"habitTrackerDisabled,omitempty"`; LoggingEnabled bool `json:"loggingEnabled"`; Header struct{ Overline string `json:"overline"`; Heading string `json:"heading"` } `json:"header"`; Benefits any `json:"benefits,omitempty"`; Kit any `json:"kit,omitempty"`; Milestones any `json:"milestones,omitempty"`; Cta struct{ Label string `json:"label"`; Action string `json:"action"`; Param string `json:"param"` } `json:"cta"`; Assets map[string]string `json:"assets"`; Calendar *struct{ StartDate string `json:"startDate"`; TodayDate string `json:"todayDate"`; Days []DayCell `json:"days"`; EarningPaused bool `json:"earningPaused"` } `json:"calendar,omitempty"`; Stats *struct{ Coins Stat `json:"coins"`; Streak Stat `json:"streak"`; Lifelines LifelineStat `json:"lifelines"` } `json:"stats,omitempty"`; BottomSheets map[string]any `json:"bottomSheets,omitempty"`; KitLogCount int `json:"kitLogCount"`; DaysToPause *int `json:"daysToPause"`; CaseID string `json:"-"` }
func (s *Service) GetHabitTrackerData(ctx, userID string, version int, nonVoid []orders.Order) (*LogAndEarn, error)   // §3.6; nil,nil on internal error (component hides) — log it
func BuildLogAndEarn(in LogAndEarnInput) *LogAndEarn; func BuildIntro() *LogAndEarn
func LiveRun(valid map[string]bool, today time.Time) int; func LastRunBeforeGap(valid map[string]bool, today time.Time) int
func EarningPaused(uniqueLogs int, windowOpen *time.Time, today time.Time, kitCount int) bool
type OrderFlags struct{ NewOrderPlacedNotDelivered, NewKitDeliveredNotLogged bool }
func DeriveOrderFlags(ords []orders.Order, logged map[string]bool, loggedToday, hasLoggedEver bool, today time.Time) OrderFlags
// calendar.go
type MonthDay struct{ Date string `json:"date"`; DayOfMonth int `json:"dayOfMonth"`; Weekday int `json:"weekday"`; IsToday bool `json:"isToday"`; State string `json:"state"`; IsRewardDay bool `json:"isRewardDay"`; RewardEarned bool `json:"rewardEarned"`; Coins *int `json:"coins"`; InStreakRun bool `json:"inStreakRun"`; StreakBreak bool `json:"streakBreak"`; LifeUsed bool `json:"lifeUsed"` }
type MonthCalendar struct{ Month int `json:"month"`; Year int `json:"year"`; MonthLabel string `json:"monthLabel"`; FirstWeekday int `json:"firstWeekday"`; CanGoPrev bool `json:"canGoPrev"`; CanGoNext bool `json:"canGoNext"`; Days []MonthDay `json:"days"`; Legend []map[string]string `json:"legend"`; Summary struct{ DaysLogged, DaysMissed, CoinsMissed, RewardsMissed, LifelinesUsed int } `json:"summary"`; Assets map[string]string `json:"assets"` }
type CalendarResponse struct{ FirstLogDate *string `json:"firstLogDate"`; CurrentMonth struct{ Month, Year int } `json:"currentMonth"`; Legend []map[string]string `json:"legend"`; Assets map[string]string `json:"assets"`; Months []MonthCalendar `json:"months"` }
func (s *Service) GetHabitTrackerCalendar(ctx, userID string) (*CalendarResponse, error)   // §3.10, uses Redis cache key kit-tracker-calendar!<userId> TTL 300 when IsProduction
func (s *Service) InvalidateCalendarCache(ctx, userID string)                              // DEL key; errors logged
// badges.go
type BadgeView struct{ KitNumber int `json:"kitNumber"`; Name string `json:"name"`; Image *string `json:"image"`; Threshold int `json:"threshold"`; Earned bool `json:"earned"`; LogsInKit int `json:"logsInKit"`; EarnedAt string `json:"earnedAt"`; Upcoming *bool `json:"upcoming,omitempty"` }
type BadgesResponse struct{ EarnedCount int `json:"earnedCount"`; Badges []BadgeView `json:"badges"` }
func (s *Service) GetHabitTrackerBadges(ctx, userID, caseID string, nonVoid []orders.Order, gender string) (*BadgesResponse, error)
func DeriveBadges(windows []orders.KitWindow, valid map[string]bool, earned map[int]time.Time, gender string) (all []BadgeView, earnedCount int, newly []int)
func SelectBadgesForDisplay(all []BadgeView) []BadgeView
```
Read `services/bahService.js` lines for `buildHabitTrackerLogAndEarn`, `getHabitTrackerCalendar`, `computeHabitTrackerRunState`, `buildHabitTrackerMonthCalendar` to confirm exact key names and `legend` contents before implementing.

- [ ] **Step 1: Tests**: port cases from `/Users/mishika/Desktop/traya/traya-app-backend/test/bahService.getHabitTrackerData.test.js`, `bahService.monthCalendar.test.js`, `bahService.badges.test.js`, `bahService.earningWindow.test.js`, `habitTrackerStateResolver.test.js` (read them; translate assertions 1:1).
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): habit tracker data, month calendar with cache, badges`

---

## Task 25: Scratch cards and archived products

**Files:**
- Create: `internal/habit/scratchcard.go`, `internal/habit/archived.go`, tests

**Interfaces:**
```go
type ActiveCardView struct{ ID primitive.ObjectID `json:"id"`; Reward struct{ Type string `json:"type"`; Value int `json:"value"`; DisplayText string `json:"displayText"` } `json:"reward"`; CoverAsset string `json:"coverAsset"`; RevealAsset string `json:"revealAsset"`; ExpiresAt time.Time `json:"expires_at"`; Expired bool `json:"expired"`; Status string `json:"status"` }
func (s *Service) MintHabitTrackerScratchCard(ctx, in legacy.HabitCreditInput) (legacy.HabitCreditResult, error)   // §3.4 MINT
type ScratchCardsResponse struct{ Active []ActiveCardView `json:"active"`; Expired []map[string]any `json:"expired"`; History []map[string]any `json:"history"`; EmptyTitle string `json:"emptyTitle"`; EmptySubtitle string `json:"emptySubtitle"`; Title string `json:"title"`; NoRewardScreen string `json:"noRewardScreen"` }
func (s *Service) GetScratchCards(ctx, userID string) (*ScratchCardsResponse, error)
func (s *Service) RevealScratchCard(ctx, userID string, cardID primitive.ObjectID) (map[string]any, error)   // §3.4 REVEAL; errors NotFound(404), Gone(410), HTTPError{501}
func (s *Service) GetActiveCardForReveal(ctx, userID, rewardDayDate string) *struct{ ID primitive.ObjectID `json:"id"`; Status string `json:"status"` }
func CoinExpiryFields(expiresAt *time.Time, now time.Time) map[string]any
type ArchiveResult struct{ ArchivedProductIDs []string `json:"archivedProductIds"`; ActiveProductIDs []string `json:"activeProductIds"` }
func (s *Service) ArchiveProduct(ctx, userID, productID string) (*ArchiveResult, error)
func (s *Service) UnarchiveProduct(ctx, userID, productID string) (*ArchiveResult, error)
func (s *Service) GetArchivedProductIDs(ctx, userID string) (map[string]bool, error)
```

- [ ] **Step 1: Tests**: port `test/scratchCardService.{mint,list,reveal}.test.js`, `bahArchivedProductsService.test.js`, `bahController.archiveProduct.test.js` assertions; add reveal race integration (two goroutines → exactly one `reward_transactions` doc; other gets `alreadyClaimed`).
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): scratch cards and archived products`

---

## Task 26: Kit tracker page

**Files:**
- Create: `internal/habit/page.go`, `internal/habit/page_test.go`, `internal/habit/cms.go`

**Interfaces:**
```go
type CMSClient struct{ HTTP *http.Client; BaseURL string }
func (c *CMSClient) FeedbackCard(ctx, userID, caseID string, version int) (map[string]any, error)   // port getComponentDataFromNameArray(['feedback_v3'], …, {mintFormSessions:true}) — read app-backend services for the exact CMS endpoint + payload; return nil when no contents
func BuildReorderBanner(daysSinceDelivery *int, kitCount int) map[string]any
func DailyStripDays(days []DayCell, today string) []DayCell
func DailyLogDateLabel(cell DayCell, today string) *string
func (s *Service) GetRewardsCount(ctx, userID string) int
func BuildRewardScreen(core *LogAndEarn, todayCell *DayCell, daysToReward *int, weekDays []map[string]any, activeCard any, enabled bool, feedbackCard any) map[string]any
func (s *Service) GetKitTrackerPage(ctx, userID, caseID string, version int) (map[string]any, error)   // §3.5 assemble; uses ordered struct for top-level keys (state, habitTrackerDisabled, header, kitGoal, stats, assets, banner, dailyLog, rewardScreen, bottomSheets, kitLogCount, loggingEnabled, earningPaused)
```

- [ ] **Step 1: Tests**: port `kitTrackerPageService*.test.js` (reward screen, feedback card O1 rule, reorder banner from day 21/26, footer strings); `kitGoal.navigation.url == FORM_BASE_URL + "/pages/care-plan/<caseId>?source=app"` with exactly one slash.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): kit tracker page assembly`

---

## Task 27: Habit coin redeem

**Files:**
- Create: `internal/habit/redeem.go`, `internal/habit/redeem_test.go`

**Interfaces:**
```go
type RedeemInput struct{ RedeemedAmount float64 `json:"redeemedAmount"`; UserID string `json:"userId"`; Remarks string `json:"remarks"`; ShopFloTxnID string `json:"shopFloTxnId"`; CaseID string `json:"caseId"`; OrderID string `json:"orderId"`; OrderDisplayID string `json:"orderDisplayId"`; Currency string `json:"currency"`; IsJusPay *bool `json:"isJusPay"` }
func (s *Service) RedeemCoinsNonOrder(ctx, in RedeemInput) (map[string]string, error)   // §3.12; returns {"message":"Redeem coin transaction executed successfully"}; errors are real HTTPErrors (spec fix 9); Mongo transaction when IsProduction; unique index guards
```

- [ ] **Step 1: Tests**: insufficient → 400 `User have not enough coins`; duplicate shopflo txn → 400 `This reward is already redeemed`; FIFO drain across two credits (partial second); Shopflo debit called with `coins/10` via httptest.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): coin redeem with FIFO drain and idempotency`

---

## Task 28: Lifeline scheduler, worker, reconcile

**Files:**
- Create: `internal/habit/lifeline/schedule.go`, `decision.go`, `state.go`, `apply.go`, `queue.go`, `worker.go`, `reconcile.go`, tests

**Interfaces:**
```go
package lifeline
func CheckDateFor(lastLog time.Time) string                     // ISTDateString(ISTStartOfDay(lastLog)+24h)
func DueAtFor(checkDate string, graceHours int) time.Time       // ISTDayAnchor(checkDate)+24h+grace
type Decision struct{ Action string; ScheduleNext bool }        // "intact" | "apply" | "break"
func Decide(checkDateLogged, streakAlive, checkDateInKit, kitPaused bool, budgetRemaining int) Decision
type StreakState struct{ StreakAlive, CheckDateLogged bool; StreakDay int }
type KitContext struct{ KitStart *time.Time; BudgetRemaining int; KitPaused bool }
type Deps struct{ Store *mongorepo.Store; Orders func(ctx, userID string) ([]orders.Order, error); Redis *redis.Client; Clock common.Clock; Credit func(ctx, userID string, streakDay int, checkDate string) error; TenantID string; Grace int; TestDelayMS *int; Log *slog.Logger }
func (d *Deps) GetStreakState(ctx, userID, checkDate string) (StreakState, error)
func (d *Deps) GetKitContext(ctx, userID string) (KitContext, error)
func (d *Deps) ApplyLifeline(ctx, userID, checkDate string, kitStart time.Time, streakDay int) (created bool, err error)
type Queue struct{ R *redis.Client; TenantID string }
func (q *Queue) Key() string                                    // "bah:<tenant>:lifeline:due"
func (q *Queue) Enqueue(ctx, userID, checkDate string, dueAt time.Time) error   // ZREM existing members for user (scan by prefix "<userId>|") then ZADD
func (q *Queue) PendingUserIDs(ctx) ([]string, error)
func (q *Queue) PopDue(ctx, now time.Time, limit int) ([]Job, error)           // Lua: ZRANGEBYSCORE due -inf now LIMIT 0 limit; move each to processing with score now+60s
func (q *Queue) Ack(ctx, job Job) error; func (q *Queue) RequeueStale(ctx, now time.Time) error
type Job struct{ UserID, CheckDate string }
func EnqueueForLog(ctx, q *Queue, userID string, logDate time.Time, grace int, testDelayMS *int, now time.Time) error
type Worker struct{ Q *Queue; D *Deps; Concurrency int; PollEvery time.Duration }
func (w *Worker) Run(ctx context.Context)                       // loop until ctx done
func (d *Deps) ProcessJob(ctx, j Job) (Decision, *Job, error)  // returns next job to schedule
func ReconcileUserLifelines(lifelineDates []string, realValid map[string]bool, budget int) (keep, remove []string)
func (w *Worker) RunReconcileDaily(ctx)                          // 05:00 IST ticker; Redis lock "bah:<tenant>:lifeline:reconcile-lock" TTL 10m; seeds checks for active streaks without pending
```

- [ ] **Step 1: Tests**: port all 10 `lifeline*.test.js` + `applyLifeline.test.js` + `habitKitWindow.test.js` from api-server BAH tests (read them; translate 1:1); queue integration with `TEST_REDIS_ADDR`: enqueue twice same user replaces; `PopDue` respects score; stale requeue.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(habit): redis-backed lifeline scheduler, worker and daily reconcile`

---

## Task 29: Log & Earn core

**Files:**
- Create: `internal/logearn/constants.go`, `internal/logearn/config.go`, `internal/logearn/cap.go`, `internal/logearn/service.go`, tests

**Interfaces:**
```go
package logearn
type ZTier struct{ UpTo *int `json:"upTo"`; Amount int `json:"amount"` }
type Bonuses struct{ FirstEver, DayThree, ReorderWelcome, MilestoneEvery, MilestoneAmount int }
var DefaultZTiers = []ZTier{{ptr(7),2},{ptr(14),3},{nil,4}}; var DefaultBonuses = Bonuses{5,5,5,7,10}
var DeadOrderStatuses = []string{"void","cancelled","returned","rto","lost","damaged","ghost"}
type RewardsConfig struct{ ZTiers []ZTier; Bonuses Bonuses }
type ConfigClient struct{ HTTP *http.Client; BaseURL string; cache sync.Map } // per tenant, 10 min
func (c *ConfigClient) Get(ctx, tenantID string) RewardsConfig   // GET /api/carestack/config/{tenantId}; defaults on any error
func ZAmountForDay(day int, tiers []ZTier) (tier, amount int)
func BonusForDay(day int, firstEver bool, b Bonuses) int
func ComputeStreakDay(prev []pgrepo.DoseLog, target time.Time, lifelineBridged bool) (streakDay int, isFirstEver bool)
type CapInfo struct{ BulkX int `json:"bulkX"`; TreatmentDay int `json:"treatmentDay"`; EarningCapDays int `json:"earningCapDays"`; TotalWindowDays int `json:"totalWindowDays"`; EarningCapHit bool `json:"earningCapHit"`; WindowExpired bool `json:"windowExpired"`; EarningDaysConsumed int `json:"earningDaysConsumed"`; EarningDaysRemaining int `json:"earningDaysRemaining"`; InGracePeriod bool `json:"inGracePeriod"`; DeliveryDate *time.Time `json:"deliveryDate"`; OrderID *string `json:"orderId"`; HasInFlightOrder bool `json:"-"`; OrderServiceFailed bool `json:"-"` }
type Deps struct{ Store *pgrepo.LogEarnStore; Customers *pgrepo.CustomerStore; Orders *orders.TROrderClient; Config *ConfigClient; Clock common.Clock; S3Base string; Log *slog.Logger }
type Service struct{ Deps; TenantID string }
func New(t *tenant.Tenant, d Deps) *Service
func (s *Service) GetEarningCapInfo(ctx, customerID string) (CapInfo, error)
```

- [ ] **Step 1: Tests**: `ComputeStreakDay` (empty → 1/first; gap 2 → +1; gap 3 → 1; bridged gap 5 → +1); `BonusForDay` (day 1 first-ever 5; day 3 5; day 7 10; day 21 10; day 3 first-ever 10); cap math (bulkX 2 → 60/75; consumed 60 → capHit; treatmentDay 80 → expired); config client httptest with malformed body → defaults.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(logearn): constants, config client, cap info, streak math`

---

## Task 30: Log & Earn endpoints logic

**Files:**
- Create: `internal/logearn/state.go`, `internal/logearn/actions.go`, tests

**Interfaces:**
```go
type Last7 struct{ Date string `json:"date"`; StreakDay int `json:"streakDay"`; InrCredited int `json:"inrCredited"`; IsBackfill bool `json:"isBackfill"` }
type Product struct{ Name string `json:"name"`; CartDisplayName string `json:"cartDisplayName"`; Dosage string `json:"dosage"`; ImageURL struct{ CartImgURL string `json:"cartImgUrl"`; ImageCdnURL string `json:"imageCdnUrl"`; SingleImages string `json:"singleImages"` } `json:"image_url"` }
type State struct{ CustomerID string `json:"customerId"`; Balance int `json:"balance"`; StreakDay int `json:"streakDay"`; LogDoneToday bool `json:"logDoneToday"`; ZForNext int `json:"zForNext"`; ZTier int `json:"zTier"`; Last7 []Last7 `json:"last7"`; BackfillAvailable bool `json:"backfillAvailable"`; LifelineAvailable bool `json:"lifelineAvailable"`; CapInfo /* embedded, flattened via custom MarshalJSON or repeat fields */; Products []Product `json:"products"` }
func (s *Service) State(ctx, customerID string) (*State, error)
type LogResult struct{ Success bool `json:"success"`; StreakDay int `json:"streakDay"`; InrCredited int `json:"inrCredited"`; EarningCapHit bool `json:"earningCapHit"`; Balance int `json:"balance"` }
func (s *Service) LogToday(ctx, customerID string, orderID *string) (*LogResult, error)
func (s *Service) BackfillYesterday(ctx, customerID string, orderID *string) (*LogResult, error)
type LifelineResult struct{ Success bool `json:"success"`; Message string `json:"message"`; Balance int `json:"balance"`; PriorStreakDay int `json:"priorStreakDay"`; DaysBridged int `json:"daysBridged"` }
func (s *Service) UseLifeline(ctx, customerID string) (*LifelineResult, error)
type RedeemResult struct{ Success bool `json:"success"`; Redeemed int `json:"redeemed"`; Balance int `json:"balance"` }
func (s *Service) Redeem(ctx, customerID string, subtotal int, amount *int, orderID *string) (*RedeemResult, error)
```
All write flows run inside `Store.WithTx`; dose log insert + ledger appends are one transaction (spec fix 11). Messages verbatim from inventory logearn §3.

- [ ] **Step 1: Tests** (integration with PG + httptest order-service): first log → streakDay 1, credited 2+5=7, ledger 2 rows; second same day → 400 `Already logged today`; day-3 bonus; cap hit → 0 credited but dose row inserted; backfill rules; lifeline once per order (`Lifeline already used for this order`); redeem cap 50% and `Nothing to redeem`; redeem same orderId twice → 400 `This order is already redeemed`.
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(logearn): state, log, backfill, lifeline, redeem`

---

## Task 31: Controllers, routes and wiring

**Files:**
- Create: `controllers/legacy_controller.go`, `controllers/habit_controller.go`, `controllers/crm_controller.go`, `controllers/logearn_controller.go`, `controllers/deps.go`, `routes/routes.go`, `routes/routes_test.go`
- Modify: `main.go`

**Interfaces:**
```go
package controllers
type Deps struct{ Cfg *setup.Config; Registry *tenant.Registry; Verifier *auth.Verifier; Master *mongo.Client; Redis *redis.Client; HTTP *setup.HTTPClients; TrayaPG *pgxpool.Pool; Clock common.Clock; Log *slog.Logger; Dispatcher *legacy.Dispatcher; CCD legacy.CCDPublisher }
func (d *Deps) legacy(c *fiber.Ctx) *legacy.Service; func (d *Deps) habit(c *fiber.Ctx) *habit.Service; func (d *Deps) logearn(c *fiber.Ctx) *logearn.Service   // build per request from tenant.From(c)
package routes
func Setup(app *fiber.App, d *controllers.Deps)
```
Route table (exact; `T` = tenant middleware applied globally after `/health`):
```
// legacy (RequireEconomy(Legacy))
GET  /streakAndRewardBalance            RequireJWT                  → GetStreakAndRewardBalance(userID, id.AppVersion or query version, id.RawToken)   FamilyLegacyErr
GET  /bahLogForGivenDate                RequireJWT                  → date=query date (parse RFC3339/any JS-parsable; invalid → treat as now like JS Invalid Date? JS `new Date('bad')` → Invalid Date → moment invalid; keep simple: 400 {err}); showBahV3 = appVersion>=71  FamilyLegacyErr
POST /activityLogForBAH                 RequireJWT                  → body map; ValidateLogActivity; FamilyLegacyMessage (400 validation; 500 else)
PUT  /multipleActivityLogForBAH         RequireJWT                  → FamilyLegacyMessage
GET  /coinTransaction                   RequireJWT                  → page/limit ints default 1/6; FamilyCoinTxn
GET  /bah/:userId/calendar              RequireJWT                  → identity userID; query date required (400 {error:"date is required"} — read index.js route 33 for exact text), mode default calendar; FamilyCalendar
GET  /rewardBalance/:caseId             RequireV2Token              → uuid validate (400 {message:"Invalid caseId"} — read uuidValidator for text); {rewardBalance}; FamilyV85
GET  /medicinesForHowToUse              RequireJWT                  → FamilyLegacyErr
GET  /latestOrderHowtoUseV2/:caseId     RequireV2Token              → uuid validate; FamilyProxy
GET  /latestRoutineV2/:caseId           RequireV2Token              → FamilyProxy
// CRM
GET  /bahHistory/:caseId                RequireJWT                  → FamilyLegacyErr
GET  /rewardBalance/:customerId         (same handler as /rewardBalance/:caseId — one route serves both)
GET  /streakMaster                      RequireJWT                  → FamilyLegacyErr
POST /extraRewardsToUser/:caseId        RequireJWT, RequireAdmin    → FamilyLegacyErr
PUT  /syncRewardBalanceWithShopFlo/:caseId RequireJWT, RequireAdmin → FamilyLegacyErr
// habit public (RequireEconomy(Habit))
POST /archiveProductForBAH, /unarchiveProductForBAH   RequireJWT   → body productId; FamilyV85
GET  /bah/scratch-card                  RequireJWT                  → FamilyV85 (pass 404/410 through)
POST /bah/scratch-card/:id/reveal       RequireJWT                  → FamilyV85
GET  /kit-tracker-page|-calendar|-badges RequireJWT                 → version from query appVersion||version||header; FamilyV85 ({message} with status; 500 message texts per inventory)
// habit internal (RequireInternal)
POST /bah/archive-product, /bah/unarchive-product     body userId (uuid) productId
GET  /bah/scratch-card?userId=          POST /bah/scratch-card/:id/reveal body userId
GET  /config/cms/kit-tracker-page|calendar|badges     query userId caseId, header x-app-version
POST /bah/scratch-card/habit-tracker-mint, POST /bah/coin/habit-tracker-credit   body userId streakDay phoneNumber checkInDate → MintOrCredit → 200 result
POST /bah/coin/redeem                   body RedeemInput → 200 {message}
// logearn (RequireEconomy(LogEarn), RequireGateway)
GET  /consumers/logearn/state           200
POST /consumers/logearn/log|backfill|lifeline|redeem   201 ; FamilyLogEarn
```
`main.go`: `setup.MustLoad`, logger, `ConnectMongo`, traya PG pool, redis, `NewHTTPClients`, registry, verifier, dispatcher, CCD publisher, `routes.Setup`, lifeline worker for each tenant with Habit when `LifelineWorkerEnabled`, graceful shutdown drains dispatcher.

- [ ] **Step 1: HTTP tests** (`app.Test` with fake registry/tenant and stubbed services via interfaces on Deps): each route family's error envelope; auth applied (401 without token); `/bah/:userId/calendar` uses token user not path; logearn POST returns 201; internal routes reject without `x-internal-token`.
- [ ] **Step 2–4:** implement; PASS. Boot the service against `.env` and curl `/health`.
- [ ] **Step 5: Commit** `feat(http): controllers, routes, auth wiring and main`

---

## Task 32: Migrations and tools

**Files:**
- Create: `migrations/mongo/README.md` (indexes are code — `mongorepo.EnsureIndexes`), `migrations/pg/logearn/001_dose_logs_cash_ledger.sql` (from Task 8), `cmd/migrate/main.go`, `cmd/parity/main.go`, `cmd/parity/testdata/sample.jsonl`

**Interfaces:**
```
go run ./cmd/migrate --tenant mool            # applies pg/logearn/*.sql in order using tenant registry pools; records in schema_migrations(name, applied_at)
go run ./cmd/parity --a http://api-server --b http://localhost:3000 --cases cases.jsonl --ignore txnDate,createdAt,updatedAt,timestamp
   # each line {"name","tenant","method","path","headers":{},"body":{}}; prints per-case PASS/DIFF with JSON pointer paths; exit 1 on any DIFF
```
- [ ] **Step 1: Test** parity differ on two JSON docs (null vs missing key is a DIFF; array order matters; ignored paths skipped).
- [ ] **Step 2–4:** implement; PASS. **Step 5: Commit** `feat(tools): logearn migrations runner and parity harness`

---

## Task 33: Docs, swagger, final verification

**Files:**
- Modify: `README.md`, `CLAUDE.md`, controllers (swag annotations on every route), `docs/swagger.*` via `swag init`
- Modify: `docs/superpowers/specs/2026-09-18-tr-bah-service-design.md` §8 (testcontainers → env-driven compose)

- [ ] **Step 1:** Add swag annotations (`@Summary`, `@Param x-tenant-id header string true`, `@Router`) to every controller; run `swag init`; mount `/api/docs/*`.
- [ ] **Step 2:** `gofmt -l . ; go vet ./... ; go test ./... ; docker compose -f docker-compose.test.yml up -d && TEST_* go test ./... -tags integration` all green.
- [ ] **Step 3:** README sections: overview, tenants, auth per tenant, env vars, run/test, rollout pointer (`docs/superpowers/specs/2026-09-18-rollout-plan.md`), parity tool usage.
- [ ] **Step 4: Commit** `docs: swagger, README, spec test note`

---

## Self-review

**Spec coverage:** §2.1 endpoints → Tasks 17–21 (legacy/CRM), 24–27 (habit), 30 (logearn), 31 (routes). §3 layout → file structure above. §3.3 tenancy → Task 9. §4 indexes/idempotency → Tasks 6, 8, 15, 27, 30. §5 auth → Task 10, 31. §6.1 fixes: 1 (Task 31 route), 2 (Task 15), 3 (Task 14), 4–5 (Task 19), 6 (Task 7/20), 7 (Task 21), 8 (Task 26), 9 (Task 27), 10 (Task 27), 11 (Tasks 8, 30), 12 (Tasks 18, 31). §6.3 time → Task 4. §6.4 seam + CCD → Tasks 15, 19. §6.5 lifelines → Task 28. §6.6 clients → Tasks 3, 12, 15, 18, 26, 29. §6.7 envelopes → Task 5, 31. §7 config → Task 2. §8 testing → each task + Task 32 parity. §9 cutover → README (Task 33).

**Gaps found and fixed inline:** `/rewardBalance/:customerId` handled by the shared handler (Task 17 `GetUserRewardBalance` accepts either id). `getActivityLogs` needed by `bahHistory` → Task 21. Streak-restart bonus idempotency: decided in Task 15 (same-IST-day guard, remarks unchanged for display parity).

**Type consistency:** `legacy.HabitCreditInput/Result` and `legacy.HabitCreditor` are defined in Task 14 and implemented by `habit.Service.MintOrCredit` (Task 22/25); `mongorepo.CoinTxnRow` (Task 7) is reused by Task 20; `orders.Order` aliases `pgrepo.Order` (Task 11) and `TRAppOrder` is mapped into it in Task 29's cap logic; `common.Family` constants (Task 5) are used by Task 31.
