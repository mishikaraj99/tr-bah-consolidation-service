# Inventory — tr-consumer-backend (MOOL / ACNE) Log & Earn

Source repo: `/Users/mishika/Desktop/traya/tr/tr-consumer-backend` (NestJS, branch `master`, ignore `dist/`).
Generated 2026-09-18. Ground truth for the **logearn economy** used by the `mool` and `acne` tenants.
The legacy MOOL "BAH" module (`src/modules/build-a-habbit/**`) is **not ported** — summarised at the end for context only.

---

## 0. CONVENTIONS

- No global prefix, no versioning. Controller paths are full paths. Success = raw JSON; POST returns **201** by Nest default (Go: keep 201 for logearn POSTs).
- Error shape (all filters): `{ "message": string, "statusCode": number, "timestamp": ISO8601, "path": request.url }`. Validation arrays are joined with `' / '`, each capitalised.
- **No guards**. Identity comes from query params (`customerId`) and the `x-tenant-id` header. Auth is terminated upstream in `tr-consumer-api-gateway`:
  - Public path shape: `/consumer-api/service/consumers/logearn/<route>` → proxied to `TR_CONSUMER_SERVICE_BASE_URL + /consumers/logearn/<route>`.
  - Gateway accepts `authorization: Bearer <jwt>` / `x-login-token` / `x-access-token` (static `PUBLIC_API_TOKEN`); errors `UnauthorizedException('Authorization token is missing')` / `('Invalid token or token expired. Please login again.')`.
  - Forwarded headers: `x-tenant-id`, `x-source` (default `WEB`), `x-ip-address`, `x-fp-id`, `x-trace-id`, `content-type`, **`x-user-info`** (JSON), `user-agent`, `locale`.
  - `x-user-info` fields: `<tenant>_user_id` per tenant, `traya_user_id`, `first_name`, `case_id`, `gender`, `customer_id` (= `decoded.sub ?? decoded.id`).
  - In practice the backend reads `customerId` from the query and ignores `x-user-info`. **Go**: identity = `x-user-info.customer_id` when present, else `customerId` query (same trust model as today).
- Tenancy: `x-tenant-id` or `?tenantId`; missing → 400 `'Tenant ID header missing'`. `TENANTS = { acne:'acne', traya:'traya', mool:'mool', trayauae:'trayauae', kibo:'kibo', weightwise:'weightwise' }`.
- **Mongo**: master DB collection `tenant` `{tenant_id, tenant_name}`; tenant DB `${tenant_id}_${MONGO_DATABASE_SUFFIX||'dev'}`; special case `NODE_ENV!=='development' && tenant==='traya'` → `TrayaProd`. URI `${MONGO_URL}/${db}?${MONGO_OPTIONS}`.
- **Postgres**: tenant DB `${tenant_id}_${POSTGRES_DATABASE_SUFFIX||'user_service'}`; master `POSTGRES_MASTER_DATABASE_NAME` holds table `tenants (tenant_id PK, name)`; write host `POSTGRES_WRITE_*`, read replica `POSTGRES_READ_*`; snake_case naming; prod `ssl: DATABASE_SSL==='false'` (sic), pool `DATABASE_MAX_CONNECTIONS||100`.
- Redis (`REDIS_HOST/PORT/PASSWORD`) — **no logearn keys**.
- Env (`.env.sample`): `POSTGRES_*`, `MONGO_*`, `REDIS_*`, `TR_ORDER_SERVICE_BASE_URL`, `TR_CMS_SERVICE_BASE_URL` (runtime), `S3_IMAGE_BASE_URL` (runtime), `CMS_SERVER_BASE_URL`, `KAFKA_*`, others unrelated.

---

## 1. ROUTES — `LogEarnController` base `consumers/logearn`

| Method | Path | Input | Response |
|---|---|---|---|
| GET | `/consumers/logearn/state` | query `customerId` (required → 400 `'customerId is required'`), header `x-tenant-id` | §3 `state` |
| POST | `/consumers/logearn/log` | query `customerId`, `orderId?`; header | `{ success, streakDay, inrCredited, earningCapHit, balance }` |
| POST | `/consumers/logearn/backfill` | query `customerId`, `orderId?`; header | same as log |
| POST | `/consumers/logearn/lifeline` | query `customerId`; header | `{ success:true, message:'Lifeline applied. Streak recovered.', balance, priorStreakDay, daysBridged }` |
| POST | `/consumers/logearn/redeem` | query `customerId`; header; body `RedeemDto` | `{ success:true, redeemed, balance }` |

