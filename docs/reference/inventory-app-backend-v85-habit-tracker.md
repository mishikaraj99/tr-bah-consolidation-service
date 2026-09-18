# Inventory — traya-app-backend v85 Habit Tracker

Source repo: `/Users/mishika/Desktop/traya/traya-app-backend` (branch `master`, ignore `dist/`).
Generated 2026-09-18. Ground truth for the **v85 economy**: kit-tracker page/calendar/badges, scratch cards,
lifeline derivation, archived products, coin redeem. Where this document and the JS disagree, the JS wins.

---

## 0. WIRING / CONVENTIONS

`server.js`: plain Express, **no auth middleware**. Mounts `/config`, `/user`, `/learn`, `/api/kit-setup`, `/bah`, `/bah/scratch-card`, `/spin-wheel`, `/order`; `errorHandler` last. `PORT||3000`.

**Identity**: `userId` from query (GET) or body (POST), validated with uuid `validate`. `caseId` from query, resolved via `helper/index.js#findValidCaseId(caseId, userId)`: valid uuid → as-is; else Redis `user_case:${userId}` (TTL 3600); else `caseRepository.findCaseIdByUserId(userId)` then cached. `x-app-version` header → `appVersion` (Number, 0 if NaN). Tenant: single-tenant `traya`.

**Success**: raw JSON, `200`. **Errors**: bah/scratch controllers → `{ message }` with `resolveHttpStatus(err, 500)` (`statusCode ?? status` if 100–599). cms controller → `errorHandler` → `{ status:false, statusCode, reason, message }`. Factories return plain `{status, message}`: bad request 400, not found **400** (sic), forbidden 403, unique 422, teapot 418, conflict 409.

**Datastores**: Mongo `MONGO_URL`; Redis ioredis `SERVICES_CACHE_HOST/PORT/PASSWORD/TLS` with `getCache/setCache(key,data,ttl=3600)/removeCache`; Postgres Objection+Knex `DATABASE_*` (orders/cases/users). BullMQ has only `learn-counter-sync` — **no lifeline queue here** (it lives in api-server).

---

## 1. ROUTES

### `/bah` (bahRoutes.js)
| Method | Path | Controller | Params |
|---|---|---|---|
| POST | `/bah/coin/redeem` | `bahController.redeemCoins` | body → `saveTheRedeemRewardTransactionForNonOrderTxn` |
| POST | `/bah/archive-product` | `bahController.archiveProductForBAH` | body `userId` (uuid), `productId` |
| POST | `/bah/unarchive-product` | `bahController.unarchiveProductForBAH` | body `userId`, `productId` |
| POST | `/bah/coin/habit-tracker-credit` | `scratchCardController.mintHabitTrackerScratchCard` | body `userId`,`streakDay`,`phoneNumber`,`checkInDate` |

### `/bah/scratch-card` (scratchCardRoutes.js)
| Method | Path | Controller | Params |
|---|---|---|---|
| POST | `/bah/scratch-card/habit-tracker-mint` | `mintHabitTrackerScratchCard` | same body |
| GET | `/bah/scratch-card` | `getScratchCards` | query `userId` |
| POST | `/bah/scratch-card/:id/reveal` | `revealScratchCard` | path `id`; body `userId` |

### `/config` (configRoutes.js)
| Method | Path | Controller | Params |
|---|---|---|---|
| GET | `/config/cms/kit-tracker-page` | `cmsController.getKitTrackerPage` | query `caseId`,`userId`; header `x-app-version` |
| GET | `/config/cms/kit-tracker-calendar` | `getKitTrackerCalendar` | same |
| GET | `/config/cms/kit-tracker-badges` | `getKitTrackerBadges` | query `caseId`,`userId` |
| GET | `/config/cms/bah-page` | `getBahConfig` | → `{ reminderOnBahPage: 'top'|'bottom'|'both' }` |

### Controller validation & responses (verbatim)
- `redeemCoins` → `200 { message: 'Redeem coin transaction executed successfully' }`. Service swallows all errors → controller returns `200` with empty body on failure. **Go: return the real error** (`{message}` with status) — a silent 200 is a bug, not a contract.
- `archive/unarchive`: `400 'Invalid or missing userId'`; `400 'productId is required'`; `400 'Cannot remove the last product'`. Success `200 { archivedProductIds: string[], activeProductIds: string[] }`.
- `mintHabitTrackerScratchCard`: `400 'Invalid or missing userId'`; `400 'Invalid or missing streakDay'` (integer ≥1); `400 'Missing phoneNumber'`; `400 'Missing checkInDate'`. `reward = SCRATCH_CARDS_ENABLED ? resolveHabitTrackerReward(streakDay) : null`; `isMilestone = reward && reward.type==='ladder'`. Milestone → mint; else → `creditHabitTrackerCoins`.
  - mint: `{success:true, minted:true, cardId, coins, type, card:{...activeCardView}}` | `{success:true, minted:false, paused:true}` | `{success:true, minted:false, alreadyCredited:true}` | `{success:true, minted:false, alreadyExists:true, card?}`
  - credit: `{success:true, credited:true, coins, type}` | `{success:true, credited:false, paused:true}` | `{success:true, credited:false, alreadyCredited:true}`
- `getScratchCards`: `400 'Invalid or missing userId'`.
- `revealScratchCard`: `400 'Invalid or missing userId'`, `400 'Missing card id'`; `404 'Scratch card not found'`, `410 'Scratch card has expired'`, `501 'Unsupported reward type'`.
- `getKitTrackerPage`: `400 'Invalid or missing userId'`, `400 'Invalid or missing caseId'`; `500 'Kit tracker data unavailable'`.
- `getKitTrackerCalendar`: same 400s; `500 'Kit tracker calendar unavailable'`. **Cache** only when `NODE_ENV !== 'development'`: key `` `kit-tracker-calendar!${userId}` ``, TTL 300.
- `getKitTrackerBadges`: same 400s; `500 'Kit tracker badges unavailable'`.

---

## 2. MODELS (all `timestamps:true`)

