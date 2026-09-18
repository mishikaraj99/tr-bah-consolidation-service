# tr-bah-service — Rollout and Gateway Cutover Plan

Date: 2026-09-18. Companion to `2026-09-18-tr-bah-service-design.md`. Ticket order follows `docs/reference/bah-migration-runbook.pdf` (Waves 0–4); this document adds the **traffic-routing mechanics** for each tenant and the go/no-go gates.

## 1. How traffic reaches BAH today

| Tenant | Client | Path today | Auth terminated in | Deployment |
|---|---|---|---|---|
| traya (app) | Traya mobile app | `https://<api-server host>/<route>` — no gateway, no prefix | `traya-api-server` (Passport JWT + Redis login gate) | ECS service `traya-api-server-prod-prod` behind an ALB |
| traya (CRM) | CRM console | same routes on `traya-internal-api-service-prod-prod` | `traya-internal-api-server` (same BAH component) | ECS service behind its own ALB |
| traya (v85) | api-server → app-backend | `${APP_BACKEND_SERVER_BASE_URL}/bah/*`, `/config/cms/kit-tracker-*` | none (internal) | ECS service `traya-app-backend` |
| mool, acne | MOOL/ACNE apps | `https://<gateway>/consumer-api/service/consumers/logearn/*` | `tr-consumer-api-gateway` (JWT / login token / static token → `x-user-info`) | gateway proxies to `TR_CONSUMER_SERVICE_BASE_URL` (tr-consumer-backend) |

The gateway picks the backend from the **fourth path segment** (`/consumer-api/service/<service>/…`) in
`src/api-gateway-proxy/api-gateway-proxy.service.ts#getRequestUrl`; `consumers` → tr-consumer-backend.

## 2. Principles

1. **Public URLs never change.** No mobile release is required at any step.
2. **One route per deploy, reads before writes, smallest traffic first** (runbook §06). A write never shares a deploy with a read.
3. **The new service is deployed and verified before any traffic moves.** Its own health, indexes and env are checked in production with synthetic calls first.
4. **Every flip is a config change, revertible in minutes** — an env var or a routing rule, never a code deploy of the old service (except the initial proxy shim).
5. **Both Traya deployments flip together** (app and CRM internal service) so the CRM and the app read the same store.
6. **Parity gate before every flip**: `cmd/parity` diff is clean (ignoring volatile fields) for both legacy and v85 cohorts on that route.

## 3. Phase 0 — Deploy dark (no traffic)

1. Provision `tr-bah-service` as an ECS service in the same VPC as api-server and tr-consumer-backend, with:
   - access to the Traya Mongo (`TrayaProd`), Traya Postgres (`DATABASE_*`), the tr Mongo/Postgres clusters, and the shared Redis (`SERVICES_CACHE_*`);
   - all env vars from `.env.example` populated (`SHOPFLO_WALLET_API_ENPOINT` and `COMMUNITY_BASE_URL` with trailing `/`);
   - `HABIT_LIFELINE_WORKER_ENABLED=false`, `CRM_ADMIN_GUARD=false`.
2. Register the `master.tenant` documents (`traya`, `mool`, `acne`) if not already present; set `TENANT_ECONOMIES`.
3. Run `go run ./cmd/migrate --tenant mool` and `--tenant acne` (creates `dose_logs`, `cash_ledger`, indexes) — no-op if tables exist.
4. Hit `/health`, then one authenticated read per economy with a staff account; confirm the Mongo index bootstrap ran (`db.reward_transactions.getIndexes()` shows the partial unique index) in each tenant DB.
5. Run the parity harness against production api-server and this service for the recorded legacy and v85 cohorts (runbook ticket 9). **Gate: zero diffs.**

Rollback: delete the service. Nothing points at it.

## 4. Phase 1 — Traya via api-server proxy shims (runbook Waves 2–3)

api-server keeps terminating auth; each flipped route becomes a thin proxy to `${BAH_SERVICE_BASE_URL}<same path>` forwarding
`Authorization`, `x-app-version`, `x-tenant-id: traya`, `x-trace-id` and the raw body/query, returning the upstream status and body unchanged.
One env flag per route in api-server (`BAH_PROXY_<ROUTE>=true`) selects proxy vs local handler, so a flip and its rollback are env changes on both Traya deployments.