`RedeemDto`: `subtotal` int ≥1 required; `amount?` int ≥1; `orderId?` uuid.

Related (context, not ported): `GET /consumers/medicine/howToUse?customerId&isLastOrderMedicineRequired`, `GET /consumers/medicine/latestMedicineForBah?customerId`.

---

## 2. POSTGRES MODELS (tenant DB)

**`dose_logs`**
```
id uuid PK; customer_id uuid NOT NULL; order_id uuid NOT NULL;
log_date timestamp NOT NULL  -- 'Stored in UTC. IST conversion done in application code.'
streak_day integer NOT NULL; z_tier integer NOT NULL; inr_credited integer default 0; is_backfill boolean default false;
created_at timestamp default NOW()
UNIQUE INDEX "UQ_dose_logs_customer_date" (customer_id, log_date)
```
Caveat: TS writes `logDate = new Date()` (full timestamp) so the unique index does not enforce one-per-day. **Go writes `istDayAnchor(now)`** (UTC instant of 00:00 IST) so the index works. Reads compare by IST date string.

**`cash_ledger`**
```
id uuid PK; customer_id uuid NOT NULL; order_id uuid NULL; amount integer NOT NULL (negative debits, 0 for LIFELINE_RECOVERED);
reason varchar NOT NULL; balance_after integer NOT NULL; created_at timestamp default NOW()
INDEX (customer_id); INDEX (customer_id, reason)
```
`reason` ∈ `DOSE_LOG, BACKFILL, FIRST_EVER_BONUS, DAY3_BONUS, MILESTONE_BONUS, REORDER_WELCOME, LIFELINE_RECOVERED, REDEEM`.

`customer` (read-only): `id`, `gender`.

IST helpers (`src/common/utils/ist.ts`, correct): `IST_OFFSET_MS = 5.5h`; `istDateOf(d) = (d+offset).toISOString().slice(0,10)`; `istDayAnchor(d) = Date.parse(istDateOf(d)) - offset`; `istDaysBetween(a,b) = max(0, round((parse(istDate(b)) - parse(istDate(a)))/86400000))`.

---

## 3. LOG & EARN LOGIC — `logearn.service.ts`

### Constants
```
DEFAULT_Z_TIERS = [ {upTo:7, amount:2}, {upTo:14, amount:3}, {upTo:null, amount:4} ]     // ₹/day by streak day
DEFAULT_BONUSES = { firstEver:5, dayThree:5, reorderWelcome:5, milestoneEvery:7, milestoneAmount:10 }
DEFAULT_CAP_INFO = { bulkX:1, treatmentDay:0, earningCapDays:30, totalWindowDays:45, earningCapHit:false, windowExpired:false,
                     earningDaysConsumed:0, earningDaysRemaining:30, inGracePeriod:false, deliveryDate:null, orderId:null }
DEAD_ORDER_STATUSES = ['void','cancelled','returned','rto','lost','damaged','ghost']
redeem cap: maxRedeem = floor(subtotal * 0.5)
```

### Pure helpers
```
zAmountForDay(streakDay, zTiers) → first tier with (upTo===null || streakDay<=upTo) → { tier: index, amount }
bonusForDay(streakDay, isFirstEver, b): 0 + (isFirstEver?5:0) + (streakDay===3?5:0) + (streakDay>0 && streakDay%7===0 ? 10 : 0)
computeStreakDay(previousLogs, targetDate, lifelineBridged):
   empty → {streakDay:1, isFirstEver:true}
   lastLog = newest by logDate; gap = istDaysBetween(lastLog.logDate, targetDate)
   gap<=2 || lifelineBridged → {streakDay: last.streakDay+1, isFirstEver:false} ; else {streakDay:1, isFirstEver:false}
   // one missed day (gap 2) continues; gap ≥3 resets
```