### `scratch_cards`
```
user_id String req; source String req default 'habit_tracker'; reward_type String enum ['coins','free_product'] default 'coins';
reward_value Number; reward_meta{ slug String, streak_day Number, tier String ('ladder'|'daily') };
reward_day_date String req ('YYYY-MM-DD'); phone_number String; status String enum ['active','claimed'] default 'active';
expires_at Date req; claimed_at Date default null; reward_transaction_ref ObjectId ref reward_transactions default null; credit_remarks String
```
Indexes: `{user_id:1, source:1, reward_day_date:1}` UNIQUE; `{user_id:1, status:1}`.

### `user_habit_tracker_badges`
`user_id String req; kit_number Number req; badge_id String; source String default 'auto'; earned_at Date req` — index `{user_id:1, kit_number:1}` UNIQUE.

### `habit_tracker_lifeline_logs`
`user_id String req; date_covered Date req; kit_start Date req; applied_at Date req; source String req default 'auto'; streak_day_at_apply Number` — index `{user_id:1, date_covered:1}` UNIQUE.

### `user_bah_archived_products`
`user_id String req; archived_product_ids [String] req default []; updated_at Date` — index `{user_id:1}` UNIQUE.

### `user_activity_logs_for_bah`
`user_id String req; check_ins_for_date Date req; product_prescriptions Array req; is_valid_for_streak Boolean req; is_active Boolean req; is_lifeline Boolean default false; log_source String enum ['bah','habit_tracker'] default 'bah'` — index `{user_id:1, is_active:1, check_ins_for_date:-1}`.

### `reward_transactions`, `redeem_reward_transactions`, `streak_logs`, `streak_masters` — see legacy inventory §2.1 (identical; `streak_masters` has unique `{slug:1}`).

### Postgres (read-only): `orders` (`id, user_id, case_id, status, delivery_date, created_at, order_meta.line_items[].variant_id, order_phone_number, order_display_id`), `cases` (`id, user_id`), `users` (`id, phone_number, gender`).

---

## 3. BUSINESS LOGIC

### 3.1 Constants — `services/constants/habitTracker.js`
```
COIN_TO_RUPEE_DIVISOR = 10
HABIT_TRACKER_HEADER = { overline:'KIT TRACKER', heading:'Did you use your kit today?' }
HABIT_TRACKER_CTA    = { label:'Log Now', action:'habit_tracker', param:'' }
HABIT_TRACKER_INTRO_HEADING = 'Use kit regularly to get discounts'
HABIT_TRACKER_INTRO_CTA = { label:'Explore Now', action:'habit_tracker', param:'' }
HABIT_TRACKER_INTRO_BENEFITS = [{key:'coins',label:'Earn Coins'},{key:'streak',label:'Start Streak'},{key:'lifelines',label:'Get Lifelines'}]
HABIT_TRACKER_INTRO_MILESTONE_DAYS = [1,2,3,4]
WINDOW_DAYS_BACK = 7 ; WINDOW_DAYS_FORWARD = 7
DAILY_COINS_BASE = 30 ; WEEKLY_STEP = 10 ; MAX_WEEK = 5
REWARD_LADDER = { 7:100, 14:150, 21:200, 28:250, 35:300 }
MAX_REWARD_DAY = 35 ; PAUSE_AFTER_LOGS = 35
KIT_WINDOW_DAYS_PER_KIT = 30 ; KIT_WINDOW_BUFFER_DAYS = 10
LIFELINES_TOTAL = 3
REORDER_CTA_MIN_KIT_AGE_DAYS = 21
REORDER_CTA_TEXT  = '{days} days since the last order was placed.' ; REORDER_CTA_LABEL = 'Order Next Kit'
DAY_LABELS = ['S','M','T','W','T','F','S']   // 0=Sun
BAH_ONBOARDING_WEBVIEW_MIN_APP_VERSION = 80 ; BAH_ONBOARDING_WEBVIEW_DELIVERED_WITHIN_DAYS = 30
FIRST_LOG_REWARD_COINS = 100
BADGE_THRESHOLD = 30 ; BADGES_TOTAL = 21 ; BADGE_IMAGE_MAX_KIT = 21
BADGE_IMAGES = { M: '.../Male_badges_{n}.png?v=1784111714', F: '.../Female_badges_{n}.png?v=1784111743', disabled: '.../Disable_Badge_{n}.png?v=1784111725' }
BADGES = { default: { images: BADGE_IMAGES, threshold: 30 } }
```
`utils/config.js`:
```
HABIT_TRACKER_DAILY_TIERS = [{week:1,slug:'habit-daily-week-1',coins:30},{2,'habit-daily-week-2',40},{3,'habit-daily-week-3',50},{4,'habit-daily-week-4',60},{5,'habit-daily-week-5',70}]
HABIT_TRACKER_LADDER = [{days:7,slug:'habit-ladder-7',coins:100},{14,'habit-ladder-14',150},{21,'habit-ladder-21',200},{28,'habit-ladder-28',250},{35,'habit-ladder-35',300}]
HABIT_TRACKER_MAX_STREAK_DAY = 35 ; HABIT_TRACKER_COIN_EXPIRY_DAYS = 90
HABIT_TRACKER_DAILY_REMARKS_PREFIX = 'habit-tracker-daily-' ; HABIT_TRACKER_LADDER_REMARKS_PREFIX = 'habit-tracker-ladder-'
HABIT_TRACKER_IS_FOR_SUPERADMIN = false ; HABIT_TRACKER_SCRATCH_CARDS_ENABLED = true ; HABIT_TRACKER_SCRATCH_CARD_WINDOW_DAYS = 7
GENDER = { MALE:'M', FEMALE:'F' }
```
`HABIT_TRACKER_ASSETS` (prefix `https://cdn.shopify.com/s/files/1/0100/1622/7394/files/`):
```
coins: Coin.svg?v=1783944580 ; streak: Streak.svg?v=1783925135 ; lifeline: lifeline.svg?v=1783925135 ; lifelineUsed: lifeline-used.svg?v=1783925135
lifelineDisabled: lifeline-disabled.svg?v=1784804191 ; rewardUpcoming: Property_1_gift_new.svg?v=1783925135 ; rewardReceived: reward-received.svg?v=1783925135
rewardLocked: reward-grey.svg?v=1783925134 ; locked: Property_1_locked.svg?v=1783925135 ; streakStaysAlive: First_Rewards_Container.svg?v=1785332255
streakBreaksBS: Streak_breaks.svg?v=1785331824 ; coinsLocked: Property_1_coins.svg?v=1783925135 ; streakBreak: streak-break-svg.svg?v=1785332818
streakDisabled: Streak-disabled.svg?v=1784804060 ; kitBox: KitArriving.png?v=1784719927 ; kitImage: KitImage_893677a2-53da-4752-99c2-c04637eaf92d.png?v=1784719525
scratchTileCover: all-cards-scratch.png?v=1784719526 ; expiredTileCover: all-cards-expired.png?v=1784719524
scratchRevealThumbnail: ScratchRevealThumbnail_1.png?v=1785163313 ; noRewardScreen: noRewardScreen.png?v=1785333032
streakBreakMainCalender: streakBreakMainCalender.svg?v=1785339424
```

