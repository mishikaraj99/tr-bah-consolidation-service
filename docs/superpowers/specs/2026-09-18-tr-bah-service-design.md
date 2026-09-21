# tr-bah-service — Design

Date: 2026-09-18. Status: approved for implementation.
Author: Claude with Nikhil Nayak (nikhilnayak@traya.health).

## 1. Goal

Consolidate every Build-A-Habit (BAH) surface that today lives in three Node services — the legacy 3/7/21
economy and CRM routes in `traya-api-server`, the v85 Habit Tracker in `traya-app-backend`, and the
Log & Earn ledger in `tr-consumer-backend` — into one Go service that is multi-tenant from day one.

Tenants: `traya`, `mool`, `acne`. Each tenant has its own Mongo database and its own Postgres database.
Traya runs two economies (legacy + v85); mool and acne run Log & Earn.

The service is the migration target described in `docs/reference/bah-migration-runbook.pdf`. Public URLs
do not change: api-server flips each route to proxy here, path for path, one ticket per deploy.

## 2. Scope

### 2.1 Endpoints implemented

**Traya — legacy economy** (public paths, mounted at root, no prefix)

| Method | Path | Source | Notes |
|---|---|---|---|
| GET | `/streakAndRewardBalance` | api-server | heaviest read; version gates; cohorts; modals |
| GET | `/bahLogForGivenDate` | api-server | dose list for a date; archived filter at appVersion ≥ 85 |
| POST | `/activityLogForBAH` | api-server | the shared write; `isHabitTracker` seam |
| PUT | `/multipleActivityLogForBAH` | api-server | forgiving bulk check-in flip |
| GET | `/coinTransaction` | api-server | paginated ledger with synthesised Expired rows |
| GET | `/bah/:userId/calendar` | api-server | **identity bound to token; path param ignored** |
| GET | `/rewardBalance/:caseId` | api-server | V2 token auth |
| GET | `/medicinesForHowToUse` | api-server | |
| GET | `/latestOrderHowtoUseV2/:caseId` | api-server | recommendation-service proxy + reminder merge |
| GET | `/latestRoutineV2/:caseId` | api-server | recommendation-service proxy |

**Traya — CRM keepers** (served to `traya-internal-api-service` as well)

| Method | Path | Notes |
|---|---|---|
| GET | `/bahHistory/:caseId` | fans out to four reads + Shopflo balance |
| GET | `/rewardBalance/:customerId` | same handler as `/rewardBalance/:caseId`; value may be a caseId or a userId |
| GET | `/streakMaster` | |
| POST | `/extraRewardsToUser/:caseId` | manual grant |
| PUT | `/syncRewardBalanceWithShopFlo/:caseId` | |

**Traya — v85 economy** (each served at BOTH its api-server public path and its app-backend path)

| Public (api-server) | Internal (app-backend) |
|---|---|
| POST `/archiveProductForBAH` | POST `/bah/archive-product` |
| POST `/unarchiveProductForBAH` | POST `/bah/unarchive-product` |
| GET `/bah/scratch-card` | GET `/bah/scratch-card?userId=` |
| POST `/bah/scratch-card/:id/reveal` | same |
| GET `/kit-tracker-page` | GET `/config/cms/kit-tracker-page` |
| GET `/kit-tracker-calendar` | GET `/config/cms/kit-tracker-calendar` |
| GET `/kit-tracker-badges` | GET `/config/cms/kit-tracker-badges` |
| — | POST `/bah/scratch-card/habit-tracker-mint` and POST `/bah/coin/habit-tracker-credit` (server-to-server) |
| — | POST `/bah/coin/redeem` (server-to-server) |

Public paths take identity from the Traya JWT. Internal paths take `userId`/`caseId` from query/body and
are reachable only with the internal service token (`INTERNAL_SERVICE_TOKEN` header `x-internal-token`),
because app-backend has no auth today and these routes will be called by api-server during migration.

**mool, acne — Log & Earn** (paths as today, POST returns 201)