### Cap info
```
getEarningCapInfo(customerId, tenantId):
  orders = OrderService.getAllNonVoidOrdersByCaseId(tenantId, customerId)     // GET ${TR_ORDER_SERVICE_BASE_URL}/orders/orders/get-all-orders-for-app/{customerId}?voidType=nonVoid, header x-tenant-id
  latestOrder = orders sorted by (delivery_date||created_at) DESC, first with status==='delivered'
  inFlight = any order whose lowercased `-`→`_` status is non-empty, !== 'delivered', not in DEAD_ORDER_STATUSES
  !latestOrder → { ...DEFAULT_CAP_INFO, orderId:null, hasInFlightOrder: inFlight }
  bulkX = Number(latestOrder.bulk_order_duration)||1, then override from getAllOrderDetailsByOrderId(latestOrder.id).bulkOrderDuration (best-effort)
  deliveryDate = new Date(delivery_date||created_at); treatmentDay = max(0, floor((now-deliveryDate)/86400000))
  earningCapDays = bulkX*30; totalWindowDays = earningCapDays+15
  earningDaysConsumed = COUNT(dose_logs where customer_id AND log_date >= deliveryDate)
  earningCapHit = consumed >= earningCapDays; windowExpired = treatmentDay > totalWindowDays; inGracePeriod = capHit && !expired
  on error → { ...DEFAULT_CAP_INFO, orderServiceFailed:true }
getRewardsConfig(tenantId):  // cached per instance, no TTL — Go: cache 10 min per tenant
  GET ${TR_CMS_SERVICE_BASE_URL}/api/carestack/config/${tenantId}, headers x-tenant-id, Content-Type json
  rewards = data.data.rewards ?? data.rewards ?? {}; { zTiers: rewards.zTiers ?? DEFAULT_Z_TIERS, bonuses: {...DEFAULT_BONUSES, ...rewards.bonuses} }; errors → defaults
currentBalance(customerId) = latest cash_ledger row (created_at DESC).balance_after ?? 0
creditCash/debitCash: inside a transaction, SELECT latest row FOR UPDATE, insert {customer_id, order_id||null, amount (±), reason, balance_after}; debit throws 400 'Insufficient cash balance' when balance<amount
isLifelineBridgingGap(customerId, lastLogDate): latest LIFELINE_RECOVERED entry exists AND created_at > lastLogDate
```

### `state(customerId, tenantId)`
```
{ customerId, balance, streakDay (latest log's streakDay else 0), logDoneToday,
  zForNext (0 when earningCapHit else zAmountForDay(streak+1).amount), zTier,
  last7: [{ date:'YYYY-MM-DD' IST, streakDay, inrCredited, isBackfill }]  // newest first, up to 7 of last 14 logs
  backfillAvailable = latestLog && istDaysBetween(latestLog.logDate, now) <= 2 && !yesterdayLogged,
  lifelineAvailable = latestLog && !logDoneToday && gap > 2 && NOT (LIFELINE_RECOVERED entry with created_at >= capInfo.deliveryDate),
  bulkX, treatmentDay, earningCapDays, totalWindowDays, earningCapHit, windowExpired, earningDaysConsumed, earningDaysRemaining, inGracePeriod, orderId,
  products: [{ name, cartDisplayName, dosage, image_url:{ cartImgUrl, imageCdnUrl, singleImages } }] }
```
Products come from `getAllOrderDetailsByOrderId(capInfo.orderId)` line items: `dosage = productUsageInstructions[0].dosageDescription`; images `${S3_IMAGE_BASE_URL}${media.url}` for `usageType` ∈ `cart_cdn_images` (cartImgUrl) / `image_cdn_path` (imageCdnUrl) / `single_images` (singleImages); `name = productVariant.name`; `cartDisplayName = product name`. Failures → `products = []` with a logged error.

`getAllOrderDetailsByOrderId(orderId, tenantId)`: `GET ${TR_ORDER_SERVICE_BASE_URL}/orders/orders/get-orders-by-order-ids?columns=orders.customerId,orders.bulkOrderDuration,orderLineItem.quantity,productVariant.productCode,productVariant.name,productVariant.priceAmount,productUsageInstruction.instructionType,productUsageInstruction.dosageCode,productUsageInstruction.dosageDescription,productMedia.mediaType,productMedia.usageType,productMedia.url,product.shortDescription,product.longDescription&orderIds={orderId}&joins=orderLineItem,productVariant,productUsageInstruction,productMedia,product`, header `x-tenant-id`. Read `src/modules/logearn/logearn.service.ts:413-445` for the exact response walk before porting.