### 3.2 `habitTrackerStateResolver.js` — 16 states, first match wins
Helpers: `rupees(bal) = `₹${floor(bal/10)} off``; `lifelineLabel(used,total) = used<=0 ? `${total} Per Kit` : used>=total ? 'All Used' : `${used}/${total} Used``; `daysToNextReward(d)` = next of [7,14,21,28,35] > d minus d, else null. `coins = rupees(coinBalance)`; `actual = lifelineLabel(used,total)`. Returns `{state, heading, ctaLabel, coinsSubtitle, streakSubtitle, lifelinesSubtitle}`; `\n` literal.

| # | Condition | state | heading | cta | coinsSub | streakSub | lifelinesSub |
|---|---|---|---|---|---|---|---|
|1|`kitArrivingIntro`|`kit_arriving_intro`|`Use kit regularly to get discounts`|`Explore Now`|coins|`Start Now`|`${total} Per Kit`|
|2|`earningPaused && !loggedToday`|`rewards_paused_unlogged`|`Rewards are paused.\nOrder now to start earning again`|`Log Now`|coins|`Keep Going!`|actual|
|3|`earningPaused && loggedToday`|`rewards_paused_logged`|same|`View Logs`|coins|`Keep Going!`|actual|
|4|`newOrderPlacedNotDelivered`|`new_order_placed`|`New kit loading! You can continue\nlogging your current kit.`|`Log Now`|coins|`Streak Intact`|`${total} Per Kit`|
|5|`streakBroken && brokeOnRewardDay`|`streak_broken_reward_day`|`Your streak broke! Start afresh today`|`Log Now`|coins|`Start Today`|actual|
|6|`streakBroken`|`streak_broken_coins_day`|`Your streak broke! Start afresh today`|`Log Now`|coins|`Start Today`|actual|
|7|`used >= total && !loggedToday`|`lifelines_all_used_unlogged`|`No lifelines left\nLog today to keep your streak alive`|`Log Now`|coins|`Save Streak`|`All Used`|
|8|`used > 0 && !loggedToday`|`lifeline_used_unlogged`|`Streak saved! We used ${used} ${used===1?'lifeline':'lifelines'}\n${remaining} ${remaining===1?'lifeline':'lifelines'} remaining`|`Log Now`|coins|`Save Streak`|`${used}/${total} Used`|
|9|`newKitDeliveredNotLogged`|`new_kit_delivered`|`Do not miss out on your rewards!\nLog your dose now`|`Log Now`|coins|`Keep Going!`|`${total} Per Kit`|
|10|`rewardToday && loggedToday`|`reward_day_logged`|`Bonus unlocked!\nNext weekly reward is waiting for you`|`View Logs`|coins|`Keep Going!`|`${total} Per Kit`|
|11|`rewardToday && !loggedToday`|`reward_day_unlogged`|`Today is the day!\nClaim your weekly reward`|`Claim Reward`|coins|`Keep Going!`|`${total} Per Kit`|
|12|`dayBeforeReward && loggedToday`|`day_before_reward_logged`|`All done for today\nLog tomorrow to earn the bonus`|`View Logs`|coins|`Keep Going!`|`${total} Per Kit`|
|13|`dayBeforeReward && !loggedToday`|`day_before_reward_unlogged`|`Tomorrow is bonus day,\nLog today to get there`|`Log Now`|coins|`Keep Going!`|actual|
|14|`!loggedToday && hasLoggedEver`|`active_streak_unlogged`|`${n} days closer to the mystery bonus` with `n = daysToNextReward(effectiveDay-1)`; null → `Keep your streak going!`|`Log Now`|coins|`Keep Going!`|actual|
|15|`loggedToday`|`day_logged`|`All done for today\nLog daily to earn the bonus reward`|`View Logs`|coins|`Keep Going!`|actual|
|16|fallback|`day1_never_logged`|`Did you use your kit today?`|`Log Now`|`Earn Discount`|`Start Now`|`${total} Per Kit`|

Inputs: `loggedToday, hasLoggedEver, effectiveDay, rewardToday, dayBeforeReward, streakBroken, brokeOnRewardDay, kitArrivingIntro, newOrderPlacedNotDelivered, newKitDeliveredNotLogged, earningPaused, lifelinesUsed, lifelinesTotal, coinBalance`.

### 3.3 Coin/reward math
```
habitTrackerDailyCoins(day): week = clamp(ceil(day/7),1,5); return 30 + (week-1)*10
resolveHabitTrackerReward(streakDay):
  if !isInteger || <1 || >35 → null
  ladder = LADDER.find(days==streakDay) → {type:'ladder', slug, coins}
  else week tier → {type:'daily', slug, coins}
ensureHabitTrackerStreakMaster(reward):   // upsert by slug, $setOnInsert
  isLadder → days = N from slug, displayName `Habit Tracker Ladder - Day ${days}`
  else days=0, displayName `Habit Tracker Daily - ${slug.replace('habit-daily-','').replace('-',' ')}`  // 'Habit Tracker Daily - week 1'
  fields: display_name, days, is_active:true, is_for_superadmin:false, reward_coins
creditHabitTrackerCoins({userId, streakDay, phoneNumber, checkInDate}):
  reward = resolve; !reward → {success:true, credited:false, paused:true}
  dateStr = moment.utc(checkInDate).format('YYYY-MM-DD'); remarks = (ladder?'habit-tracker-ladder-':'habit-tracker-daily-') + dateStr
  if existsCreditTransactionWithRemarks(userId, remarks) → {success:true, credited:false, alreadyCredited:true}
  master = ensure(reward); saveRewardTransaction({userId, streakMasterRef, phoneNumber, reason:remarks, expiryDurationInDays:90, customAmount:reward.coins})
  → {success:true, credited:true, coins, type}
creditCoinsService.saveRewardTransaction: today+1 day, then + expiry days, zero time (UTC midnight) → expire_at (≈91 days)
  insert reward_transactions { user_id, streak_master_id, credit_coins: customAmount||reward_coins, is_credit_transaction:true, is_debit_transaction:false,
     total_debit_coins:0, credit_remarks: reason||display_name, all_coins_used:false, status:'success', expire_at }
  // NO Shopflo call on this path (deliberately removed)
```