Order (req/day from runbook §01; each step = deploy flag → watch 24h in Last9 on **both** services → next):

| # | Route | Watch |
|---|---|---|
| 1 | `GET /latestRoutineV2/:caseId` | count, error rate, p95 unchanged |
| 2 | `GET /latestOrderHowtoUseV2/:caseId` | + `reminderInfo` present when expected |
| 3 | `GET /medicinesForHowToUse` | |
| 4 | `GET /rewardBalance/:caseId` (+ internal `/rewardBalance/:customerId`) | balance parity spot checks |
| 5 | `GET /bah/:userId/calendar` | 403/400 rate (IDOR fix means mismatched path ids are now ignored, not errors) |
| 6 | `GET /coinTransaction` | pagination totals equal |
| 7 | `GET /bahLogForGivenDate` | `isPostApiNeeded/isPutApiNeeded` distribution unchanged; archived filter for v85 |
| 8 | `GET /streakAndRewardBalance` | modals/banner copy diff clean; CRM `/bahHistory` still renders |
| 9 | `PUT /multipleActivityLogForBAH` | 200 rate 100% (forgiving semantics) |
| 10 | `POST /activityLogForBAH` — **its own deploy** | reward_transactions per log per cohort = 1; scratch cards minted on ladder days; `kit-tracker-calendar!` keys invalidated |
| 11 | v85 routes (`/archiveProductForBAH`, `/unarchiveProductForBAH`, `/bah/scratch-card*`, `/kit-tracker-*`) — api-server points `APP_BACKEND_SERVER_BASE_URL`-based calls to this service instead, or flips them like the others | 404/410 pass-through preserved |
| 12 | CRM: `/bahHistory/:caseId`, `/streakMaster`, `/extraRewardsToUser/:caseId`, `/syncRewardBalanceWithShopFlo/:caseId` | CRM ops sign-off |

Preconditions for step 10 (runbook tickets 6, 7, 8, 9): idempotency index live (Phase 0), CCD decision recorded (api-server subscribes to Redis `ccd_update`), parity clean, dev-branch port re-baselined (this service *is* the rewrite).

Rollback for any step: set the flag back to `false` on both deployments. Data written by this service is in the same collections, so the local handler resumes seamlessly. For step 10 the idempotency key also protects against a double-credit during a half-flipped fleet.

## 5. Phase 2 — Traya direct routing at the ALB (remove the hop)

Once every route is proxied and stable for a week, move routing from api-server into the load balancer so api-server is no longer on the BAH path:

1. Create a target group for `tr-bah-service` on the api-server ALB.
2. Add listener rules (priority above the default api-server rule) with **path patterns** for the BAH routes: `/streakAndRewardBalance`, `/bahLogForGivenDate`, `/activityLogForBAH`, `/multipleActivityLogForBAH`, `/coinTransaction`, `/bah/*`, `/rewardBalance/*`, `/medicinesForHowToUse`, `/latestOrderHowtoUseV2/*`, `/latestRoutineV2/*`, `/kit-tracker-*`, `/archiveProductForBAH`, `/unarchiveProductForBAH`, `/streakMaster`, `/bahHistory/*`, `/extraRewardsToUser/*`, `/syncRewardBalanceWithShopFlo/*`. Use **weighted target groups** per rule: start 10% new / 90% api-server, then 50%, then 100%, each after a 24h watch. The ALB adds `x-tenant-id: traya` via a fixed header on the rule (or the service defaults the tenant to `traya` when the header is absent on a JWT-authenticated route — set `DEFAULT_TENANT=traya` on the Traya ALB deployment).
3. Repeat on the internal-api-service ALB for the CRM routes.
4. After 100% for a week, remove the proxy shims and the local BAH handlers from api-server (runbook ticket 21), keeping the tether functions (`calculateDaysDifference`, `getBahRunningLogDay`, `getLatestMedicines`, `getUserRewardBalance`, `saveRewardTransaction`) as api-server-internal code.

Rollback: set the rule weight back to 0% for the new target group (seconds), or delete the rule.

Why not go straight to Phase 2? The ALB approach cannot flip "one route per deploy with a 24h watch on both services" as cheaply as env flags on a service that already logs per route to Last9, and it needs the service to accept `Authorization` for both app and CRM populations at once. Phase 1 proves parity per route; Phase 2 removes latency and the api-server dependency.