`GET /consumers/logearn/state`, `POST /consumers/logearn/log`, `POST /consumers/logearn/backfill`,
`POST /consumers/logearn/lifeline`, `POST /consumers/logearn/redeem`.

Also kept as an internal function, not a route: `getLatestMedicines` (used by `bahLogForGivenDate` and
`bahHistory`).

### 2.2 Not implemented

The 17 routes the runbook deletes in Wave 0: `/faqs`, `/termsAndConditions`, `/coinRewardHistory`,
`/newBahFlowEligibility`, `/bahHistoryWeb/:caseId`, `/bah-challenges-cx-data`, `/latestOrderHowtoUse/:caseId`
(v1), `/addCoinBalance`, `/shopflo/txn/:caseId`, `/shopflo/debitReward`, `/shopflo/creditReward`,
`/coinScript/:streakMasterId`, `/uploadUsageDetails`, PUT `/activityLogForBAH`, `/activityLogsBAH`,
`/bahDetailDateWiseLikePagination`, GET `/latestMedicines`. The legacy MOOL Mongo BAH module. Home-page
widget handlers (they stay in app-backend and call this service). Deployment manifests beyond the Dockerfile.

## 3. Architecture

### 3.1 Repository layout

```
tr-bah-service/
  main.go                         Fiber app; middlewares; /health; /api/docs; graceful shutdown
  go.mod                          module traya-bah-service, go 1.25
  Dockerfile, .air.toml, .gitignore, .env.example, README.md, CLAUDE.md
  setup/
    env.go                        typed config loaded once; fails fast on missing required vars
    mongo.go                      master client; per-tenant DB resolver; index bootstrap (sync.Once per DB)
    postgres.go                   per-tenant pgx pool resolver (traya: DATABASE_*; mool/acne: POSTGRES_*)
    redis.go                      primary client (SERVICES_CACHE_*), pub/sub publisher
    httpclient.go                 pooled http.Client factory with timeouts and keep-alive
    logger.go                     slog JSON logger; request id; PII masking helpers
  tenant/
    registry.go                   master.tenant lookup, cached; tenant → economy + auth strategy map
    context.go                    TenantCtx {ID, Mongo *mongo.Database, PG *pgxpool.Pool, Economies, Auth}
    middleware.go                 x-tenant-id → TenantCtx in c.Locals("tenant"); 400 when missing/unknown
  auth/
    identity.go                   Identity {UserID, CaseID, Email, Roles, Phone, FirstName, Gender}
    jwt_traya.go                  Bearer HS256 (JWT_SECRET) + Redis <tenant>:user!<id>login!status gate
    v2token.go                    x-access-token / authorization == "Bearer " + V2_FORM_DATA_TOKEN
    internal_token.go             x-internal-token == INTERNAL_SERVICE_TOKEN
    gateway.go                    x-user-info JSON → Identity; customerId query fallback
    middleware.go                 RequireJWT, RequireV2Token, RequireInternal, RequireGateway, RequireAdmin
  internal/
    common/
      clock.go                    Clock interface (Now) for tests
      istdate.go                  ISTShift(t), UTCMidnight(t), ISTDay(t), ISTDateString(t), ISTDaysBetween, ISTDayAnchor
      response.go                 envelope helpers per family (legacy {err}, v85 {message}, logearn {message,statusCode,timestamp,path})
      errors.go                   HTTPError{Status, Message}; factories BadRequest, NotFound(400 for v85!), Gone, Forbidden
      validate.go                 Joi-equivalent validators producing the same message text
      mongoutil.go                ObjectID parsing, day-range builders ($gt/$gte variants named explicitly)
    orders/
      kit_calculator.go           getKitDetail, getAllDetailsRelatedToNonVoidOrders, currentKitStart, getHabitTrackerKitWindows, getCurrentRunningKitStartDate
      traya_orders.go             Traya Postgres reads: non-void orders, first delivered, how-to-use orders, latest order + count
      tr_orderservice.go          mool/acne order-service client: non-void orders by customer, order details by id
      variants.go                 VARIANT_IDS_ARRAY, VARIANT_ID_MAPPING, vitamin dedupe, OLD_TO_ACTIVE map fetch (config-service)
    legacy/
      constants.go                every constant and copy string from inventory §6
      streak.go                   createStreakLogForUser, getBahRunningLogDay, streak-broken recompute
      rewards.go                  saveRewardTransaction (with idempotency key), creditRewardCoinsToUser, streak-restart bonus, first-log bonus
      cohorts.go                  O8+, male/female restart cohorts, 15-day check-in eligibility + feedback URL
      banner.go                   getBannerWidgetData, getBahChallengeEntryPointBanner, getPostLoggingModalContent, modals builder
      balance.go                  getStreakAndRewardBalance, getOnlyRewardBalance, getEarliestExpiringUnusedCoins
      logs.go                     getBahLogForGivenDate (dosage split), saveActivityLogs, POST handler flow, multiple-log PUT
      cointxn.go                  getRewardCoinHistoryPaginated (aggregation), getRewardCoinHistory
      calendar.go                 getBahCalendarLogData
      medicines.go                getLatestMedicines, getPrescriptionForMedicinesByOrders, getMedicinesForHowToUsePurpose
      howtouse.go                 recommendation-service proxies + reminderInfo merge
      crm.go                      bahHistory, extraRewardsToUser, syncRewardBalanceWithShopFlo, streakMaster
      tasks.go                    updateTaskForUserToDisplay (task_master + user_task_details)
      ccd.go                      CCD_UPDATE publisher
      shopflo.go                  wallet client: ensure, credit, debit, read
    habit/
      constants.go                tiers, ladder, assets, copy
      reward.go                   resolveHabitTrackerReward, ensureHabitTrackerStreakMaster, creditHabitTrackerCoins, saveRewardTransaction(90d)
      state_resolver.go           16-state resolver
      data.go                     getHabitTrackerData, buildHabitTrackerLogAndEarn, intro, order flags, runs, earning window
      bottomsheets.go             coin / streak / lifeline sheets
      page.go                     kitTrackerPageService: reorder banner, strip, reward screen, assemble, feedback card
      calendar.go                 month calendar + run state; Redis cache
      badges.go                   derive, select, upsert-on-read
      scratchcard.go              mint, list, reveal, active-for-reveal
      archived.go                 archive / unarchive / filter
      redeem.go                   /bah/coin/redeem (FIFO drain, Shopflo debit)
      lifeline/
        schedule.go               checkDateFor, dueAtFor
        decision.go               decideLifeline
        state.go                  getStreakState, getKitContext, breakStreak
        apply.go                  applyLifeline (idempotent)
        queue.go                  Redis ZSET scheduler: Enqueue(userID, checkDate, dueAt), Pending(), Pop due
        worker.go                 poll loop, processLifelineJob, credit on apply
        reconcile.go              reconcileUserLifelines, daily seeding, Redis lock
    logearn/
      constants.go                z-tiers, bonuses, cap defaults, dead statuses, messages
      ist.go                      wraps common ISTDay helpers
      ledger.go                   currentBalance, creditCash, debitCash (pg tx + FOR UPDATE)
      doselog.go                  dose_logs reads/writes
      cap.go                      getEarningCapInfo
      config.go                   CMS rewards config with 10-min per-tenant cache
      service.go                  state, logToday, backfillYesterday, useLifeline, redeem
  repositories/
    mongo/                        activity_logs.go, streak_logs.go, streak_masters.go, reward_transactions.go,
                                  redeem_transactions.go, archived_products.go, lifeline_logs.go, scratch_cards.go,
                                  badges.go, customer_activity_log.go, tasks.go
    pg/                           traya_orders.go, traya_users_cases.go, traya_products.go, traya_reminders.go,
                                  traya_forms.go, logearn_doselogs.go, logearn_ledger.go, tenant_customers.go
  controllers/
    legacy_controller.go, habit_controller.go, crm_controller.go, logearn_controller.go
  routes/routes.go
  migrations/
    mongo/001_indexes.go          idempotent CreateMany per tenant DB on first resolve
    pg/logearn/001_dose_logs_cash_ledger.sql
  cmd/parity/main.go              replay + diff tool (runbook ticket 9)
  docs/reference/*                runbook + three inventories (ground truth for copy/constants)
  docs/superpowers/specs, plans
```