### 3.4 Scratch cards — `scratchCardService.js`
```
SCRATCH_SOURCE='habit_tracker'
ASSETS = { coverAsset: Property_1_gift_new.svg?v=1783925135, revealAsset: reward-received.svg?v=1783925135, noRewardScreen: noRewardScreen.png?v=1785333032 } (cdn prefix)
COINS_DISPLAY_TEXT = 'Bonus Coins Earned!'
toActiveCardView(card) = { id:_id, reward:{type:reward_type, value:reward_value, displayText:'Bonus Coins Earned!'}, coverAsset, revealAsset, expires_at, expired:false, status }

MINT (ladder days only):
  reward = resolve(streakDay); !reward → {success:true, minted:false, paused:true}
  dateStr = utc(checkInDate) 'YYYY-MM-DD'; expiresAt = utc(checkInDate)+7d
  creditRemarks = (ladder?'habit-tracker-ladder-':'habit-tracker-daily-')+dateStr
  if existsCreditTransactionWithRemarks → {success:true, minted:false, alreadyCredited:true}
  create { user_id, source, reward_type:'coins', reward_value: coins, reward_meta:{slug, streak_day, tier:type}, reward_day_date:dateStr, phone_number, status:'active', expires_at, credit_remarks }
  on 11000: existing = findMintedCardForRewardDay; card = active && not expired ? view : null → {success:true, minted:false, alreadyExists:true, ...(card?{card}:{})}
  success → {success:true, minted:true, cardId, coins, type, card: view}

LIST getScratchCards({userId}):
  cards = find({user_id}) sort createdAt desc
  claimed coin cards → coinExpiry via batched reward_transactions lookup by reward_transaction_ref (expire_at || claimed_at+90d)
  status=='claimed' → history.push({ id, reward:{type,value}, claimed_at, ...coinExpiryFields })
  status=='active' && expires_at<=now → expired.push({ id, reward:{type,value}, expires_at, expired:true, status:'expired', expiryLabel:`Expired on ${format('D MMMM, YYYY')}` })
  else active.push(view)
  return { active, expired, history, emptyTitle:'No scratch cards yet', emptySubtitle:'Log consistently to earn coin scratch cards', title:'Scratch Cards', noRewardScreen }
coinExpiryFields(expiresAt): {} if none; else { coinExpiresAt, coinExpired: <=now, coinExpiryLabel: expired ? `Expired on ${'D MMMM, YYYY'}` : `Expires on ${'D MMM, YYYY'}` }
Expiry is DERIVED, never stored.

REVEAL revealScratchCard({userId, cardId}):
  claimed = findOneAndUpdate({_id, user_id, status:'active', expires_at:{$gt:now}}, {status:'claimed', claimed_at:now}, {new:true})   // credit lock
  if !claimed: existing = findCardByIdForUser; !existing → 404 'Scratch card not found'; claimed → return {revealed:true, alreadyClaimed:true, reward:{type,value}, ...coinExpiryFields}; else 410 'Scratch card has expired'
  try:
    reward_type!='coins' → 501 'Unsupported reward type'
    existingTxn = findCreditTransactionByRemarks(userId, credit_remarks); if existingTxn: link; return {revealed:true, reward, alreadyCredited:true, ...coinExpiryFields(existingTxn.expire_at)}
    master = ensure({type:tier, slug, coins:reward_value}); {rewardRef} = saveRewardTransaction(... reason: credit_remarks, expiryDurationInDays:90, customAmount: reward_value)
    link(claimed._id, rewardRef._id) best-effort
    return {revealed:true, reward:{type,value}, ...coinExpiryFields(rewardRef.expire_at)}
  catch: reopenClaimedCard(claimed._id) (status active, claimed_at null); rethrow
getActiveCardForReveal({userId, rewardDayDate}) → {id, status} if active && not expired else null
```