## 6. Phase 3 — MOOL and ACNE via the consumer gateway

Change `tr-consumer-api-gateway`:

```ts
// api-gateway-proxy.service.ts
this.baseURLMap.bah = this.configService.get<string>('TR_BAH_SERVICE_BASE_URL');
private static readonly BAH_SUBSERVICES = new Set(['logearn']);           // later: 'bah'
getRequestUrl(requestUrl: string) {
  const [, , , serviceName, subService] = requestUrl.split('/');
  if (serviceName === 'consumers' && ApiGatewayProxyService.BAH_SUBSERVICES.has(subService)
      && this.rolloutService.isEnabled('bah_service', tenantId, customerId)) {
    return this.baseURLMap.bah;
  }
  …existing switch…
}
```

- `rolloutService.isEnabled` reads `BAH_SERVICE_ROLLOUT_PERCENT_<TENANT>` (0–100) and hashes `customer_id` from `x-user-info` so a given customer is always routed to the same backend during the ramp (sticky by customer avoids a user seeing two ledgers mid-day). Missing customer id → old backend.
- The gateway already forwards `x-tenant-id`, `x-user-info`, `x-trace-id`; nothing else changes. The new service reads identity from `x-user-info.customer_id` with `customerId` query fallback (design §5).
- **Data**: both backends write the same tenant Postgres (`dose_logs`, `cash_ledger`), so a percentage ramp is safe. Before ramping, run the SQL fix that anchors existing `dose_logs.log_date` to the IST day (`UPDATE dose_logs SET log_date = (timezone('Asia/Kolkata', log_date))::date AT TIME ZONE 'Asia/Kolkata' …`) in a maintenance window, so the unique index holds for new writes. Until then keep the ramp at ≤10%.
- Ramp per tenant: `mool` 5% → 25% → 100%, then `acne` the same, each step after a 24h watch of 4xx/5xx and `inrCredited` distribution.
- Rollback: set the percent to 0. No data migration needed.

After 100% for both tenants: remove `LogEarnModule` from tr-consumer-backend (and the inert legacy MOOL BAH module).

## 7. Observability and gates

- Last9 dashboards per route for `tr-bah-service`, api-server (both deployments), tr-consumer-backend: request count, 4xx/5xx, p95. Alert on any route where new-service 5xx > 0.5% or p95 > 2× baseline for 10 minutes.
- Business counters emitted by the new service as logs/metrics: `bah_credits_total{tenant,economy,kind}`, `bah_credit_duplicates_total`, `scratch_cards_minted_total`, `lifeline_applied_total`, `logearn_credits_inr_total`.
- Daily reconciliation query during Phases 1–3: per user, count of `reward_transactions` with `credit_remarks` like `bah-legacy-%` per IST day must be ≤ 1; `cash_ledger` `balance_after` must equal running sum.
- Go/No-Go for each flip: parity diff clean, previous route stable 24h, no open Sev-1 on BAH, on-call aware.

## 8. Lifeline worker and CCD

- Enable `HABIT_LIFELINE_WORKER_ENABLED=true` on exactly one task first (the worker is idempotent and Redis-locked), one week after Phase 1 step 10, when the v85 rollout decision (runbook ticket 22) is "roll out". Until then the producer enqueues and the ZSET simply accumulates (bounded: one member per user).
- CCD: api-server adds a Redis subscriber on `ccd_update` that calls the existing `processAsyncEvent('CCD_UPDATE', …)` handler. Deploy it before Phase 1 step 10. Verify `customer_computed_data.current_streak_count` keeps updating after the write flips.

## 9. Timeline (indicative, 1 route/day cadence)

| Week | Work |
|---|---|
| 1 | Phase 0 deploy dark, indexes, parity harness recordings |
| 2 | Phase 1 steps 1–6 (reads, small → medium) |
| 3 | Phase 1 steps 7–9; CCD subscriber deployed |
| 4 | Phase 1 step 10 (write) alone; then 11–12 |
| 5 | Phase 3 mool 5→25→100% |
| 6 | Phase 3 acne; Phase 2 ALB weighted rules 10% |
| 7–8 | Phase 2 to 100%; remove api-server shims and consumer-backend logearn module |