Dependency rule: `controllers → internal/* → repositories → setup`. `internal/*` never imports Fiber.
`tenant` and `auth` are imported only by `routes`, `controllers` and `main`.

### 3.2 Request lifecycle

1. `recover`, request logger (same format as config-service: time | ip | x-trace-id | instance | status | method | path | latency | error).
2. `tenant.Middleware`: reads `x-tenant-id`, resolves `TenantCtx`, stores in `c.Locals`. Unknown tenant → 400 `{message:"Tenant not found: <id>"}`.
3. Route-level auth middleware per route family (see §5).
4. Controller parses params, calls the economy service with `(ctx, TenantCtx, Identity, input)`, writes the family envelope.
5. Economy services obtain collections/pools from `TenantCtx`; never from globals.

### 3.3 Tenancy

- Registry: Mongo `master` DB, collection `tenant` `{tenant_id, tenant_name}`. Cached in-process after first read; unknown ids are re-checked at most once a minute.
- `DEFAULT_TENANT` (optional): when set, a request without `x-tenant-id` is treated as that tenant. Used only in the ALB-direct rollout phase for Traya (rollout plan §5); unset elsewhere so a missing header is a 400.
- Tenant Mongo DB name: `${tenant_id}_${MONGO_DATABASE_SUFFIX}`; when `ENVIRONMENT=production` and tenant is `traya` → `TrayaProd`. Overridable per tenant with `MONGO_DB_NAME_<TENANT>`.
- Tenant Postgres: `traya` → single pool from `DATABASE_*` (the api-server database, where `orders`, `users`, `cases`, `product_sku_mapping`, `medicine_master`, `user_order_reminders`, `form_session` live). `mool`/`acne` → pool per tenant on `${tenant_id}_${POSTGRES_DATABASE_SUFFIX}` using `POSTGRES_WRITE_*` (reads also go to the writer; a `POSTGRES_READ_*` replica pool is used for `SELECT`s in `state` when configured).
- Economy map (code constant, overridable by `TENANT_ECONOMIES` JSON env): `traya: [legacy, habit]`, `mool: [logearn]`, `acne: [logearn]`. A route whose economy is not enabled for the tenant returns 404 `{message:"Not available for tenant <id>"}`.
- Redis is shared (one instance, `SERVICES_CACHE_*`), so every key this service owns is tenant-scoped. New keys are prefixed `bah:<tenant>:`. The three keys with an existing Node owner are prefixed `<tenant>:` in front of their original name, because one Redis serving three tenants would otherwise collide on a bare user id:

| key | canonical name here | still owned in Node by |
| --- | --- | --- |
| kit-tracker calendar cache | `<tenant>:kit-tracker-calendar!<userId>` | traya-app-backend |
| login gate | `<tenant>:user!<userId>login!status` | traya-api-server |
| user case cache | `user_case:<userId>` | traya-api-server (read-only here) |

  Prefixing alone would break the migration, since the Node services keep writing the bare names. So both prefixed keys carry a **legacy fallback**, built in `internal/common/rediskey.go`:

  - **reads** try the prefixed key first, then the legacy one (`common.SharedKeyCandidates`);
  - **writes** only ever use the prefixed key, so this service never pollutes another tenant's namespace;
  - **invalidation deletes both**, otherwise traya-app-backend would keep serving a stale calendar after a log.

  `common.LegacyRedisFallback` (default `true`) is the kill switch. Flip it to `false` once traya-api-server and traya-app-backend adopt the `traya:` prefix; after that the bare keys are ignored and the tenants are fully isolated. The fallback is a no-op for mool and acne, which have no Node writer of these keys.

## 4. Data model

All Mongo collection names, field names and types are exactly those in the inventories. New indexes
(created idempotently per tenant DB on first resolve, in the background, failures logged not fatal):

| Collection | Index | Purpose |
|---|---|---|
| `reward_transactions` | `{user_id:1, status:1, expire_at:1, all_coins_used:1}` | balance aggregation |
| `reward_transactions` | `{user_id:1, createdAt:-1}` | history |
| `reward_transactions` | partial UNIQUE `{user_id:1, idempotency_key:1}` where `idempotency_key` exists | idempotent credits |
| `redeem_reward_transactions` | partial UNIQUE `{user_id:1, order_id:1}` where `order_id` exists and `status:'success'` | one redemption per order |
| `redeem_reward_transactions` | partial UNIQUE `{user_id:1, shop_flo_txn_id:1}` where exists | one redemption per Shopflo txn |
| `user_activity_logs_for_bah` | `{user_id:1, is_active:1, check_ins_for_date:-1}` | as app-backend |
| `streak_logs` | `{user_id:1, is_active:1}` | |
| `streak_masters` | UNIQUE `{slug:1}` | as app-backend |
| `scratch_cards` | UNIQUE `{user_id:1, source:1, reward_day_date:1}`, `{user_id:1, status:1}` | as app-backend |
| `user_habit_tracker_badges` | UNIQUE `{user_id:1, kit_number:1}` | |
| `habit_tracker_lifeline_logs` | UNIQUE `{user_id:1, date_covered:1}` | |
| `user_bah_archived_products` | UNIQUE `{user_id:1}` | |
| `customeractivitylogs` | `{case_id:1, event:1, createdAt:-1}` | |