### 3.5 `kitTrackerPageService.js`
```
KIT_GOAL_LOGS=30 ; KIT_GOAL_TITLE='Hair gets thick & strong' ; FEEDBACK_COMPONENT_NAME='feedback_v3' ; FEEDBACK_CARD_ORDER_COUNT=1
REORDER_BANNER_DAYS_PER_KIT=30 ; START_OFFSET=-9 ; PAUSE_OFFSET=5 ; CTA_LABEL='Order Next Kit'
REWARD_SCREEN_ASSETS = { coinAnimation: Coin.svg?v=1783944580, scratchCover: Property_1_gift_new.svg?v=1783925135, scratchReveal: reward-received.svg?v=1783925135 }

buildReorderBanner(daysSinceDelivery, kitCount=1):
  not number → null; kits=max(1,kitCount); kitDays=30*kits; showFrom=kitDays-9; pause=kitDays+5
  days<showFrom → null; daysToPause=max(0,pause-days); paused=days>=pause
  text=`${d} ${d===1?'day':'days'} since the last order was placed.`
  → { text, daysToPause, daysSinceDelivery, kitCount:kits, paused, cta:{label:'Order Next Kit', action:'reorder'} }
dailyStripDays(days, today): todayIdx; endIdx = first idx>=todayIdx with isRewardDay else min(todayIdx+6, len-1); slice(max(0,endIdx-6), endIdx+1)
dailyLogDateLabel(cell, today): format('D MMM') + ', Today' if isToday; null if invalid
getRewardsCount(userId): active.length + history claimed today in Asia/Kolkata; errors → 0
getFeedbackCard({userId, caseId, version, orderCount}): null if !userId||!caseId; null if orderCount!==1; CMS getComponentDataFromNameArray(['feedback_v3'], userId, caseId, version, {mintFormSessions:true}) keyed by name; null if no contents
buildRewardScreen(core, {todayCell, daysToReward, weekDays, activeCard, scratchCardsEnabled, feedbackCard}):
  rawStreak = core.stats.streak.value||0 ; loggedToday = todayCell && (logged||lifelineUsed) ; projected = loggedToday ? raw : raw+1
  if todayCell.isRewardDay:
     scratchCard = { coverAsset, revealAsset, reward:{type:'coins', value: todayCell.coins, displayText:'Bonus Coins Earned!'} } (+ id,status when activeCard; autoCreditOnComplete:false when !enabled)
     rewardClaimed = enabled ? !activeCard : true
     title/subtitle = claimed ? (`${p}-day streak reward claimed`, 'Rewards already credited') : (`${p}-day streak reward unlocked!`, 'Scratch to reveal your bonus coins')
     → { mode:'reward', streak:p, title, subtitle, loggedToday, rewardClaimed, scratchCard, feedbackCard }
  else → { mode:'normal', streak:p, title:`${p}-Day Streak!`, loggedToday, coinsEarned: coins, coinsEarnedText: coins!=null ? `${coins} Coins Earned` : null,
           subCopy: daysToReward===null ? null : `Log ${d} more ${d===1?'day':'days'} to scratch mystery reward`, animationAsset: coinAnimation, days: weekDays||[], feedbackCard }
```
`assembleKitTrackerPage` response of `GET /config/cms/kit-tracker-page`:
```json
{ "state": core.state|null, "habitTrackerDisabled": core.state==='kit_arriving_intro',
  "header": { "title":"Kit Tracker", "badges":{"count":badgesCount,"icon":"Award"},
              "rewards":{"count":rewardsCount,"icon":"https://cdn.shopify.com/s/files/1/0100/1622/7394/files/Property_1_gift_new.svg?v=1783925135"},
              "reminder":{"enabled":true,"icon":"BellRing","iconOff":"BellOff"} },
  "kitGoal": { "logged": core.kitLogCount, "goal":30, "kitNumber": getAllDetailsRelatedToNonVoidOrders(orders).runningMonthForHairKit||0,
               "title":"Hair gets thick & strong", "topLevelText":"Kit <kitNumber> Goal",
               "navigation":{"action":"web_page","url":"${FORM_BASE_URL||'https://form.traya.health'}pages/care-plan/${core.caseId}?source=app"} },
  "stats": core.stats, "assets": core.assets||{}, "banner": ..., 
  "dailyLog": { "todayCoins": "₹<todayCell.coins>"|null, "dateLabel", "coinsLabel": "Earn <n> coins"|null, "days":[weekDays+dateLabel], "footer": ... },
  "rewardScreen": buildRewardScreen(...), "bottomSheets": core.bottomSheets||{}, "kitLogCount", "loggingEnabled", "earningPaused": core.calendar.earningPaused||false }
```
Derived: `todayCell`, `nextReward` (first idx>=todayIdx isRewardDay), `daysToReward`, `loggedToday`, `footerDaysToLog = daysToReward===null?null:(loggedToday?d:d+1)`, `weekDays`, `activeCard = (enabled && todayCell?.isRewardDay && userId) ? getActiveCardForReveal : null`.
banner: `kit_arriving_intro` → `{ text:'Start logging once your kit arrives' }`; else if `minDaysAfterOrderDelivered>=0 && !isOrderPlaced` → `buildReorderBanner(minDays, currentKitCount)` where `currentKitCount = kitExpireDays>0 ? round(kitExpireDays/30) : 1`; else null.
footer: `daysToReward===null` → null; `===0` → `!loggedToday ? 'Log today to unlock reward' : (enabled && activeCard) ? 'Scratch your reward to claim it' : 'Reward claimed'`; else `` `Log ${footerDaysToLog} more ${footerDaysToLog===1?'day':'days'} to unlock reward` ``.
Orchestration: `rewardsCount` first; `nonVoidOrders = getNonVoidOrders(caseId)` (PG); parallel `getHabitTrackerData({userId, version, nonVoidOrders})`, `getHabitTrackerBadges({userId, nonVoidOrders})`, rewardsCount, `getFeedbackCard(..., orderCount)`; `!core → 500 'Kit tracker data unavailable'`.
Quirks to FIX in Go: `core.caseId` is undefined (Go passes caseId through); `FORM_BASE_URL` joined without `/` (Go ensures one slash).