### `logToday(customerId, tenantId, orderId?)`
```
previousLogs = last 14 dose_logs desc
any log IST date === today → 400 'Already logged today'
capInfo; resolvedOrderId = orderId || capInfo.orderId
!resolvedOrderId → 400 (capInfo.hasInFlightOrder ? 'Your kit is on the way! Dose logging starts once it is delivered.' : 'No active order found. Please place an order first.')
capInfo.windowExpired → 400 'Earning window has expired. Please reorder to continue.'
lifelineBridged = isLifelineBridgingGap(customerId, previousLogs[0]?.logDate ?? null)
{streakDay, isFirstEver} = computeStreakDay(previousLogs, now, lifelineBridged)
{tier, amount:z} = zAmountForDay(streakDay, cfg.zTiers); bonus = bonusForDay(streakDay, isFirstEver, cfg.bonuses)
reorderBonus = 0; if streakDay===1 && !isFirstEver: existingWelcome = latest REORDER_WELCOME; if !existing || (deliveryDate && existing.created_at < deliveryDate) → reorderBonus = 5
effective* = capHit ? 0 : value ; total = z+bonus+reorder
INSERT dose_logs { customer_id, order_id: resolvedOrderId, log_date: istDayAnchor(now) /*Go*/, streak_day, z_tier: tier, inr_credited: total, is_backfill:false }
creditCash(z, 'DOSE_LOG', orderId) if >0 ; creditCash(bonus, isFirstEver ? 'FIRST_EVER_BONUS' : streakDay===3 ? 'DAY3_BONUS' : 'MILESTONE_BONUS') if >0 ; creditCash(reorder, 'REORDER_WELCOME') if >0
return { success:true, streakDay, inrCredited: total, earningCapHit, balance }
```

### `backfillYesterday(customerId, tenantId, orderId?)`
```
previousLogs empty → 400 'Log today first before backfilling.'
any log IST date === yesterday → 400 'Yesterday already logged'
same order/window guards as logToday
istDaysBetween(previousLogs[0].logDate, now) > 2 → 400 'Your streak has broken, so there is nothing to backfill. Use your lifeline to restore it, or log today to start a new streak.'
logsBeforeYesterday = logs with istDate < yesterday; {streakDay} = computeStreakDay(logsBeforeYesterday, yesterday, lifelineBridged); isFirstEver=false
total = capHit ? 0 : z+bonus
INSERT dose_logs { ..., log_date: istDayAnchor(yesterday), is_backfill:true }
if today's log exists: today.streak_day = streakDay+1; save
creditCash(z, 'BACKFILL'); creditCash(bonus, 'DAY3_BONUS'|'MILESTONE_BONUS')
return { success:true, streakDay, inrCredited, earningCapHit, balance }
```

### `useLifeline(customerId, tenantId)`
```
latestLog = newest; none → 400 'No streak to recover'
gap = istDaysBetween(latest.logDate, now); gap<=2 → 400 'Streak is not broken; lifeline not needed'
!capInfo.orderId → 400 (in-flight msg | 'No active order found. Please place an order first.')
LIFELINE_RECOVERED with created_at >= capInfo.deliveryDate → 400 'Lifeline already used for this order'
INSERT cash_ledger { amount:0, reason:'LIFELINE_RECOVERED', balance_after: currentBalance, order_id: capInfo.orderId }
return { success:true, message:'Lifeline applied. Streak recovered.', balance, priorStreakDay: latest.streakDay, daysBridged: gap }
```

### `redeem(customerId, subtotal, amount?, orderId?)`
```
balance = currentBalance; maxRedeem = floor(subtotal*0.5); redeemAmount = min(amount ?? balance, balance, maxRedeem)
redeemAmount <= 0 → 400 'Nothing to redeem'
debitCash(redeemAmount, 'REDEEM', orderId)
return { success:true, redeemed: redeemAmount, balance: currentBalance() }
// 24h cooldown is commented out in source — not ported
```
Go adds: when `orderId` is present, a `REDEEM` row for the same `(customer_id, order_id)` short-circuits with 400 `'This order is already redeemed'` (unique partial index `cash_ledger (customer_id, order_id) WHERE reason='REDEEM' AND order_id IS NOT NULL`).

---

## 4. LEGACY MOOL BAH MODULE (context only — NOT ported)
`src/modules/build-a-habbit/**` mounts 18 routes under `consumers/bah` (rewardBalance, streakAndRewardBalance, coinRewardHistory, bahHistory/:caseId, extraRewardsToUser, faqs, termsAndConditions, bahLogForGivenDate, bahDetailDateWiseLikePagination, activityLogsBAH, POST/PUT activityLogForBAH, multipleActivityLogForBAH, redeemCoins, refundCoins, streakMaster, newBahFlowEligibility). Mongo collections `user_activity_logs_for_bahs` (note plural), `streak_logs`, `streak_masters`, `reward_transactions`, `redeem_reward_transactions`. Its money-moving paths are no-ops because `this.tenantId` is never assigned. Copy is Traya-branded. Decision recorded 2026-09-18: MOOL/ACNE get Log & Earn only.