Idempotency keys are written into a dedicated `idempotency_key` field, not into `credit_remarks`.
MongoDB partial filters accept only equality, `$exists`, `$type` and range operators — not `$regex` —
so a prefix match on `credit_remarks` cannot back a partial unique index, and `credit_remarks` is
human-facing copy that CRM grants legitimately repeat. `credit_remarks` still carries the same value
for display and for the cross-service existence check against rows app-backend wrote. The keys:
- legacy first-log bonus: `bah-legacy-first-log`
- legacy milestone: `bah-legacy-<slug>-<IST YYYY-MM-DD of checkInDate>` (slug of the 3/7/21 master)
- legacy streak-restart bonus: `bah-legacy-restart-<IST date>`
- v85: unchanged `habit-tracker-daily-<date>` / `habit-tracker-ladder-<date>`
Manual CRM grants and sync credits keep their human `credit_remarks` (not covered by the partial index).

Postgres (mool/acne tenant DBs) — `migrations/pg/logearn/001_dose_logs_cash_ledger.sql` creates `dose_logs`
and `cash_ledger` exactly as the TypeORM entities, plus `CREATE UNIQUE INDEX IF NOT EXISTS uq_cash_ledger_redeem_order
ON cash_ledger (customer_id, order_id) WHERE reason = 'REDEEM' AND order_id IS NOT NULL`. Applied with
`go run ./cmd/migrate --tenant mool` (not on startup).

## 5. Auth

| Strategy | Used by | Behaviour |
|---|---|---|
| `RequireJWT` (traya) | all public legacy/v85/CRM routes | `Authorization: Bearer <jwt>`; HS256 with `JWT_SECRET`; claims `id`→UserID, `caseId`→CaseID, `email`, `roles`, `first_name`, `phone_number`; Redis `<tenant>:user!<id>login!status` must be non-empty (falling back to the legacy unprefixed `user!<id>login!status` while `LegacyRedisFallback` is on). Missing header → 401 `{message:'No authorization header provided.'}`; invalid → 401 `{message:'Invalid or expired token.'}`. `authToken` (raw token) is kept on the identity for the community share URL. |
| `RequireV2Token` | `/latestOrderHowtoUseV2`, `/latestRoutineV2`, `/rewardBalance/:caseId` | `x-access-token` or `authorization` equals `"Bearer " + V2_FORM_DATA_TOKEN` (constant-time compare). 401 `{message:'Unauthorized'}`. |
| `RequireInternal` | app-backend-path routes, mint, redeem | `x-internal-token` equals `INTERNAL_SERVICE_TOKEN`. 401. |
| `RequireGateway` (mool/acne) | logearn routes | `x-user-info` JSON → `customer_id`, `case_id`, `gender`; else `customerId` query. Missing → 400 `'customerId is required'`. |
| `RequireAdmin` | `/extraRewardsToUser`, `/syncRewardBalanceWithShopFlo` | roles[0] ∈ {ADMIN, SUPER_ADMIN, TEAM_LEAD} else 403 `{Error:'Only admin and super admin are allowed'}` — only when `CRM_ADMIN_GUARD=true` (api-server does not guard these today; default false to preserve behaviour). |