### 3.6 `getHabitTrackerData(context)` — core payload
```
todayM = startOf day; fetchStart = today - 42d; fetchEnd = endOf today
parallel: aggregateActiveCoinBalanceWithEarliestExpiry(userId) → [{balanceCoins, expireAt}] (match status success, expire_at>now, all_coins_used false; sum credit-debit; min expire_at)
          findActiveStreakSummary(userId); findActivityLogsForStreakWindow(userId,{from,to}) (proj check_ins_for_date,is_valid_for_streak); findLifelineLogDates(userId)
coinBalance = round(balanceCoins); coinExpiryDate = expireAt||null; currentStreak, lastLogDate, firstLogDate from streak
if version>=71 && lastLogDate && today.diff(lastLog,'days')>2 → currentStreak = 0
loggedDates = keys of all docs (is_lifeline not projected → always undefined → all docs); validDates = keys where is_valid_for_streak
loggedToday = isLoggedToday(lastLogDate); hasLoggedEver = !!firstLogDate; kitStartM = habitTrackerKitStart({orders, firstLogDate})
orderFlags = deriveHabitTrackerOrderFlags; hasDeliveredKit = !!lastDelivered(orders)
if newOrderPlacedNotDelivered && !hasLoggedEver && !hasDeliveredKit → return buildHabitTrackerIntro()
validLogCount = countValidStreakLogs(userId, since kitStartM)
lifelineDates = lifeline logs date_covered on/after kitStart; validDates ∪= lifelineDates; bridgedDays = lifelineDates; lifelinesUsed = size
liveRun = habitTrackerLiveRun(validDates, today)
breakAnchor = min(firstLogDate, earliest of loggedDates∪validDates∪bridgedDays)
logAndEarn = buildHabitTrackerLogAndEarn({today, currentStreak, loggedDates, validDates, firstLogDate: breakAnchor, coinBalance, bridgedDays})
effectiveDay = loggedToday ? liveRun : liveRun+1 ; rewardToday = LADDER[effectiveDay]!==undefined ; dayBeforeReward = LADDER[effectiveDay+1]!==undefined
streakBroken = hasLoggedEver && liveRun===0 ; brokeOnRewardDay = streakBroken && LADDER[lastRunBeforeGap+1]!==undefined
currentKitOrder = lastDelivered(orders)
kitCount = 1 ; runningKitStartDate = getCurrentRunningKitStartDate(orders) ; deliveryM = startOfDay(that)|null
windowOpenM = deliveryM; if deliveryM: firstInWindowLog = findFirstRealActivityLogOnOrAfter(userId, deliveryM) (is_active, is_lifeline≠true); if found windowOpenM = its day
earningPaused = habitTrackerEarningPaused({uniqueLogsInWindow: validLogCount, windowOpenM, today, kitCount})
windowDaysTotal = 30*kitCount+10 = 40 ; daysToPause = !windowOpenM ? null : paused ? 0 : max(0, 40 - today.diff(windowOpenM))
resolved = resolveHabitTrackerState({...}) → overwrite state/header.heading/cta.label/stats subtitles; stats.streak.value = streakBroken?0:currentStreak; stats.lifelines.used
bottomSheets = { coin_bottomsheet, streak_bottomsheet, lifeline_bottomsheet }; calendar.earningPaused; kitLogCount=validLogCount; loggingEnabled=!!currentKitOrder; daysToPause
errors → log + null
```
Valid log = `is_active:true && is_valid_for_streak:true` (lifeline docs count).
```
habitTrackerEarningPaused({uniqueLogsInWindow, windowOpenM, today, kitCount, maxLogs=35}): logs>=35 → true; !windowOpenM → false; today.diff(windowOpenM,'days') >= 30*kitCount+10
habitTrackerKitStart({orders, firstLogDate}) = startOfDay(getCurrentRunningKitStartDate(orders)) ?? startOfDay(firstLogDate) ?? null
deriveHabitTrackerOrderFlags: lastDelivered = delivered with max(delivery_date||created_at);
   newOrderPlacedNotDelivered = any status!=='delivered' && (!lastDelivered || created_at > deliveredTime)
   newKitDeliveredNotLogged = lastDelivered && hasLoggedEver && !loggedToday && no loggedDate on/after startOfDay(lastDelivered date)
habitTrackerLiveRun(validDates, today): cursor = has(today)?today:today-1; count consecutive back
habitTrackerLastRunBeforeGap(validDates, today): walk back from yesterday to a valid day (guard 400), count that run
buildHabitTrackerLogAndEarn: start=today-7; 15 cells
  runMap from (start-35) to today: valid → run+1; else if !bridged → run=0 (bridged carries)
  liveRun = has(today)?runMap[today]:(runMap[yesterday]||0); effectiveDay = has(today)?liveRun:liveRun+1
  cell i: d=start+i; diff=d-today; streakDayNumber = past ? runMap[d]||0 : effectiveDay+diff
     rewardCoins=LADDER[n]; isRewardDay; dailyCoins=habitTrackerDailyCoins(n); logged=!future&&loggedDates.has; validForStreak=!future&&valid.has; lifelineUsed=past&&bridged.has
     prevRun=runMap[d-1]||0; isStreakBreak = past && !valid && !lifelineUsed && prevRun>0 && (!firstLogDate || !d.isBefore(firstLogDate,'day'))
     → { date, dayLabel: DAY_LABELS[d.day()], dayOfMonth, isToday, isRewardDay, coins: isRewardDay?rewardCoins:dailyCoins, logged, lifelineUsed, isStreakBreak }
  return { header:{overline:'KIT TRACKER',heading:'Did you use your kit today?'}, cta:{label:'Log Now',action:'habit_tracker',param:''}, assets:{...ASSETS},
           calendar:{startDate, todayDate, days}, stats:{ coins:{value, label:'Total Coins', subtitle:'₹<floor(bal/10)> off', action:'coin_bottomsheet', icon:ASSETS.coins},
           streak:{value, label:'Current Streak', subtitle:'Keep going', action:'streak_bottomsheet', icon:ASSETS.streak},
           lifelines:{total:3, used:0, label:'Lifelines', subtitle:'0/3 used', action:'lifeline_bottomsheet', icon:ASSETS.lifeline, iconDisabled:ASSETS.lifelineDisabled} } }
buildHabitTrackerIntro(): { state:'kit_arriving_intro', habitTrackerDisabled:true, loggingEnabled:false, header:{overline:'KIT TRACKER', heading:'Use kit regularly to get discounts'},
   benefits:[{key:'coins',icon:ASSETS.coins,label:'Earn Coins'},{key:'streak',icon:ASSETS.streak,label:'Start Streak'},{key:'lifelines',icon:ASSETS.lifeline,label:'Get Lifelines'}],
   kit:{image:ASSETS.kitBox, badge:{label:'Arriving'}},
   milestones:[{day:1,label:'1st Log',coins:30,icon:ASSETS.coins,locked:false},{day:2,coins:30,icon,locked:true},{day:3,...},{day:4,...}],
   cta:{label:'Explore Now',action:'habit_tracker',param:''}, assets:{...ASSETS} }
```

### 3.7 Bottom sheets
```
coins({coinBalance, coinExpiryDate, orders}):
  rupees=floor(bal/10)
  header = bal>0 ? { title:`${bal} Coin balance`, pill:`Get ₹${rupees} off on your next kit`, conversion:'10 coins = ₹1' }
                 : { title:'Log daily to start earning coins', subtitle:'Earn coins, unlock rewards, & save on your next kit', conversion:'10 coins = ₹1' }
  expiry = (bal>0 && date) ? { text:`${bal} coins expire on ${format('DD MMM YYYY')}`, cta: resolveReorderCta(orders) } : null
  weeklyCoins[w=1..5] = { week:`Week ${w}`, days:`Day ${7(w-1)+1}-${7w-1}`, coins:30+(w-1)*10, label:`${coins} coins per day`, icon:ASSETS.coins }
  bonus = { icon: ASSETS.rewardUpcoming, text:'Unlock a bonus reward every 7 days you log' }
  note  = { title:'Note: Daily coins reset to 30 per day if you', points:['Break your streak','Start a new kit'] }
  resolveReorderCta(orders): minDaysAfterOrderDelivered < 21 → null; else { label:'Order Next Kit', action:'reorder' }
streak({currentStreak}):
  streak=max(0,n); nextBonus = first of [7,14,21,28,35] > streak else null
  header = streak===0 ? { title:'Start your streak today', subtitle:'Streak is the number of days in a row you have logged, even if a lifeline was used' }
                      : { title:`${streak}-Day Streak`, subtitle: nextBonus ? `Keep going to unlock your Day ${nextBonus} bonus.` : 'Keep your streak going!' }
  infoPoints = [ { icon:ASSETS.streak, text:'Streak stays alive when you log or when a lifeline is used.', example:ASSETS.streakStaysAlive },
                 { icon:ASSETS.streak, text:'Streak breaks only if a log is missed and no lifeline is available.', example:ASSETS.streakBreaksBS } ]
lifeline({lifelinesUsed, lifelinesTotal}):
  used=clamp; remaining=total-used; heartsRow(n) = total icons, first n lifelineUsed rest lifeline
  header = used===0 ? { title:`${total} Lifelines`, subtitle:'If you miss logging a day, we use 1 lifeline to keep your streak active.', hearts:heartsRow(0) }
                    : { title:`${remaining} of ${total} Lifelines left`, subtitle:'Keep logging daily to save your remaining lifelines', hearts:heartsRow(used) }
  infoPoints = [ { icon:ASSETS.coins, text:'You earn coins even when a lifeline is used.' },
                 { icon:ASSETS.lifeline, text:`You get ${total} lifelines per kit. Unused ones don't carry forward to next month.` },
                 { icon:ASSETS.streak, text:'Your streak breaks only if you miss a day and have no lifelines left.' } ]
  legend = [ {hearts:heartsRow(0), label:'Full Lifelines'} ] + for u in 1..total: { hearts:heartsRow(u), label: u===total ? 'All Lifelines Used' : `${u}/${total} Lifeline${u===1?'':'s'} Used` }
```

### 3.8 Lifeline derivation (pure; the write lives in api-server's worker — see legacy inventory §3.18)
```
habitTrackerLifelines({validDates, today, kitStartM, maxLifelines}): !kitStart → {liveRun:0, used:0, bridged:∅}
  cursor = has(today)?today:today-1; pending=[]
  while cursor >= kitStart: if valid.has(cursor): remaining=max-used; if pending.length>remaining: bridge tail(remaining); used=max; break
                                                  bridge pending; used+=len; pending=[]; liveRun+=1
                            else pending.push(cursor); cursor-=1d
habitTrackerLifelineDaysToApply({realValidDates, lifelineDates, today, kitStartM, maxLifelines}): !kitStart→[]; remaining=max-size; <=0→[]; has(today)→[]
  walk back from yesterday to kitStart pushing days not in lifelineDates; break on first real valid (anchor); !anchor→[]; reverse().slice(0,remaining)
applyHabitTrackerLifeline: see legacy §3.18 applyLifeline (identical semantics; upsert with $setOnInsert)
```

### 3.9 Badges
```
deriveHabitTrackerBadges({kitWindows, validDates, config, earnedByKit, gender, total=21}):
  genderKey = gender==='F'?'F':'M'; maxKit = max(21, keys...)
  for kit 1..maxKit: logsInKit = count validDates in [start,end); threshold = def.threshold||30; derivedEarned = logs>=threshold; earned = derived || earnedByKit.has
     newlyEarned when derived && !already; badges.push({ kitNumber, name:`Kit ${n}`, image: resolveImage(def,n,earned,gender), threshold, earned, logsInKit, earnedAt })
resolveImage: def.image if set; null if n>21; (earned ? images[gender] : images.disabled).replace('{n}', n)
formatEarnedAt: !earned||!date → 'Upcoming' else format('D MMMM, YYYY')
selectBadgesForDisplay: highestEarned; upcoming = first !earned with n>highest; cutoff = upcoming?n:highest; filter n<=cutoff && (earned || n===cutoff); map upcoming:!earned; sort DESC
getHabitTrackerBadges({userId, caseId, nonVoidOrders, gender}):
  orders; parallel findValidStreakLogDates(userId), findEarnedBadges(userId), gender||getUserDetails→gender (default 'M')
  kitWindows = getHabitTrackerKitWindows(orders); derive; if newlyEarned: upsert each {user_id, kit_number} $setOnInsert {source:'auto', earned_at:now}; backfill earnedAt
  return { earnedCount, badges: selectBadgesForDisplay(badges) }   // GET WRITES (upsert-on-read) — keep
getHabitTrackerKitWindows(orders): oldest-first; skip kitCount<=0; anchor delivery_date||created_at; subKitStart=max(prevKitEnd, orderDate); expand kitCount×30d; emit {kitNumber (global ++), start, end}
```

### 3.10 Calendar — `GET /config/cms/kit-tracker-calendar`
```
computeHabitTrackerRunState({today, rangeStart, activeStart, validDates}): runMap (rangeStart-35..today, valid?run+1:0, NO bridge carry); lastValid (probe back ≤400, stop before activeStart); bandDays = consecutive valid back from lastValid; liveRun = runMap[lastValid]||0; effectiveDay = has(today)?liveRun:liveRun+1
buildHabitTrackerMonthCalendar({month, year, today, firstLogDate, activeStart, loggedDates, validDates, lifelineDates, runState}):
  for dom 1..daysInMonth:
     state = future→'future' : logged.has→'logged' : lifeline.has→'lifeline' : (activeStart && key>=activeStart && key<today)→'missed' : 'none'
     lifeUsed = !future && lifeline.has ; dayRun = past ? runMap[key]||0 : effectiveDay+diff
     rewardCoins=LADDER[dayRun]; isRewardDay; rewardEarned = isRewardDay && !future && valid.has
     showsCoins = state in (logged, lifeline) || (isRewardDay && !future); coins = showsCoins ? (isRewardDay?rewardCoins:dailyCoins(dayRun)) : null
     streakBreak = state==='missed' && prevRun>0
     counters: logged→daysLogged++; lifeUsed→lifelinesUsed++; missed→daysMissed++, coinsMissed+=dailyCoins(prevRun+1), LADDER[prevRun+1]→rewardsMissed++
     day = { date, dayOfMonth, weekday: d.day(), isToday, state, isRewardDay, rewardEarned, coins, inStreakRun: bandDays.has, streakBreak, lifeUsed }
  month: { month, year, monthLabel: 'MMMM YYYY', firstWeekday, canGoPrev, canGoNext, days, legend, summary{daysLogged,daysMissed,coinsMissed,rewardsMissed,lifelinesUsed}, assets }