`x-app-version` header, `appVersion` and `version` query params are read into `Identity.AppVersion` (int, 0 when absent).

## 6. Behaviour

Every function is ported from the pseudo-code in `docs/reference/inventory-*.md`. The following decisions
override the source; everything not listed is reproduced as-is, including copy strings, field order,
`null` vs missing keys, and quirks that clients already tolerate.

### 6.1 Fixes
1. `GET /bah/:userId/calendar` uses the token identity. Path segment is accepted and ignored.
2. Legacy credits are idempotent (keys above). A duplicate-key error is treated as "already credited" and is not an error to the caller.
3. Streak update filters include `is_active:true`.
4. `saveActivityLogs` past-date check compares IST dates, not day-of-month.
5. Multiple-log PUT computes the day bounds before querying (no in-place mutation), same bounds as before.
6. `coinTransaction` uses a `$unionWith` + `$lookup` aggregation with `$facet` for count and page; a missing streak master yields `streakName: ''`.
7. `extraRewardsToUser` resolves the phone number from caseId → cases → users correctly.
8. `kitGoal.navigation.url` is built with the real caseId and exactly one `/` after `FORM_BASE_URL`.
9. `/bah/coin/redeem` returns the real error (`{message}` with status) instead of a silent 200.
10. Redeem paths use a Mongo transaction (skipped when `ENVIRONMENT != production`, as config-service does) and the new unique indexes.
11. `dose_logs.log_date` is written at the IST day anchor; logearn redeem is idempotent per order.
12. Recommendation-service upstream 404/410 statuses and bodies pass through unchanged on the v85 scratch-card routes and the how-to-use routes.

### 6.2 Preserved quirks (explicitly)
`threeDaysStreakCount/sevenDaysStreakCount/twentyOneDaysStreakCount` always 0; `hasUserSeenBahUpdatedModalResult: true` literal; calendar `endDate` = today; O8+ enhanced coins display-only; `getPostLoggingModalContent` falls through to the ≥21 copy for streak 0; `coinDiscountCap {value:25}` on api-server's response; expiry ≈ 91 days; badges GET upserts; `is_lifeline` treated as absent in `getHabitTrackerData` (all docs count as logged); the `SHOPFLO_WALLET_API_ENPOINT` env name; the message text `User cannot log for date <JS Date string>` including the JS `Date.toString()` format `Thu Sep 18 2026 18:30:00 GMT+0000 (Coordinated Universal Time)`.

### 6.3 Time
`internal/common/istdate.go` provides, and every call site uses the one its source used:
- `ISTShift(t)` = `t + 330 min` (api-server `getClientTimeFromUtcTime` / `getIndianTime`).
- `UTCMidnight(t)` (api-server `setTimeZeroForDate`).
- `LocalStartOfDay/EndOfDay(t)` = UTC start/end of day (api-server `setTimeStartOfDay` with the process in UTC; `main.go` sets `time.Local = time.UTC`).
- `ISTDay(t)`, `ISTDateString(t)`, `ISTDaysBetween(a,b)`, `ISTDayAnchor(t)` = true Asia/Kolkata calendar (lifelines, logearn, `getRewardsCount`).
- `CalendarDaysDifference(a,b)` = `abs(UTCMidnight(b)-UTCMidnight(a))/24h` (api-server `calculateDaysDifference`).
Date formatting helpers reproduce moment tokens used: `DD MMM YYYY`, `D MMMM, YYYY`, `D MMM, YYYY`, `D MMM`, `MMMM YYYY`, `YYYY-MM-DD`.

### 6.4 The write seam
`POST /activityLogForBAH`:
- `isHabitTracker: true` → `habit.MintOrCredit(userID, streakDay, phone, checkInDate)` runs synchronously in-process; the response carries `scratchCard` (the active card view) on ladder days, `null` otherwise. `card` is present only when minted.
- `isHabitTracker` false/absent → `legacy.ProcessStreakAndRewards` is dispatched to a bounded worker pool (size `LEGACY_WORKERS`, default 32; queue 1024; when full the job runs inline). Response returns immediately with `scratchCard: null`.
- Both paths: bust `<tenant>:kit-tracker-calendar!<userId>` **and** the legacy `kit-tracker-calendar!<userId>`, mark tasks, enqueue the lifeline check, emit CCD.

CCD: `ccd.Publish(tenant, eventType, caseID, payload)` publishes JSON `{tenantId, eventType, caseId, payload, emittedAt}` to Redis channel `ccd_update`. Decision for runbook ticket 7: **emit from this service; api-server (owner of customer_computed_data) subscribes**. Publishing failures are logged, never fatal.

### 6.5 Lifelines
Redis-backed scheduler replaces BullMQ:
- ZSET `bah:<tenant>:lifeline:due` with member `<userId>|<checkDate>` and score `dueAt` (unix ms). `Enqueue` removes any member for the user first (upsert semantics of the BullMQ jobId).
- Worker: every `HABIT_LIFELINE_POLL_MS` (default 5000) pops members with score ≤ now via a Lua script that moves them to `bah:<tenant>:lifeline:processing` (visibility timeout 60s), runs `processLifelineJob` with concurrency 5, schedules `next` on completion, and removes from processing. Stuck members past the timeout are returned to `due`.
- Reconcile: daily at 05:00 IST (`30 23 * * *` UTC) under Redis lock `bah:<tenant>:lifeline:reconcile-lock` (TTL 10 min) — seeds a check for every active streak with none pending.
- Enabled with `HABIT_LIFELINE_WORKER_ENABLED=true` (default false, matching production today where the api-server worker is commented out). `HABIT_LIFELINE_GRACE_HOURS` default 4; `HABIT_LIFELINE_TEST_DELAY_MS` honoured.
- Credit on apply calls `habit.MintOrCredit` in-process.

### 6.6 External clients
One pooled `http.Client` per upstream (`MaxIdleConnsPerHost` from `<NAME>_MAX_SOCKETS`, timeout `HTTP_DEFAULT_TIMEOUT_MS` default 10000, retry once on connection reset for idempotent GETs):

| Client | Base env | Used for |
|---|---|---|
| recommendation | `RECOMMENDATION_SERVICE_BASE_URL` | `how-to-use/{caseId}`, `routine/{caseId}`; headers `Authorization: Bearer <V2_FORM_DATA_TOKEN>`, `x-tenant-id: traya` |
| order-service (traya) | `ORDER_SERVICE_BASE_URL` | `GET coin/expiry/month/<userId>` |
| order-service (tr) | `TR_ORDER_SERVICE_BASE_URL` | `GET /orders/orders/get-all-orders-for-app/{customerId}?voidType=nonVoid`, `GET /orders/orders/get-orders-by-order-ids?...`; header `x-tenant-id` |
| shopflo | `SHOPFLO_WALLET_API_ENPOINT`, `SHOPFLO_WALLET_ISSUER_ID`, `SHOPFLO_WALLET_MERCHANT_ID`, `SHOPFLO_WALLET_API_KEY` | ensure/credit/debit/read; amounts coins/10 |
| community | `COMMUNITY_BASE_URL` | share URL string only |
| cms (traya) | `CMS_SERVICE_BASE_URL` | 15-day check-in form, `feedback_v3` component |
| cms (tr) | `TR_CMS_SERVICE_BASE_URL` | `GET /api/carestack/config/{tenantId}` |
| config-service | `TR_CONFIG_SERVICE_BASE_URL` | `GET /static-content/data/OLD_TO_ACTIVE_VARIANT_IDS_MAP`, header `x-tenant-id`; cached 10 min |
| S3/CDN | `S3_IMAGE_BASE_URL`, `FORM_BASE_URL` | URL building |