getHabitTrackerCalendar: { firstLogDate 'YYYY-MM-DD'|null, currentMonth:{month,year}, legend:[{key:'logged'},{key:'lifeline'},{key:'missed'},{key:'reward'}], assets, months:[...] }
  unbounded fetch of all activity logs + lifeline logs; loggedDates/lifelineDates mutually exclusive (is_lifeline → lifelineDates); rangeStart = startOfMonth(min(first_date_of_log, earliest activity)) else current month; iterate to current month, guard 60; runState once
  cache key `kit-tracker-calendar!<userId>` TTL 300 (prod only)
```

### 3.11 Archived products
```
getArchivedProductIds(userId) = Set(doc.archived_product_ids||[])
getUserProductIds(userId) = unique truthy product_ids of most recent {user_id, is_active:true} doc sorted check_ins_for_date desc
archiveProduct(userId, productId): !productId → 400 'productId is required'; [universe, archived]; active = universe \ archived
  if active.includes(productId) && active.length<=1 → 400 'Cannot remove the last product'
  updated = findOneAndUpdate({user_id}, {$addToSet:{archived_product_ids}, $set:{updated_at}}, {new, upsert}) → { archivedProductIds, activeProductIds: universe \ new set }
unarchiveProduct(userId, productId): !productId → 400; parallel(getUserProductIds, findOneAndUpdate({user_id}, {$pull, $set updated_at}, {new}) NO upsert) → { archivedProductIds, activeProductIds }
```

### 3.12 `/bah/coin/redeem` — `redeemCoinService`
```
saveTheRedeemRewardTransactionForNonOrderTxn({redeemedAmount, userId, remarks, shopFloTxnId, caseId}):
  userRef = userId ? findUserContactById : caseId ? case owner → user ; !userRef → 400 'User not found'
  saveTheRedeemRewardTransaction({ redeemedAmount, userId: userRef.user_id, remarks: remarks||`Coin redeemed - ${shopFloTxnId}`, shopFloTxnId, currency:'INR', phoneNumber, isJusPay:true })
  → { message:'Redeem coin transaction executed successfully' }
saveTheRedeemRewardTransaction:
  redeemedCoins = round(amount*10)
  {rewardBalance, records} = find({user_id, status:'success', expire_at:{$gt:now}, all_coins_used:false}).sort({createdAt:1}) (FIFO); balance=floor(Σ credit-debit)
  balance < coins → 400 'User have not enough coins'
  dup guard: orderId ? existsRedemptionForOrder → 400 'This order is already redeemed' : existsRedemptionForShopfloTxn → 400 'This reward is already redeemed'
  create redeem_reward_transactions { user_id, redeemed_coins, redeemed_amount, order_id|null, order_display_id|null, currency, status:'success', remarks: remarks||`Coin redeemed - ${orderDisplayId}`, shop_flo_txn_id }
  FIFO drain: per record usable=credit-debit; consume all (allCoinUsed) or partial; $push debit_transactions {debit_transaction_id, debit_coins, transaction_status, debit_remarks, debit_transaction_date}, $set is_debit_transaction:true, all_coins_used, total_debit_coins
  if isJusPay: createShopfloDebitRewardTransaction(redeemedCoins, redeemTxn._id, phone, orderDisplayId||shopFloTxnId)
```
Go: wrap the redeem in a Mongo transaction (prod) and add a unique partial index on `redeem_reward_transactions {user_id, shop_flo_txn_id}` / `{user_id, order_id}` where present.

### 3.13 Home-page widget handlers (NOT in scope — served by app-backend's home page; listed for context)
`habit_tracker.js` → `getHabitTrackerData(context)`; `build_a_habit_v3.js` → `getStreakAndRewardBalanceV2(context)` (app-backend's own copy of the banner logic with `coinDiscountCap {value:20}`). These stay in app-backend.

---

## 4. EXTERNAL / ENV
Shopflo: `SHOPFLO_WALLET_API_ENPOINT`, `SHOPFLO_WALLET_ISSUER_ID`, `SHOPFLO_WALLET_MERCHANT_ID`, `SHOPFLO_WALLET_API_KEY` (raw Authorization); debit body `{oid, reference_id, amount: coins/10, transaction_reference}`; 404/400 → `{}`. Credit NOT called on habit path.
Env: `MONGO_URL`, `SERVICES_CACHE_*`, `DATABASE_*`, `NODE_ENV`, `PORT`, `S3_IMAGE_BASE_URL`, `FORM_BASE_URL` (default `https://form.traya.health`), `CMS_SERVER_BASE_URL` (feedback_v3 component).
Redis keys: `kit-tracker-calendar!<userId>` 300s; `user_case:<userId>` 3600s.

## 5. GOTCHAS (decisions in spec)
1. `is_lifeline` never projected → all docs in `loggedDates` — KEEP (calendar path separates them).
2. Badges GET writes — KEEP.
3. `kitGoal.navigation.url` undefined caseId / missing slash — FIX.
4. Redeem returns 200 on failure — FIX (real error).
5. Coin expiry ≈91 days — KEEP.
6. Redeem dup guard non-atomic — FIX (index + tx).
7. Two run derivations (bridged vs not) — KEEP per surface.
8. `SCRATCH_CARDS_ENABLED` hardcoded true — make env `HABIT_TRACKER_SCRATCH_CARDS_ENABLED` default true.
9. Timezone: server-local moment (UTC in prod) except `getRewardsCount` (Asia/Kolkata) and mint/credit date strings (`moment.utc`). Port per call site.