Secrets are never logged; phone numbers are masked (`+91******1234`) in logs.

### 6.7 Error envelopes
Preserved per route family, table in `inventory-api-server-legacy-bah.md` §5 and `inventory-app-backend-v85-habit-tracker.md` §0. Unexpected panics → 500 with the family shape and message `Unexpected error occurred`.

## 7. Configuration

`.env.example` lists every variable with a comment. Required at boot: `MONGO_URI`, `SERVICES_CACHE_HOST`, `JWT_SECRET`, `V2_FORM_DATA_TOKEN`, `INTERNAL_SERVICE_TOKEN`, `DATABASE_*` (traya PG), `POSTGRES_WRITE_*` + `POSTGRES_DATABASE_SUFFIX`, `MONGO_DATABASE_SUFFIX`, `ENVIRONMENT`. Upstream base URLs are validated lazily on first use so a tenant with no Shopflo, say, still boots. `PORT` default 3000. Optional: `DEFAULT_TENANT` (see §3.3).

## 8. Testing

- Unit tests (`go test ./...`, no network): state resolver (16 cases), reward tiers/ladder, kit calculator (ported from `kit_calculator` fixtures), `currentKitStart`, kit windows, lifeline decision/schedule/reconcile, streak math (gap 0/1/2, cap at 21), post-log copy table, challenge banner table, banner widget table, dosage split for every `dosageCode`, coinTransaction row synthesis, calendar painting (both modes), badges derive/select, scratch card expiry labels, logearn `computeStreakDay`/`bonusForDay`/`zAmountForDay`/cap math, IST helpers around midnight boundaries, idempotency-key formatting. All copy strings asserted verbatim against the inventories.
- Repository/integration tests against real datastores, gated on `TEST_MONGO_URI`, `TEST_PG_URI` and `TEST_REDIS_ADDR` and skipped when those are unset (`docker-compose.test.yml` provides them; locally installed services work too). Each Postgres-backed package runs in its own schema so `go test ./...` can run packages in parallel: idempotent credit under concurrent replays yields one document; redeem FIFO drain; scratch-card reveal race (two concurrent reveals → one credit); lifeline apply idempotency; logearn ledger row-lock correctness under concurrency; index bootstrap.
- HTTP tests with Fiber's `app.Test` for auth middleware outcomes and envelope shapes per family.
- `cmd/parity`: reads a JSONL of `{tenant, method, path, headers, body}`, calls two base URLs, prints a field-by-field diff (including nulls and array order) — the runbook ticket 9 harness. Ignore-list for volatile fields (`txnDate`, timestamps).
- `go vet`, `staticcheck`, `gofmt` clean; `swag init` regenerates `docs/swagger.*`.

## 9. Cutover notes (for the runbook owner)

The full traffic-routing plan (api-server proxy shims → ALB weighted rules for Traya; consumer-gateway percentage ramp for mool/acne) is in `2026-09-18-rollout-plan.md`. Summary:
- Deploy this service, run `cmd/migrate` for mool/acne, verify indexes were created in each tenant Mongo DB.
- api-server flips one route per deploy to `${BAH_SERVICE_BASE_URL}<same path>` forwarding `Authorization`, `x-app-version`, `x-tenant-id: traya`. Both api-server deployments flip together.
- app-backend's home-page widgets keep calling their in-process services until ticket 20; nothing here blocks them.
- CCD: api-server subscribes to `ccd_update` before ticket 19 flips the write.
- Lifeline worker stays disabled until ticket 19; enable with `HABIT_LIFELINE_WORKER_ENABLED=true` in exactly one environment first.
