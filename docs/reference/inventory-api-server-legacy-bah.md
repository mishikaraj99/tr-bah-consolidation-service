# Inventory — traya-api-server legacy BAH component

Source repo: `/Users/mishika/Desktop/traya/traya-api-server` (branch `master`), component root `server/components/BAH/`.
Generated 2026-09-18 by reading the JS. Line numbers refer to `server/components/BAH/handler.js` unless stated.
This is the ground truth for the **legacy 3/7/21 economy** and the **CRM routes**. Where this document and the
JS disagree, the JS wins — re-read the referenced lines.

> One section (§3.5 tail / §3.6 / start of §3.7, roughly `creditRewardCoinsToUser` through the start of
> `updateMultipleMedicineLogForUser`) was truncated in the capture. The missing pieces are reproduced in
> §3.5a and §3.6a from a direct read of handler.js:416-500 and 761-820.

---

# 0. FILE MAP

| File | Lines | Role |
|---|---|---|
| `index.js` | 604 | Route registration (`buildAHabbitRoute`) |
| `handler.js` | 4390 | All business logic |
| `bahValidator.js` | 69 | Joi validators |
| `habitKitWindow.js` | 44 | Kit-window math |
| `lifelineQueue.js` | 90 | BullMQ queue/producer |
| `lifelineWorker.js` | 113 | BullMQ consumer |
| `lifelineDecision.js` | 21 | Pure decision fn |
| `lifelineState.js` | 67 | Streak/kit state reads |
| `applyLifeline.js` | 75 | Idempotent lifeline write |
| `lifelineSchedule.js` | 15 | IST date math |
| `lifelineReconcile.js` | 22 | Pure reconcile |
| `lifelineReconcileCron.js` | 75 | Daily repeatable job |
| `medicineTrackerFAQs.json`, `medicineTrackerTCs.json` | | Static (deleted routes) |
| `test/*` | | 12 unit-test files — port these |

**Mount point:** `server/app.js:85` imports, `:284` calls `buildAHabbitRoute(app)`. Routes are mounted on the app ROOT — there is NO `/bah` prefix.

---

# 1. ROUTES (all 38, for reference; the Go service implements the subset marked in the spec)

`authenticateJwt` = `server/services/auth/passport_jwt.js`. `checkTokenForV2FormData` / `isUserAdminOrSuperAdmin` = `server/services/auth/index.js:549` / `:984`.

| # | Method | Path | Middleware | Handler | Params read |
|---|---|---|---|---|---|
| 1 | GET | `/streakMaster` | authenticateJwt | `getMasterStreakDataForExtraBonusStreak()` | — |
| 2 | POST | `/activityLogForBAH` | authenticateJwt | `saveActivityLogsAndCreateStreakAndGiveRewards` | `req.userId`, body: `isLogForToday`, `productPrescriptions`, `isHabitTracker` |
| 3 | PUT | `/activityLogForBAH` | authenticateJwt | `updateStreakLogForUser` | `req.userId`, body: `isLogForToday`, `productId`, `morningCheckIns`, `eveningCheckIns` |
| 4 | PUT | `/multipleActivityLogForBAH` | authenticateJwt | `updateMultipleMedicineLogForUser` | `req.userId`, body: `isLogForToday`, `logProductDetail[]` |
| 5 | GET | `/latestMedicines` | authenticateJwt | `getLatestMedicines(req.userId)` | — |
| 6 | GET | `/medicinesForHowToUse` | authenticateJwt | `getMedicinesForHowToUsePurpose(req.userId)` | — |
| 7 | GET | `/latestOrderHowtoUse/:caseId` | checkTokenForV2FormData | `getLatestOrderHowtoUse(caseId)` | path `caseId` |
| 8 | GET | `/latestOrderHowtoUseV2/:caseId` | checkTokenForV2FormData, uuidValidator | inline proxy | path `caseId`; query `orderId`, `cxType`, `lang` |
| 9 | GET | `/latestRoutineV2/:caseId` | checkTokenForV2FormData, uuidValidator | inline proxy | path `caseId`; query `productId`, `orderId`, `cxType`, `lang` |
| 10 | POST | `/uploadUsageDetails` | authenticateJwt, isUserAdminOrSuperAdmin | `uploadSkuUsageDetails(req.body)` | body |
| 11 | GET | `/activityLogsBAH` | authenticateJwt | `getActivityLogs` | `req.userId`, query `year`, `month` |
| 12 | GET | `/streakAndRewardBalance` | authenticateJwt | `getStreakAndRewardBalance` | `req.userId`, query `version`, header `authorization` (Bearer stripped → `authToken`) |
| 13 | GET | `/coinRewardHistory` | authenticateJwt | `getRewardCoinHistory` | `req.userId`, `req.body.isCreditTxn || true`, `req.body.isDebitTxn || true` |
| 14 | POST | `/extraRewardsToUser/:caseId` | authenticateJwt | `saveExtraBonusForUsersForAnyReason` | path `caseId`; body `streakMasterId`, `reason`, `customAmount` |
| 15 | GET | `/faqs` | authenticateJwt | `getFAQs()` | — |
| 16 | GET | `/termsAndConditions` | authenticateJwt | `getTermsAndConditions()` | — |
| 17 | POST | `/addCoinBalance` | authenticateJwt | `provideCoinsByPhoneNumbersForExistingUser` | body `phoneNumber` |
| 18 | GET | `/bahHistory/:caseId` | authenticateJwt | `getBahHistoryOfUser` | path `caseId`; query `isCreditTxn`,`isDebitTxn`,`year`,`month`; `req.userId`→`loggedInUserId`, `req.email` |
| 19 | GET | `/bahHistoryWeb/:caseId` | checkTokenForV2FormData | `getUserRewards(caseId)` | path `caseId` |
| 20 | GET | `/rewardBalance/:caseId` | uuidValidator then checkTokenForV2FormData | `getUserRewardBalance(caseId)` | path `caseId` |
| 21 | GET | `/shopflo/txn/:caseId` | authenticateJwt | `getShopfloWalletTransactionsByPhoneNumber` | path `caseId`, query `phoneNumber` |
| 22 | POST | `/shopflo/debitReward` | authenticateJwt | `createShopfloDebitRewardTransaction` | body `reward`, `referenceId`, `phoneNumber`, `reference` |
| 23 | POST | `/shopflo/creditReward` | authenticateJwt | `createShopfloCreditRewardTransaction` | body `reward`, `expiryTimestamp`, `streakId`, `phoneNumber` |
| 24 | POST | `/coinScript/:streakMasterId` | authenticateJwt, multer | `scriptForAddedCoins` | csv file, path `streakMasterId`, body `reason`, `customAmount` |
| 25 | GET | `/bahLogForGivenDate` | authenticateJwt | `getBahLogForGivenDate` | query `date`, `appVersion`; `req.userId`, `req.caseId`. `showBahV3 = Number(appVersion) >= 71` |
| 26 | POST | `/archiveProductForBAH` | authenticateJwt | `archiveProductForBAH` | `req.userId`, body `productId` |
| 27 | POST | `/unarchiveProductForBAH` | authenticateJwt | `unarchiveProductForBAH` | `req.userId`, body `productId` |
| 28 | GET | `/bah/scratch-card` | authenticateJwt | inline proxy | forwards `?userId=req.userId` |
| 29 | POST | `/bah/scratch-card/:id/reveal` | authenticateJwt | inline proxy | path `id`; body `{userId: req.userId}` |
| 30 | GET | `/bahDetailDateWiseLikePagination` | authenticateJwt | `getBahDetailWithDateWiseLikePagination` | query `date`, `req.userId` |
| 31 | GET | `/newBahFlowEligibility` | authenticateJwt | `checkUserEligibleForNewBahFlow` | `req.userId` |
| 32 | PUT | `/syncRewardBalanceWithShopFlo/:caseId` | authenticateJwt | `syncRewardBalanceWithShopFloCoinBalance` | path `caseId` |
| 33 | GET | `/bah/:userId/calendar` | authenticateJwt | `getBahCalendarLogData` | **path `userId` (IDOR — Go binds to token)**; query `date` (required), `mode` (default `'calendar'`) |
| 34 | GET | `/coinTransaction` | authenticateJwt | `getRewardCoinHistoryPaginated` | `req.userId`, query `page` (default 1), `limit` (default 6) |
| 35 | GET | `/bah-challenges-cx-data` | checkTokenForV2FormData | `bahChallengeData(caseId)` | query `caseId` |
| 36 | GET | `/kit-tracker-page` | authenticateJwt | proxy | `req.userId`, `req.caseId`, version = `query.appVersion || query.version || header['x-app-version']` |
| 37 | GET | `/kit-tracker-calendar` | authenticateJwt | proxy | same |
| 38 | GET | `/kit-tracker-badges` | authenticateJwt | proxy | same |

### Proxy targets (today; the Go service serves these in-process)

| Route | Client | Upstream path | Notes |
|---|---|---|---|
| 8 | recommendationService (`RECOMMENDATION_SERVICE_BASE_URL`, maxSockets 30) | `GET how-to-use/{caseId}` (query `orderId`,`cxType`,`lang` forwarded) | headers `Content-Type: application/json`, `Authorization: Bearer ${V2_FORM_DATA_TOKEN}`, `x-tenant-id: traya`; `passThroughError: true`. Then merges `reminderInfo` from `CustomerActivityLog` (newest doc with `reminder_days`, see §3.12a). |
| 9 | recommendationService | `GET routine/{caseId}` (query forwarded) | same headers |
| 28 | appBackendService | `GET /bah/scratch-card?userId=` | api-server re-wraps to 200 |
| 29 | appBackendService | `POST /bah/scratch-card/{id}/reveal` body `{userId}` | |
| 26/27 | raw axios | `POST ${APP_BACKEND_SERVER_BASE_URL}/bah/archive-product` / `unarchive-product` body `{userId, productId}` | returns `{archivedProductIds, activeProductIds}` |
| 36/37/38 | appBackendService | `GET /config/cms/kit-tracker-page|calendar|badges?userId&caseId`, header `x-app-version` | |
| mint (internal) | appBackendService | `POST /bah/scratch-card/habit-tracker-mint` body `{userId, streakDay, phoneNumber, checkInDate}` | never throws to caller; on error returns `{ success:false, message:'Failed to credit habit tracker coins' }` |

`axiosCallGenericFunction` (`server/components/proxy/axios.js`) always sets `Content-Type: application/json` and `x-tenant-id: traya`.

---

# 2. DATA STORES

## 2.1 Mongo (Mongoose; collection name = model name; `timestamps: true` → `createdAt`/`updatedAt`)

### `user_activity_logs_for_bah`
```
user_id            String   required
check_ins_for_date Date     required
product_prescriptions Array required
is_valid_for_streak Boolean required
is_active          Boolean  required
is_lifeline        Boolean  (written by applyLifeline; app-backend schema declares it, default false)
log_source         String   enum ['bah','habit_tracker'] default 'bah'   (app-backend schema)
```
Index (app-backend): `{user_id:1, is_active:1, check_ins_for_date:-1}`.
`product_prescriptions[]` element: `{ product_id, name, type, Dosage, dosageCode, info, description, composition, price, itemCount, image_url:{productUrl,cartImgUrl,mobileImgUrl,singleHalfImages}, cartDisplayName, newlyAdded, morningCheckIns, eveningCheckIns, bothCheckInsRequired }`.

### `streak_logs`
```
user_id String required; streak_achieve_days Number required; longest_streak_days Number required;
first_date_of_log Date required; last_date_of_log Date required; is_active Boolean;
log_source String enum ['bah','habit_tracker'] default 'bah'
```

### `streak_masters`
```
display_name String required; days Number required; slug String required (unique index in app-backend);
is_active Boolean required; is_for_superadmin Boolean required; reward_coins Number required
```
Slugs: `'first-checkin-extra-reward'`, `'existing_coins_streak'`, `'reorder_800_coin_experiment'`, v85 `habit-daily-week-N`, `habit-ladder-N`; lookups by `days: 3|7|21`.

### `reward_transactions`
```
user_id String required; streak_master_id ObjectId ref streak_masters required;
credit_coins Number; is_credit_transaction Boolean;
total_debit_coins Number (validator <= credit_coins); is_debit_transaction Boolean;
debit_transactions [ {_id:false, debit_transaction_id ObjectId ref redeem_reward_transactions, debit_coins Number,
   transaction_status String enum[success,failure] default success, debit_remarks String,
   debit_transaction_date Date, debit_transaction_updated_date Date} ];
credit_remarks String; all_coins_used Boolean; status String enum[success,failure] default success; expire_at Date
```
No indexes declared anywhere. **Go adds:** `{user_id:1, status:1, expire_at:1, all_coins_used:1}` and a partial unique `{user_id:1, credit_remarks:1}` where `is_credit_transaction:true` and `credit_remarks` matches the idempotency prefixes (see spec).

### `redeem_reward_transactions`
```
user_id String required; redeemed_coins Number; redeemed_amount Number; order_id String; order_display_id String;
shop_flo_txn_id String; currency String; status String enum[success,failure] default success; remarks String; meta Object
```

### `user_bah_archived_products`
```
user_id String required; archived_product_ids [String] required default []; updated_at Date
index {user_id:1} UNIQUE
```

### `habit_tracker_lifeline_logs`
```
user_id String required; date_covered Date required; kit_start Date required; applied_at Date required;
source String required default 'auto'; streak_day_at_apply Number
index {user_id:1, date_covered:1} UNIQUE  ← double-spend guard (err 11000 ⇒ created:false)
```

### `customeractivitylogs` (model `CustomerActivityLog`)
```
case_id String required; action_date Date required; event String required; order_id String default null;
reminder_days [Number] default []
```
Events used: `BAH_FEATURE_UPDATE`, `BAH_MISSED_LOG_FOR_YESTERDAY`; `reminder_days` in route 8.

### `task_master`, `user_task_details` — task marking on log (§6.6).

## 2.2 Postgres (Traya, Bookshelf/Knex, `DATABASE_*` env)

| Table | Columns used |
|---|---|
| `orders` | `id, user_id, case_id, status, created_at, delivery_date, order_meta (jsonb: line_items[{variant_id, product_id, quantity, name/title}]), is_bulk_order, bulk_order_duration, order_display_id, order_phone_number`. Non-void list: `status != 'void'` ORDER BY `created_at DESC`. Calendar first delivered: `status='delivered'` ORDER BY `delivery_date ASC` LIMIT 1. How-to-use: `status NOT IN ('void','unknown','ghost')`. |
| `users` | `id, phone_number, gender, email, first_name, last_name` |
| `cases` | `id, user_id` |
| `product_sku_mapping` | `product_principal_id, product_price, image_cdn_path, cart_cdn_images, mobile_image_path, single_half_images, single_images, medicine_id` |
| `medicine_master` | `id, display_name, type, description, detailed_display_name, dosage, dosage_code, info, composition` |
| `user_order_reminders` | `user_id, is_finished, tag, actual_date` |
| `form_session`, `feedback_form`, `feedback_forms_response` | 15-day check-in feedback URL (§3.1) |

---

# 3. BUSINESS LOGIC (pseudo-code)

## 3.1 `getStreakAndRewardBalance(userId, version, authToken='')` — handler.js:1381

### Six parallel queries
```
1. RewardTransactions.aggregate([
     {$match:{user_id, status:'success', expire_at:{$gt: now}, all_coins_used:false}},
     {$group:{_id:null, totalCredits:{$sum:'$credit_coins'}, totalDebits:{$sum:'$total_debit_coins'}}},
     {$project:{_id:0, balanceCoins:{$subtract:['$totalCredits','$totalDebits']}}}])
2. StreakLog.findOne({user_id, is_active:true})
3. RewardTransactions.find({user_id}, {credit_coins:1,total_debit_coins:1,expire_at:1,all_coins_used:1})
     .sort({createdAt:-1}).populate({path:'streak_master_id', select:'days'})
4. checkUserEligibleForNewBahFlow(userId)   // hardcoded {isUserEligibleForNewBahFlow:true}
5. getAllNonVoidOrdersByUserIdWithSelectedColumns(userId)
6. getUserAndCaseDetailsByUserId(userId)    // Case.where({user_id}).fetch({withRelated:['user']})
```
Bug preserved: step 3 counts `record.days` (undefined) so `threeDaysStreakCount/sevenDaysStreakCount/twentyOneDaysStreakCount` are always 0.

### O8+ cohort — `checkO8PlusEligibility(nonVoidOrderList)` (handler.js:673)
```
if len(orders) < 8 → false
caseId = orders[0].case_id ; if !caseId → false
if lower(caseId[0]) NOT IN ['2'..'9'] → false
goLive = startOfDay('2026-04-10'); end = goLive + 15 days endOfDay
latest = first order with a delivery_date (list is created_at DESC)
return startOfDay(latest.delivery_date) in [goLive, end]
```

### Streak-restart cohorts (handler.js:695-756)
```
isMaleStreakRestartCohort(caseId,gender):   gender=='M' && lower(caseId[0]) ∈ ['0','1','a'..'f']
isFemaleStreakRestartCohort(caseId,gender,orderCount): gender=='F' && orderCount ∈ [1] && lower(caseId[0]) ∈ ['0','1','a'..'f']
checkStreakRestartBonusEligibility       = STREAK_RESTART_BONUS_ENABLED(false) && male cohort
checkStreakRestartBonusEligibilityFemale = STREAK_RESTART_BONUS_FEMALE_ENABLED(true) && female cohort
resolveStreakRestartBonusAmount(userId,caseId,gender):   // used at log time only
  if male-eligible → 100
  if gender=='F' && female flag → fetch non-void orders; if female-eligible → 100
  else null
creditStreakRestartBonus: StreakMaster.findOne({is_active:true, slug:'existing_coins_streak'})
   → saveRewardTransaction({userId, streakMasterRef, phoneNumber, reason:'Streak restart bonus', customAmount})
```
In this read handler only the **male** predicate is used (effectively false).

### Core computation
```
rewardBalance = round(rewardBalanceRef[0].balanceCoins ?? 0)
currentDaysStreakCount / longestDaysStreakCount / lastLogDate / firstLogDate from streakLogRef
isBahLocked   = !isUserEligibleForNewBahFlow   // always false
hasLoggedEver = !!firstLogDate || rewardTxnRecords.length > 0
loggedToday   = isLoggedToday(lastLogDate)    // local Y/M/D equality vs new Date()
bahBanner:
   coins = streak<3 ? (O8+?200:100) : streak<7 ? (O8+?600:400) : (O8+?2500:2000)
   { title:'Log And Earn', subTitle:'Take a step closer to healthy hair', ctaText:'Log Your Routine', coins, unlockText:'Unlocking Soon!' }
```

### bannerWidgetData — only when `Number(version) >= 71` (else `null`). `getBannerWidgetData` handler.js:4169
```
CDN = process.env.S3_IMAGE_BASE_URL
daysDifference = startOfDay(today).diff(startOfDay(lastLogDate),'days')
effectiveStreakCount = daysDifference > 2 ? 0 : currentDaysStreakCount
getStreakDays() = (effectiveStreakCount==21 && daysDifference==1) ? formatDays(0) : formatDays(effectiveStreakCount)
formatDays(n) = `${n} Day` + (n==1?'':'s')      formatCoins(n) = `${n} Coin` + (n==1?'':'s')

A) !hasLoggedEver && !isBahLocked:
   title 'Log everyday to earn up to 20% off on your next kit.'
   subTitle 'Discounts worth ₹54L won last month!'
   subTitleIcon `${CDN}App/bah/new_bah/Coins.svg`
   ctaLabel 'Log & Earn' ; showBlueBar true
B) hasLoggedEver && !isBahLocked && !loggedToday:
   B1) effectiveStreakCount==0:
       if isEligibleForStreakRestartBonus: title 'Get bonus 100 coins to restart your streak!' subTitle 'Get back on track today'
       else: title 'Your streak broke. Log today to restart.'
   B2) else: title 'Log for today. Keep your streak going!'
   both: coinBalance=formatCoins(rewardBalance), streakDays=getStreakDays(), ctaLabel 'Log Now', showStreakTimeline true
C) hasLoggedEver && !isBahLocked && loggedToday:
   title 'Log done for today!' ; coinBalance ; streakDays ; ctaLabel 'View Log' ; showStreakTimeline true
D) isBahLocked && hasLoggedEver:
   title 'Log & Earn is locked.' ; subTitle 'Order next kit to unlock it.' ; ctaLabel 'View Log' ; coinBalance ; icon `${CDN}App/bah/new_bah/Lock.svg`
E) isBahLocked && !hasLoggedEver:
   title 'Log & Earn is locked.' ; subTitle 'Order next kit to start earning coins.' ; ctaLabel 'Know More' ; icon `${CDN}App/bah/new_bah/coin.svg`
```

### Streak-broken recompute (AFTER bannerWidgetData is built)
```
if Number(version) >= 71 && lastLogDate:
   daysDifference = startOfDay(today).diff(startOfDay(lastLogDate),'days')
   streakBroken = daysDifference > 2 || (currentDaysStreakCount == 21 && daysDifference == 1)
   if streakBroken: currentDaysStreakCount = 0
```

### Milestone / coins
```
isEligibleForNewModal = true ; streakRewardMessage = ''
streakRewards = O8+ ? {3:200,7:600,21:2500} : {3:100,7:400,21:2000}
currentMilestone = streak>=21?21 : streak>=7?7 : streak>=3?3 : null
currentCoins = currentMilestone ? streakRewards[currentMilestone] : (O8+ ? 200 : 100)
```

### Community share CTA at 7 / 21
```
if currentMilestone ∈ [7,21] AND authToken:
  url = `${COMMUNITY_BASE_URL}landing/${authToken}?share=true&streak_count=${currentMilestone}&reward_coins=${currentCoins}&cross=no&preventBack=true`
  communityShareButton = { label:'Share on Community', action:'web_page', variant:'primary', url }
```

### 15-day check-in nudge
```
scopeOk = !!authToken && gender=='F' && nonVoidOrderList.length==1 && longestDaysStreakCount>=3
runningWeek = scopeOk ? getAllDetailsRelatedToNonVoidOrders(nonVoidOrderList).runningWeekinMonthForHairKit : null
if scopeOk && runningWeek == 3:
   rows = user_order_reminders where user_id, is_finished=true AND (tag IN ['Week #1','Week #3'] OR actual_date >= now-3d), columns [tag, actual_date]
   w1NotCompleted = no row tag=='Week #1'; w3NotCompleted = no row tag=='Week #3'; noRecentCall = no row with actual_date >= now-3d
   isFifteenDayCheckinEligible = w1 && w3 && noRecent
   if eligible: feedbackUrl = buildFifteenDayCheckinFeedbackUrl({caseId,userId,gender,authToken})
       → `${FEEDBACK_UI_DOMAIN}/form?session_id=<sid>&source=app&authToken=<t>&syntheticId=<sid2>`
         FEEDBACK_UI_DOMAIN = prod ? 'https://feedback.traya.health' : 'https://feedback-ui.dev.hav-g.in'
         form_id 'component-1782457562767-VdwcN0', form_name '15_Day_Checkin'; null if an existing form_session for the case is stage=='completed'
       feedbackShareButton = { label:'Share treatment feedback', action:'web_page', variant:'primary', url }
```

### Streak-broken-before-log detection
```
isStreakBrokenBeforeLog = (currentDaysStreakCount==0 && hasLoggedEver)
  || (currentDaysStreakCount==1 && loggedToday && longestDaysStreakCount>1 && firstLogDate && lastLogDate && sameDay(firstLogDate,lastLogDate))
streakAfterLogging = loggedToday ? currentDaysStreakCount : currentDaysStreakCount + 1
showStreakRestartBonus = isEligibleForStreakRestartBonus && isStreakBrokenBeforeLog
postLoggingContent          = getPostLoggingModalContent({currentStreak:streakAfterLogging, hasLoggedEver, isMissedYesterday:false, isStreakRestartBonus:showStreakRestartBonus})
postLoggingContentYesterday = same but isMissedYesterday:true
```

### `modals` (exact payload)
```
logDoneModal = { type:'logDone', showCoinsAndStreaksInfo:true,
  mediaUrl:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/Tick.json',
  title: postLoggingContent.title, titleYesterday: postLoggingContentYesterday.title,
  description: postLoggingContent.description, descriptionYesterday: postLoggingContentYesterday.description,
  buttons:[{label:postLoggingContent.cta, action:postLoggingContent.ctaAction, variant:'primary'}],
  buttonsYesterday:[{label:...Yesterday.cta, action:...Yesterday.ctaAction, variant:'primary'}] }
rewardsModal = { type:'streakModal', showCoinsAndStreaksInfo:true,
  confettiAnimation:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/confetti.json',
  mediaUrl:'local:YellowCoin.json', rewardAmount:`${currentCoins}`,
  title: postLoggingContent.title, titleYesterday: postLoggingContentYesterday.title,
  description:{
     firstLog: isStreakBrokenBeforeLog ? postLoggingContent.title : 'You won rewards for doing your first log',
     streakComplete: isFifteenDayCheckinEligible
        ? 'You are staying consistent. Let your coach know if you are facing any issues with treatment'
        : `You won rewards for completing\n${currentMilestone||3}-Day streak.` },
  buttons:[ feedbackShareButton || {label:postLoggingContent.cta, action:postLoggingContent.ctaAction, variant:'primary'}, ...(communityShareButton?[communityShareButton]:[]) ],
  buttonsYesterday:[ {label:...Yesterday.cta, action:...Yesterday.ctaAction, variant:'primary'}, ...(communityShareButton?[communityShareButton]:[]) ] }
streakRestartBonusModal = (isEligibleForStreakRestartBonus && currentDaysStreakCount==0 && !loggedToday) ? {
  type:'streakRestartBonus', showCoinsAndStreaksInfo:true, title:'Bonus 100 coins credited!',
  buttons:[{label:'Okay',action:'close',variant:'primary'}] } : null
errorModal = { type:'logFailed', showCoinsAndStreaksInfo:true,
  mediaUrl:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/Error.png',
  title:'Log Failed!', description:'Something went wrong.', retryText:'Please retry.',
  buttons:[{label:'Try Again',action:'close',variant:'primary'}] }
streakBrokeModal = { type:'streakBroke', showCoinsAndStreaksInfo:true,
  mediaUrl:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/streakbroke.png',
  title:'Your streak broke ☹️', description:'Log for 3 days to earn 100 coins.',
  descriptionSecondary:"If you miss for 2 days straight, logging resets. Don't worry, total coins earned stay with you.",
  buttons:[{label:'Log for Yesterday',action:'setDateToYesterday',variant:'secondary'},{label:'Log for Today',action:'setDateToToday',variant:'primary'}] }
featureUpdateModal = { type:'featureUpdate', showCoinsAndStreaksInfo:false,
  mediaUrl:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/rocket.json',
  title:'New features.\nJust for you.', description:'Designed to make things easier,\nfaster, and better. Take a look!',
  buttons:[{label:'Okay',action:'close',variant:'primary'}] }
newOrderModal = { type:'orderDelivered', showCoinsAndStreaksInfo:false,
  mediaUrl:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/Kit_Container.png',
  title:'Congrats! Your order is delivered.', description:'You will see new products\nfrom your delivery date',
  buttons:[{label:'Okay',action:'close',variant:'primary'}] }
missedLogModal = { type:'logMissed', showCoinsAndStreaksInfo:false,
  mediaUrl:'https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/Missed.png',
  title:'Missed Logging Yesterday', description:'Last chance to log for yesterday.',
  buttons:[{label:'Did Not Use Kit Yesterday',action:'markMissed',variant:'danger'},{label:'Log for Yesterday',action:'setDateToYesterday',variant:'primary'}] }
```

### `hasUserSeenbahUpdatedModal(userId, hasLoggedEver, lastLogDate)` — handler.js:4006
```
todayStart/todayEnd = local startOf/endOf day
[latestOrder, orderCount] = Order where user_id status!='void' ORDER BY created_at DESC first (created_at, case_id, delivery_date); Order where user_id count()
hasUserSeenBahUpdatedModalResult = true
if latestOrder && new Date(latestOrder.created_at) > new Date('2025-05-20'):
    hasUserSeenBahUpdatedModalResult = !!CustomerActivityLog.findOne({case_id, event:'BAH_FEATURE_UPDATE'})
dayDiffFromLastLogDate = startOfDay(today).diff(startOfDay(lastLogDate),'days')
missedLog = caseId ? CustomerActivityLog.findOne({case_id, event:'BAH_MISSED_LOG_FOR_YESTERDAY', createdAt:{$gte:todayStart,$lt:todayEnd}}) : {}
showBAHLogMissedToUser = hasLoggedEver && dayDiffFromLastLogDate == 2 && isEmpty(missedLog)
```

### Response shape
```json
{ "rewardBalance", "currentDaysStreakCount", "longestDaysStreakCount",
  "threeDaysStreakCount", "sevenDaysStreakCount", "twentyOneDaysStreakCount",
  "lastLogDate", "firstLogDate", "isUserEligibleForNewBahFlow",
  "bahBanner", "bahTitle":"Log And Earn", "popupText",
  "coinDiscountCap":{"value":25,"type":"percentage"}, "coinConversionRatio":"0.1",
  "coinNotApplied":{"title":"Apply Traya Coins","subTitle":"Total coins: {TotalCoins}, Max usable: {ClaimableCoin}"},
  "coinApplied":{"title":"Apply Traya Coins","subTitle":"Total coins: {TotalCoins}, Max usable: {ClaimableCoin}"},
  "autoApplyCoins":true, "bahbannerNewTitle":"Log & Earn", "streakRewardMessage":"",
  "hasUserSeenBahUpdatedModalResult":true, "bannerWidgetData", "showBAHLogMissedToUser", "modals",
  "reminderOnBahPage":"top" }
```
`popupText`:
```
_1_day  : h1 'YOU WON 100 COINS! 🎊'  h2 'Redeem the coins before making final payment for next order. 10 coins = ₹1'
_3_days : h1 '3 DAY STREAK 💰'        h2 'You won 100 coins! Stay regular and win bigger rewards.'
_7_days : h1 '7 DAY STREAK 💎'        h2 'You won 400 coins! An even bigger reward awaits!'
_21_days: h1 '21 DAY STREAK 🔥'       h2 'You won 2000 coins! Your hair is grateful for your dedication.'
ctaText 'HOW TO REDEEM COINS?'  dismissText 'DISMISS'
```

## 3.2 `getBahLogForGivenDate(date, userId, showBahV3, caseId, appVersion=0)` — handler.js:2608
```
utcDate   = date ? new Date(date) : new Date()
clientDate= getClientTimeFromUtcTime(utcDate)            // +330 minutes
todayClientDate     = setTimeEndOfDay(getClientTimeFromUtcTime(now))
yesterdayClientDate = setTimeStartOfDay(todayClientDate - 1 day)
parallel:
  A = UserActivityLogsForBAH.findOne({user_id, check_ins_for_date:{$gte:setTimeStartOfDay(clientDate), $lte:setTimeEndOfDay(clientDate)}}).lean()
  B = getLatestMedicines(userId, showBahV3)   // {medicines, isMedicineLocked, bahV3ReorderText}
if (!A):
  medicineData = medicines.map({product_id,name,Dosage,dosageCode:dosageCode||'',description,image_url, morningCheckIns:false, eveningCheckIns:false, bothCheckInsRequired:false})
  orderEligibleForLog = !isMedicineLocked || isOrderEligibleForLog(caseId)
  medicineReordertext = isOrderEligibleForLog(caseId) ? {} : bahV3ReorderText
  if todayClientDate == setTimeEndOfDay(clientDate): isPostApiNeeded = true
  elif yesterdayClientDate == setTimeStartOfDay(clientDate): nextDayLog = findOne(today); if !nextDayLog: isPostApiNeeded = true
else:
  orderEligibleForLog = true
  medicineReordertext = isOrderEligibleForLog(caseId) ? {} : bahV3ReorderText
  medicineData = A.product_prescriptions || []
  if today: isPutApiNeeded = true ; elif yesterday and no today log: isPutApiNeeded = true
archivedProduct = []
if appVersion >= 85:
   ids = getArchivedProductIds(userId)
   if ids.size: archivedProduct = medicineData.filter(in ids); medicineData = medicineData.filter(not in ids)
```
`isOrderEligibleForLog(caseId)` (handler.js:4380): `moment().isBefore(moment('2026-04-20'))` only for `caseId == '02a883d7-8c25-43e0-975d-16ed335ce439'`; else false.

### Daily vs weekly dosage split (per `data.dosageCode`)
```
WEEKLY if dosageCode ∈ [ONCE_A_WEEK, TWICE_A_WEEK, THRICE_A_WEEK, TWICE_OR_THRICE_A_WEEK]
   subHeading default 'USE 2 TIMES IN A WEEK'; ONCE_A_WEEK 'MIN ONCE IN A WEEK'; THRICE_A_WEEK 'USE 3 TIMES IN A WEEK'; TWICE_OR_THRICE_A_WEEK 'USE 2 OR 3 TIMES IN A WEEK'
   dosageDisplayText = [{ text:'Log for the day', isMedicineLogged: morningCheckIns && eveningCheckIns, logTimeInDay:'ANYTIME_IN_DAY' }]
DAILY otherwise
   subHeading = 'ONCE DAILY'; if dosageCode ∈ ['2-0-2','1-0-1','1ml-0-1ml'] → 'TWICE DAILY'
   morningText/eveningText default '1ml (Morning)' / '1ml (Evening)'
     ∈ ['1-0-0','0-0-1','1-0-1'] → '1 Tablet (Morning)' / '1 Tablet (Evening)'
     == 'AS_DIRECTED'            → 'As directed (By Dr.)' both
     ∈ ['2-0-0','0-0-2','2-0-2'] → '2 Tablet (Morning)' / '2 Tablet (Evening)'
   maxDailyDosage = [{text:morningText, isMedicineLogged:morningCheckIns, logTimeInDay:'MORNING'},{text:eveningText, isMedicineLogged:eveningCheckIns, logTimeInDay:'EVENING'}]
   ∈ ['1-0-0','2-0-0','1ml-0-0'] → [morning] ; ∈ ['0-0-1','0-0-2','0-0-1ml'] → [evening] ; else both
```
`DOSAGE_CODE`: `ONCE_A_WEEK`,`TWICE_A_WEEK`,`THRICE_A_WEEK`,`TWICE_OR_THRICE_A_WEEK` (literal), `ONCE_IN_MORNING:'1-0-0'`, `TWO_TABLETS_ONCE_IN_MORNING:'2-0-0'`, `ONCE_IN_AFTERNOON:'0-1-0'`, `ONCE_IN_NIGHT:'0-0-1'`, `TWO_TABLETS_ONCE_IN_NIGHT:'0-0-2'`, `TWICE_A_DAY:'1-0-1'`, `TWO_TABLETS_TWICE_A_DAY:'2-0-2'`, `THRICE_A_DAY:'1-1-1'`, `MORNING_1ML:'1ml-0-0'`, `NIGHT_1ML:'0-0-1ml'`, `MORNING_1ML_AND_NIGHT_1ML:'1ml-0-1ml'`, `AS_DIRECTED_BY_DOCTOR:'AS_DIRECTED'`.

### Coin expiry text / streak state / challenge banner
```
parallel: expiringReward = getEarliestExpiringUnusedCoins(userId); streakLogRef = StreakLog.findOne({user_id,is_active:true}); todayLog = findOne(today IST, is_active:true)
if expiringReward && remaining_coins > 0: coinExpiryText = `${n} coin${n==1?'':'s'} expiring on ${moment(expiring_on).format('DD MMM YYYY')}`
currentStreak = streak_achieve_days||0 ; streakBroken = today.diff(lastLog,'days') > 2 || (currentStreak==21 && diff==1) → 0
isLoggedToday = !!todayLog ; isMissedYesterday = lastLogDate && !isLoggedToday && diff == 2
bahChallengeEntryPointConfig = getBahChallengeEntryPointBanner({currentStreak,hasLoggedEver,isLoggedToday,isMissedYesterday})
```
`getEarliestExpiringUnusedCoins` (handler.js:3975): `findOne({user_id, is_credit_transaction:true, status:'success', expire_at:{$gte:startOfDay(today)}, $expr:{$gt:['$credit_coins','$total_debit_coins']}}).sort({expire_at:1})` → `{...reward, remaining_coins: credit_coins-total_debit_coins, expiring_on: format('YYYY-MM-DD')}`.

### Response
```json
{ "dailyDosageTitle":"Daily Dosage", "dailyDosage":[...], "weeklyDosageTitle": weeklyDosage.length>0 ? "Weekly Dosage" : "",
  "weeklyDosage":[...], "isPostApiNeeded", "isPutApiNeeded", "orderEligibleForLog", "medicineReordertext", "coinExpiryText",
  "bahChallengeEntryPointConfig",
  "bahVideo":{ "female":"https://cdn.shopify.com/videos/c/o/v/639cdb2a63a245c2a46de9aa67bef697.mp4", "male":"https://cdn.shopify.com/videos/c/o/v/856c2a9f4dfb433b8dd99d3738621fc7.mp4" },
  "archivedProduct":[...] }
```

## 3.3 `getBahChallengeEntryPointBanner` (handler.js:3598)
`bgImg` is always `${S3_IMAGE_BASE_URL}App/Home/WeeklyChallengeBannerIMG.png`. `dayText(n) = n==1?'day':'days'`. Tier table: `streak<3 → target 3/reward 100, next 7/400`; `3..6 → target 7/400, next 21/2000`; `7..20 → target 21/2000, next null`.

| Condition (in order) | title | subtitle | cta |
|---|---|---|---|
| `isMissedYesterday && !isLoggedToday` | `You missed yesterday's log.` | `Restore your streak by logging for yesterday.` | `Log now` |
| `!hasLoggedEver && !isLoggedToday` | `Log now and claim your 100 coins` | `Start your 3-day streak today` | `Log now` |
| `!hasLoggedEver && isLoggedToday` | `Log for 2 more days to earn 100 more coins.` | `Continue your 3-day streak` | `View Log` |
| `streak >= 21` | `Congratulations! You've completed the 21-day streak!` | `Keep logging to maintain your healthy habit.` | `isLoggedToday?'View Log':'Log now'` |
| PRE-LOG `streak==0` | `Start logging today and build streaks.` | `3 days to unlock 100 coins` | `Log now` |
| PRE-LOG `streak==2` | `Log now to earn 100 coins.` | `Complete your 3 day streak now.` | `Log now` |
| PRE-LOG `streak==1` | `${3-streak} ${dayText} left to unlock 100 coins.` | `Continue your 3 day streak` | `Log now` |
| PRE-LOG `streak==6` | `Log today to earn 400 coins.` | `Complete your 7 day streak today` | `Log now` |
| PRE-LOG `3<=streak<6` | `${7-streak} ${dayText} left to unlock 400 coins` | `Build your 7 day streak` | `Log now` |
| PRE-LOG `streak==20` | `Log today to earn 2000 coins.` | `Complete your 21 day streak` | `Log now` |
| PRE-LOG `7<=streak<20` | `${21-streak} ${dayText} left to unlock 2000 coins.` | `Complete your 21-day streak.` | `Log now` |
| POST-LOG `target-streak <= 0` and next tier | `Log tomorrow to start your ${nextTarget}-day streak.` | `${nextTarget-streak} ${dayText} to earn ${nextReward} coins.` | `View Log` |
| POST-LOG `target-streak <= 0`, no next tier | `Congratulations! You've completed the 21-day streak!` | `Keep logging to maintain your healthy habit.` | `View Log` |
| POST-LOG `7<=streak<21` | `Log for ${rem} more ${dayText} to earn 2000 coins.` | `Complete your 21-day streak` | `View Log` |
| POST-LOG `3<=streak<7` | `Log for ${rem} more ${dayText} to earn 400 coins.` | `${rem} ${dayText} to 7 day streak` | `View Log` |
| POST-LOG `streak<3` | `Log for ${rem} more ${dayText} to earn 100 coins.` | `` | `View Log` |

## 3.4 `getPostLoggingModalContent` (handler.js:3852)
`ctaAction` is `'close'` except the `isMissedYesterday` case.

| Condition (in order) | title | description | cta | ctaAction |
|---|---|---|---|---|
| `isMissedYesterday` | `Well done!` | `Log for today and continue your streak` | `Log now` | `setDateToToday` |
| `!hasLoggedEver` | `100 coins unlocked for your 1st log.` | `Log for 2 more days to build 3-day streak and earn 100 more coins` | `Okay` | `close` |
| `streak==1 && isStreakRestartBonus` | `Bonus 100 coins credited!` | `` | `Okay` | `close` |
| `streak==1` | `🔥 Great start!` | `2 more days to unlock 100 coins.\nCome tomorrow and log to continue your streak.` | `Keep Going` | `close` |
| `streak==2` | `🎉 2-Day Streak!` | `Next: Come tomorrow and log to complete your 3-day streak.` | `Continue` | `close` |
| `streak==3` | `🎉 3-Day Streak!` | `Next: Log for 4 more days to complete your 7-day streak.` | `Continue` | `close` |
| `streak 4..5` | `${s}-Day Streak!` | `Log for ${7-s} more ${dayText} to hit 7 day streak.` | `Keep Building` | `close` |
| `streak==6` | `6-Day Streak!` | `Come tomorrow and log to complete your 7-day streak.` | `Keep Building` | `close` |
| `streak==7` | `🏆 7-Day Streak!` | `Log for 14 more days to complete your 21-day streak.` | `Go for 21` | `close` |
| `streak 8..19` | `🚀 ${s} Days Strong!` | `Log for ${21-s} more ${dayText} to unlock 2000 coins.` | `Stay Consistent` | `close` |
| `streak==20` | `🚀 20 Days Strong!` | `Log for 1 more day to unlock 2000 coins.` | `Stay Consistent` | `close` |
| `streak>=21` (and streak==0 fallthrough) | `🎊 ${s}-Day Streak!` | `Congratulations! You've completed the 21-day streak!\nKeep logging to maintain your healthy habit.` | `Amazing!` | `close` |

## 3.5 `POST /activityLogForBAH` — `saveActivityLogsAndCreateStreakAndGiveRewards` (handler.js:104)
```
1. validateLogActivityObject({isLogForToday, productPrescriptions})     // §3.19
2. archivedProductIds = getArchivedProductIds(userId); if non-empty filter productPrescriptions
3. checkInDate = getClientTimeFromUtcTime(new Date())          // now + 330 min
   if !isLogForToday:
      checkInDate -= 1 day
      todayLog = findOne({user_id, check_ins_for_date:{$gte:setTimeZeroForDate(checkInDate), $lt:setTimeZeroForDate(getClientTimeFromUtcTime(now))}})
      if todayLog: return { message: `User cannot log for date ${checkInDate}` }   // 200, no scratchCard key
4. userActivityLogRef = saveActivityLogs(userId, checkInDate, productPrescriptions)
5. if userActivityLogRef:
     if isHabitTracker: rewardOutcome = await processStreakAndRewards(...) (SYNC); scratchCard = rewardOutcome?.scratchCard ?? null
     else: emit 'create_streak_and_update_task' (fire-and-forget → processStreakAndRewards)
6. return { message: `User checked in successfully for date ${checkInDate}`, scratchCard }
```
`checkInDate` in the messages is a JS Date `toString()` — e.g. `Thu Sep 18 2026 18:30:00 GMT+0000 (Coordinated Universal Time)`.

`saveActivityLogs(userId, checkInDate, productPrescriptions)` (handler.js:252):
```
existing = findOne({user_id, check_ins_for_date:{$gt:setTimeZeroForDate(checkInDate), $lt:setTimeZeroForDate(checkInDate+1d)}})
if existing → return null   (silently)
if checkInDate.getDate() < new Date().getDate():        // Go: compare IST dates
    todayLog = findOne({user_id, check_ins_for_date:{$gt:setTimeZeroForDate(clientNow), $lt:clientNow}})
    if todayLog → return null
if !Array.isArray(pp) && pp.length<1 → reject 400 'Product prescription cannot be a blank array'
create({user_id, check_ins_for_date:checkInDate, product_prescriptions, is_valid_for_streak:true, is_active:true})
try cache.del(`kit-tracker-calendar!${userId}`)   // non-fatal
```

`processStreakAndRewards({userId, checkInDate, userActivityLogRef, isHabitTracker})` (handler.js:174):
```
parallel: createStreakLogForUser(userId, checkInDate, is_valid_for_streak); getUserAndCaseDetailsByUserId(userId)
{streakRef, isStreakBreaked, alreadyLogged} = streakResult
if alreadyLogged → return {scratchCard:null}
gender = user.gender.toUpperCase() ?? 'M'; caseId = case.id; phone = user.phone_number
kitStartedTaskName = gender=='F' ? 'female_kit_started' : 'male_kit_started'
streakRestartBonusAmount = isStreakBreaked ? resolveStreakRestartBonusAmount(userId,caseId,gender) : null
parallel:
  rewardResult = creditRewardCoinsToUser(userId, streakRef.streak_achieve_days, phone, isHabitTracker, checkInDate)
  streakRestartBonusAmount ? creditStreakRestartBonus(userId, phone, amount) : noop
  updateTaskForUserToDisplay({userId, caseId, userTaskName:'build_a_habbit_sticky', taskCompletionResponse:'CUSTOMER'})
  updateTaskForUserToDisplay({... 'build_a_habit' ...})
  updateTaskForUserToDisplay({... kitStartedTaskName ...})
if is_valid_for_streak: try enqueueLifelineForLog(userId, checkInDate)  // non-fatal
return { scratchCard: rewardResult?.card ?? null }
on error: log + postToSlack; return {scratchCard:null}
```

`createStreakLogForUser(userId, checkInDate, isValidForStreak)` (handler.js:325):
```
if !isValidForStreak → reject 400 'Not eligible to create streak'
s = StreakLog.findOne({user_id, is_active:true})
if s:
   daysDiff = calculateDaysDifference(s.last_date_of_log, checkInDate)     // ABSOLUTE, UTC-midnight granular
   if daysDiff == 0 → return {streakRef:s, isStreakBreaked:false, alreadyLogged:true}
   isStreakBreaked = (daysDiff > 1) || (s.streak_achieve_days == 21)
   data = { streak_achieve_days: s.streak_achieve_days+1, last_date_of_log: checkInDate }
   if isStreakBreaked: data.first_date_of_log = checkInDate; data.streak_achieve_days = 1
   new = findOneAndUpdate({user_id}, {$set:data}, {new:true})            // Go: add is_active:true to filter
else:
   new = create({user_id, streak_achieve_days:1, longest_streak_days:1, first_date_of_log:checkInDate, last_date_of_log:checkInDate, is_active:true})
if new.longest_streak_days < new.streak_achieve_days: update longest_streak_days
emit CCD_UPDATE { eventType:'ACTIVITY_LOG', caseId, payload:{ logDate:checkInDate, streakDate:new.last_date_of_log, streakCount:new.streak_achieve_days } }
return {streakRef:new, isStreakBreaked}
```

### 3.5a `creditRewardCoinsToUser(userId, continuousCheckInDays, phoneNumber, isHabitTracker, checkInDate)` — handler.js:761 (direct read)
```
if isHabitTracker → return mintHabitTrackerScratchCardViaAppBackend(userId, continuousCheckInDays, phoneNumber, checkInDate)
     // Go: call habit.MintOrCredit in-process; result has `.card` on ladder days
firstTimeMaster = StreakMaster.findOne({is_active:true, slug:'first-checkin-extra-reward'})
existingMaster  = StreakMaster.findOne({is_active:true, slug:'existing_coins_streak'})
txn = RewardTransactions.findOne({user_id, $or:[{streak_master_id:firstTimeMaster._id},{streak_master_id:existingMaster._id}]})
if !txn: saveRewardTransaction({userId, streakMasterRef:firstTimeMaster, phoneNumber})     // first-ever log bonus (100)
if continuousCheckInDays ∈ [3,7,21]:
   master = StreakMaster.findOne({is_active:true, days: continuousCheckInDays})
   isEligibleForO8PlusRewards = false        // hardcoded; O8+ is display-only
   customAmount = null
   saveRewardTransaction({userId, streakMasterRef:master, phoneNumber, customAmount})
```
**Go adds idempotency**: `reason` (→ `credit_remarks`) becomes `bah-legacy-<slug>-<IST YYYY-MM-DD of checkInDate>` for milestone credits and `bah-legacy-first-log` for the first-log bonus, with a partial unique index so replays produce exactly one credit. Display of `credit_remarks` in history stays whatever is stored.

## 3.6a `PUT /activityLogForBAH` — `updateStreakLogForUser` (handler.js:416, direct read)
```
validate {isLogForToday bool req, productId string req, morningCheckIns bool req, eveningCheckIns bool req}; error → THROWN raw (500 {err})
checkInDate = clientNow
if !isLogForToday:
   todayLog = findOne({user_id, check_ins_for_date:{$gt:setTimeZeroForDate(clientNow), $lt:clientNow}})
   if todayLog → 400 'User has today logs. User cannot update logs for yesterday'
   checkInDate -= 1 day
log = findOne({user_id, check_ins_for_date:{$gt:setTimeZeroForDate(checkInDate), $lt:checkInDate}})
if !log → 400 'Invalid checkin request'
pp = log.product_prescriptions || []; if len<1 → 400 'Product prescription cannot be blank array. Please check record saved successfully or not.'
target = pp.filter(product_id === productId); if none → 400 'Invalid product id'
if target.morningCheckIns && target.eveningCheckIns → 400 'Already checkedIn for this product.'
updateOne({_id, 'product_prescriptions.product_id':productId}, {$set:{'product_prescriptions.$.morningCheckIns':morningCheckIns, '...eveningCheckIns':eveningCheckIns}})
return { message:'Checkin updated successfully for your product' }
```
(Route is deleted in Wave 0 — NOT implemented in Go; kept here for reference.)

## 3.7 `PUT /multipleActivityLogForBAH` — `updateMultipleMedicineLogForUser` (handler.js ~470)
```
if !logProductDetail || len==0 → return {message:'Medicine logged successfully for your products'}   // 200 no-op
validateMultipleActivityLogRequest → 400 on bad shape
checkInDate = clientNow
if !isLogForToday:
   todayLog = findOne({user_id, check_ins_for_date:{$gt:setTimeZeroForDate(checkInDate), $lt:checkInDate}})
   if todayLog → return {message:'Medicine logged successfully for your products'}   // SILENT no-op (200)
   checkInDate -= 1 day
log = findOne({user_id, check_ins_for_date:{$gt:setTimeZeroForDate(checkInDate), $lt:setTimeZeroForDate(checkInDate+1d)}})
if !log → return {message:'Medicine logged successfully for your products'}       // SILENT no-op
if pp.length < 1 → reject 400 'Product prescription cannot be blank array. ...'
targets = pp.filter(p => Number(p.product_id) in productIds && (!p.morningCheckIns || !p.eveningCheckIns))
bulkWrite([ updateOne({_id, 'product_prescriptions.product_id':p.product_id},
   {$set:{'product_prescriptions.$.morningCheckIns': map[p.product_id]?.morningCheckIns || false,
          'product_prescriptions.$.eveningCheckIns': map[p.product_id]?.eveningCheckIns || false}}) ])
return {message:'Medicine logged successfully for your products'}
```
`logProductDetail[].productId` is Joi `number().required()`; compared via `Number(...)`. Neither PUT touches streaks, coins, Redis, lifelines or CCD.

## 3.8 `GET /coinTransaction` — `getRewardCoinHistoryPaginated(userId, page=1, limit=6)` (handler.js:4080)
```
rows = []
RedeemRewardTransactions.find({user_id}) → each:
   {txnCoinAmount:redeemed_coins, txnType:'Debit', txnStatus:status||'', remarks:remarks||'', expiryDate:'', txnDate:createdAt,
    showCoins: status!=='failure', text: status==='failure' ? `${amt} coins redeemed on this order added back to wallet` : '', streakName:''}
RewardTransactions.find({user_id}).populate('streak_master_id','display_name') → each:
   push {txnCoinAmount:credit_coins, txnType:'Credit', txnStatus:status||'', remarks:credit_remarks||'', expiryDate:expire_at||'', txnDate:createdAt,
         showCoins:true, text:'', streakName: streak_master_id.display_name || ''}
   isExpired = expire_at && new Date(expire_at) < now
   if isExpired && !all_coins_used && status!=='failure':
      push {txnCoinAmount: credit_coins-(total_debit_coins||0), txnType:'Expired', txnStatus:'expired', remarks:'Coins Expired',
            expiryDate:expire_at, txnDate:expire_at, showCoins:true, text:'', streakName: display_name || ''}
sort by txnDate DESC; total; totalPages=ceil(total/limit); slice((page-1)*limit, page*limit)
return { rewardHistory: paginated, pagination:{page, limit, total, totalPages, hasNextPage: page<totalPages, hasPrevPage: page>1} }
```
Go: implement as a `$unionWith` aggregation with `$lookup` on streak_masters, `$sort`, `$facet` for count+page. Dangling lookups → `streakName: ''`.

## 3.9 `GET /bah/:userId/calendar` — `getBahCalendarLogData({userId, date, mode})` (handler.js:3401)
```
inputDate = moment(date).utcOffset('+05:30').startOf('day') ; invalid → 400 'Invalid date format.'
firstOrder = orders where {user_id, status:'delivered'} ORDER BY delivery_date ASC LIMIT 1 (delivery_date)
deliveryDate = firstOrder ? IST startOfDay(delivery_date) : IST startOfDay(now)
mode=='calendar':
   startDate = max(inputDate - 60 days, deliveryDate).startOf('day')
   lastVisibleDay = inputDate.endOf('month').startOf('day'); endDate = max(inputDate, lastVisibleDay)
mode=='streak':
   streak = StreakLog.findOne({user_id,is_active:true}).sort({updatedAt:-1}) (first_date_of_log last_date_of_log)
   if !streak → 'No active streak found for this user.'
   currentStreakLength = lastLog.diff(firstLog,'days') + 1 ; gap = today.diff(lastLog,'days')
   if currentStreakLength >= 21 : start=today, end=today+20d
   elif gap > 2                 : start=today, end=today+20d
   else                         : start=firstLog, end=firstLog+20d (endOf day)
else → 'Invalid mode. Use "calendar" or "streak".'
parallel:
   activityLogs = find({user_id, check_ins_for_date:{$gte:start,$lte:end}, is_active:true}) (check_ins_for_date is_valid_for_streak)
   rewardLogs   = RewardTransactions.find({user_id, createdAt:{$gte:start,$lte:end}, status:'success'}) (createdAt)
seed each day d in [start..end]:
   calendar: d < deliveryDate → 'inactive' ; d > inputDate → 'inactive' ; else 'missed'
   streak:   'missed'
for log in activityLogs: if is_valid_for_streak && key present → 'logged'
for r in rewardLogs:     if rawData[day(r.createdAt)]=='logged' → 'coin'
if rawData[today]=='missed' → 'active'
isTodayLoggedOrCoin = rawData[today] ∈ ['logged','coin']
if rawData[yesterday]=='missed' && !isTodayLoggedOrCoin → 'active'
if mode=='streak' && lastLogDate: any 'missed' day after lastLogDate → 'inactive'
monthKey = IST format('MMMM YYYY')
return { startDate: start.format('YYYY-MM-DD'), endDate: TODAY.format('YYYY-MM-DD')  /* bug preserved */,
         data: [ { month:"June 2025", monthData:[{date:'YYYY-MM-DD', status}, ...] }, ... ] }
```
Errors: missing `date` → `400 {error:'...'}`; other → `500 {error:'Internal Server Error', details}`.
Day keys `today`/`yesterday` are IST `YYYY-MM-DD`. Go binds `userId` to the token and ignores the path segment.

## 3.10 `GET /rewardBalance/:caseId`, `getUserRewardBalance`, `getOnlyRewardBalance`
```
getUserRewardBalance(caseId, userId=null): effectiveUserId = userId ?? getUserIdFromCaseId(caseId) (400 on bad/unknown uuid); return getOnlyRewardBalance
getOnlyRewardBalance(userId): same aggregation as §3.1 query 1; return Math.round(balanceCoins) or 0
```
Route responds `200 { rewardBalance: <number> }`; errors via `sendErrorHttpResponse` → `{message}`.

## 3.11 `getLatestMedicines(userId, showBahV3=false)` (handler.js:818) — 45-day window
```
nonVoidOrderList = orders (status!=void, created_at DESC, cols id/status/created_at/delivery_date/order_meta/case_id)
allDeliveredOrder = filter(status=='delivered')
IF allDeliveredOrder empty:
   if any non-void order:
      logDaysLeft=0; text1='Your Order is on its way'; text2='You can start logging in your routine after your kit is delivered.'
      isReorderRequired=false; showText=true
      if showBahV3: medicines = prescriptionsFor([nonVoidOrderList[last]]); isMedicineLocked=true
ELSE:
   {kitExpireDays, minDaysAfterOrderDelivered} = getAllDetailsRelatedToNonVoidOrders(allDeliveredOrder)
   if kitExpireDays + 15 >= minDaysAfterOrderDelivered: logDaysLeft = kitExpireDays - minDaysAfterOrderDelivered + 15
   for each delivered order:
      {diffDays, kitCount} = getKitDetail(order); kitCountForBAH = kitCount==0 ? 1 : kitCount
      if logDaysLeft >= 0 && (kitCountForBAH*30)+15 >= diffDays:
          push into ordersForLast45Days
          if kitExpireDays - minDaysAfterOrderDelivered <= 9:
             text1 = `${minDaysAfterOrderDelivered} Days Since Your Last Order - Act Fast! 🚀`
             text2 = `Log in access expires in ${logDaysLeft} days. Reorder to keep tracking your routine.`
             isReorderRequired=true; showText=true
             if showBahV3: bahV3ReorderText = { h1:'Your next kit order is due.', h2:'Redeem your coins and get discount upto 20% on your next order.', isBahLocked:false }
   isLatestStatusDelivered = nonVoidOrderList[0].status == 'delivered'
   if ordersForLast45Days empty:
      if !isLatestStatusDelivered:
         text1='Your Order is on its way'; text2='You can start logging in your routine after your kit is delivered.'; isReorderRequired=false; showText=true
         if showBahV3: medicines = prescriptionsFor([nonVoidOrderList[0]]); isMedicineLocked=true
      else:
         if showBahV3: bahV3ReorderText = { h1:'Log & Earn is locked.', h2:'Order now to use this feature and keep earning coins.', isBahLocked:true }
                       medicines = prescriptionsFor([nonVoidOrderList[0]]); isMedicineLocked=true
         text1='Oops! Your log and earn is locked'; text2='Reorder now to unlock the feature and maintain your streak'
         isReorderRequired=true; showText=true; ctaText='Save My Streak!'
   elif !isLatestStatusDelivered:
      text1='Your Next Order is on its way'
      text2='You are currently logging in for your existing routine. You’ll be able to log in for your upcoming kit after it is delivered.'
      isReorderRequired=false; showText=true; if showBahV3: bahV3ReorderText = {}
   if ordersForLast45Days non-empty: medicines = prescriptionsFor(ordersForLast45Days)
return {medicines, logDaysLeft, text1, text2, isReorderRequired, showText, ctaText:'Order Next Kit'(default)} + if showBahV3: {isMedicineLocked, bahV3ReorderText}
```
`getPrescriptionForMedicinesByOrders(orders)` → `getMedicineIds` + `getProductDesc`:
```
getMedicineIds(orders):
  oldToActiveMap = getStaticContent('OLD_TO_ACTIVE_VARIANT_IDS_MAP')  // GET ${TR_CONFIG_SERVICE_BASE_URL}/static-content/data/<key>, x-tenant-id: traya, cached
  per order: variantIds = pluck(order.order_meta.line_items,'variant_id') mapped via map[id].associatedTo || VARIANT_ID_MAPPING[id] || id
  if i>0 && i==last: newlyAddedMedicines = difference(mapped(orders[0].line_items), variantIds)
  medicineIds = uniq(compact(concat all))
  if both 44396552913074 (Hair_vitamin) and 37547237310642 (Discontinued_vitamin) present → drop 37547237310642
getProductDesc(ids, newlyAddedMedicines):
  product_sku_mapping whereIn product_principal_id JOIN medicine_master
  → { product_id: product_principal_id, name: display_name, type, Dosage: dosage, dosageCode: dosage_code||'', info, description, composition,
      price: product_price, itemCount: 1,
      image_url:{ productUrl:`${S3}${image_cdn_path}`, cartImgUrl:`${S3}${cart_cdn_images}`, mobileImgUrl:`${S3}${mobile_image_path}`, singleHalfImages:`${S3}${single_half_images}` },
      cartDisplayName: detailed_display_name, newlyAdded: newlyAddedMedicines.includes(Number(product_principal_id)) }
  sortBy('newlyAdded').reverse()
```
`VARIANT_ID_MAPPING` lives in `server/utils/config.js` — copy it verbatim into Go.

## 3.12 `getMedicinesForHowToUsePurpose(userId)` (handler.js:947)
```
orders = status NOT IN (void, unknown, ghost), created_at DESC
if orders: isPrescriptionLocked = !orders.some(status ∈ [shipped, delivered]); latest fields from orders[0]
per order:
  userCanSeeMedicinesForDays = 50; daysDifference = calculateDaysDifference(order.created_at, now)
  if orders.length > 1:
     last=orders[0].status; secondLast=orders[1].status
     if last ∉ [delivered,shipped] && secondLast=='delivered' → howToUseText='Last Delivered'
     elif last=='delivered' && daysDiff(orders[0].created_at, now) > 50 → 'Expired'
     elif last ∉ [delivered,shipped] && daysDiff(orders[0].created_at, now) > 50 → 'Last Delivered'
  else: if daysDifference > 50 → 'Expired'
  if order.is_bulk_order: userCanSeeMedicinesForDays = order.bulk_order_duration*30 + 20
  if daysDifference < userCanSeeMedicinesForDays → include ; else include only if list still empty
medicinesPrescription = getPrescriptionForMedicinesByOrders(included)
return { medicinesPrescription, isPrescriptionLocked, latestOrderId, latestOrderDisplayId, latestOrderStatus, latestOrderDate, howToUseText, showNew:true }
```

### 3.12a `GET /latestOrderHowtoUseV2/:caseId` reminder merge
After the recommendation-service call returns `data`, api-server reads `CustomerActivityLog.findOne({case_id: caseId, reminder_days:{$exists:true,$ne:[]}}).sort({createdAt:-1})` and sets `data.reminderInfo = { reminder_days, order_id, action_date }` (null when none). Read `index.js` route 8 for the exact key names before porting.

## 3.14 `saveRewardTransaction({userId, streakMasterRef, phoneNumber, reason, customAmount})` (handler.js:1924)
```
expiryDurationInDays = slug == 'reorder_800_coin_experiment' ? 2 : GET ${ORDER_SERVICE_BASE_URL}/coin/expiry/month/<userId> (number of days)
todayClientDate = clientNow + 1 day
futureDate = setTimeZeroForDate(todayClientDate + expiryDurationInDays days)   // UTC midnight
RewardTransactions.create({ user_id, streak_master_id, credit_coins: customAmount || reward_coins, is_credit_transaction:true,
  is_debit_transaction:false, total_debit_coins:0, credit_remarks: reason || display_name, all_coins_used:false, status:'success', expire_at: futureDate })
emit CCD_UPDATE {eventType:'COIN_CREDITED', caseId, payload:{amount: credit_coins}}
try getShopfloWalletTransactions(phoneNumber)   // ensure wallet (auto-create on 404); failure → log only
try createShopfloCreditRewardTransaction(credit_coins, futureDate epoch ms, rewardRef._id, phoneNumber)   // failure → log only
return { rewardRef }
```

## 3.15 Helpers
```
calculateDaysDifference(d1,d2) = abs((utcMidnight(d2)-utcMidnight(d1)) / 86400000)
getClientTimeFromUtcTime(date) = date + 330 minutes
setTimeZeroForDate(date)       = setUTCHours(0,0,0,0)
setTimeStartOfDay / setTimeEndOfDay = process-local (UTC in prod) start/end of day
getIndianTime(date)            = date + 330 minutes
getBahRunningLogDay(userId): s = StreakLog.findOne({user_id,is_active:true})
   if s: diff(last_date_of_log, getIndianTime(now)) > 2 → 'regular_day' ; elif streak_achieve_days ∈ [2,6,20] → that number ; else 'regular_day'
   return { logRunningDay (0 when no streak), userHasUsedBAH: !!s }
getMasterStreakDataForExtraBonusStreak(): StreakMaster.find({is_active:true, is_for_superadmin:true})
   → [{streakMasterId:_id, slugName:slug, displayName:display_name, rewardCoins:reward_coins}]
```

## 3.16 CRM / admin handlers (KEEP set)
- **`GET /bahHistory/:caseId` → `getBahHistoryOfUser`** (handler.js:2242). Resolve `{user, user_id}` from caseId; parallel `getRewardCoinHistory(userId,isCreditTxn,isDebitTxn)`, `getStreakAndRewardBalance(userId, 70)`, `getActivityLogs(userId, year, month)`, `getLatestMedicines(userId)`; then Shopflo balance (`total_wallet_balance * 10`). Return spread-merge of all four plus `{latestMedicines, isBalanceSyncingRequired:false, shopFloCoins, isUserAuthorizedToGiveCoins: COIN_CREDIT_ACCESS_EMAIL_IDS.includes(req.email), shopFloError}`. Version 70 ⇒ `bannerWidgetData` null, no recompute.
- **`getRewardCoinHistory(userId, isCreditTxn, isDebitTxn)`** (handler.js:1801). Row shapes as §3.8 minus `streakName` and minus synthesised Expired rows; `{rewardHistory, text1:'You can redeem your coins in the the checkout stage at payment page - only eligible redeemable coins will be visible there!', text2:''}`.
- **`getActivityLogs(userId, year, month)`**: month grid `{activityLogs:{'1':[...],...}, activityLogTime:{'1': IST createdAt | null}, isYesterdayLogExist}` — keys are day-of-month strings for the whole month (default current month, end = today), values are `product_prescriptions` arrays; IST time via +330 min.
- **`POST /extraRewardsToUser/:caseId`** (handler.js:1882). `streakMasterId` and `reason` mandatory (400 'All fields are mandatory'). `StreakMaster.findOne({_id, is_for_superadmin:true})` else 400 `'Invalid streak master id'`. `customAmount` only when `slug === 'existing_coins_streak'` else 400 `'Invalid streak master id for custom amount'`. Phone via caseId → user. Returns `{rewardRef, message: '<n> coins credited successfully' | 'Failed to credit coins'}`.
- **`PUT /syncRewardBalanceWithShopFlo/:caseId`** (handler.js:3292). local = `getStreakAndRewardBalance(userId,70).rewardBalance`; shopflo = `round(total_wallet_balance * 10)`; NaN → 400 `'No shopflo coins found for this user'`. `expire_at` = tomorrow + 1 month (UTC midnight). shopflo > local → local credit for diff, `credit_remarks:'Credit reward to sync balance'`, master `existing_coins_streak`. local > shopflo → `createShopfloCreditRewardTransaction(diff, futureDate, uuidv4(), phone)`. Returns `{message:'Reward balance synced successfully'}`.
- **`GET /rewardBalance/:customerId`** (internal-service deployment) — same as `getUserRewardBalance` but the path param is a userId; in Go both resolve: try caseId → userId, fall back to treating the value as a userId.
- **`GET /streakMaster`** → §3.15.

## 3.18 Lifeline modules (port to `internal/habit/lifeline`)
**lifelineSchedule** (TZ Asia/Kolkata): `checkDateFor(lastLogDate) = IST startOfDay(lastLogDate)+1d 'YYYY-MM-DD'`; `dueAtFor(checkDate, graceHours) = IST(checkDate)+1d+graceHours`.
**lifelineQueue**: queue `habit-lifeline`, job `lifeline-check`, grace `HABIT_LIFELINE_GRACE_HOURS||4`, jobId `user_<userId>`, delay = `HABIT_LIFELINE_TEST_DELAY_MS` if set else `max(0, dueAt-now)`; schedule = remove-then-add upsert; `enqueueLifelineForLog(userId, logDate)`.
**lifelineDecision** (in order): `checkDateLogged → intact/next:true`; `!streakAlive → intact/next:false`; `!checkDateInKit → intact/next:false`; `kitPaused → break/next:false`; `budgetRemaining>0 → apply/next:true`; else `break/next:false`.
**lifelineState**: `getStreakState`: streak = StreakLog active; logged = ActivityLog `{user_id, is_active:true, is_valid_for_streak:true, is_lifeline:{$ne:true}, check_ins_for_date in [day00, nextDay00)}` → `{streakAlive: streak && days>0, checkDateLogged, streakDay}`. `breakStreak`: set `streak_achieve_days:0` on active streak. `getKitContext`: orders non-void; `kitStart = currentKitStart(orders, now, getKitDetail(o).kitCount)`; if none → `{null, budget 3, paused false}`; `lifelinesUsed = count lifeline logs date_covered>=kitStart`; `validLogs = count activity logs active+valid+check_ins>=kitStart`; → `{kitStart, budgetRemaining:max(0,3-used), kitPaused: validLogs>=35}`.
**habitKitWindow**: `currentKitStart(orders created_at DESC, today, getKitCount)`: iterate OLDEST first; skip kitCount<=0; `orderDate = delivery_date||created_at`; `subKitStart = prevKitEnd>orderDate ? prevKitEnd : orderDate`; for k in kitCount: `subKitEnd = +30d; kitStart = subKitStart; if today in [subKitStart, subKitEnd) return kitStart; subKitStart = subKitEnd`; `prevKitEnd = subKitStart`; return last kitStart.
**applyLifeline** (idempotent): `dateCovered = startOfDay(checkDate)`; create lifeline log (11000 → created=false); ensure activity doc `{check_ins_for_date:dateCovered, product_prescriptions: latest active doc's or [], is_valid_for_streak:true, is_active:true, is_lifeline:true}` if none exists; pipeline update streak `{user_id,is_active:true,last_date_of_log:{$lt:dateCovered}}` → `last_date_of_log=dateCovered, streak_achieve_days+1, longest=max`; del redis `kit-tracker-calendar!<userId>`; return `{created}`.
**lifelineWorker**: `processLifelineJob({userId, checkDate})`: state, kit, `checkDateInKit = kitStart && checkDate >= format(kitStart)`; decide; apply → `applyLifeline(streakDay+1)`; if created → `creditLifelineReward({userId, streakDay:streakDay+1, checkDate})` (habit mint/credit, non-fatal); break → `breakStreak`; next = scheduleNext ? `{checkDate+1d, dueAt}` : null; schedule next after completion. Concurrency 5.
**lifelineReconcile**: `reconcileUserLifelines({lifelineDates, realValidDates, budget=3})` removal-only; `findStreaksMissingCheck(active, pending) = active \ pending`.
**lifelineReconcileCron**: daily `30 23 * * *` UTC (05:00 IST); active streaks = `StreakLog.find({is_active:true, streak_achieve_days:{$gt:0}})`; for each without a pending check → `seedCheck(user_id, checkDateFor(last_date_of_log||now), dueAtFor(checkDate, 4h))`.
Currently disabled in api-server prod (`server/setup/bullmq.js:368-369`) — the Go service turns it on behind `HABIT_LIFELINE_WORKER_ENABLED`.

## 3.19 `bahValidator.js` rules
```
logActivityObject (POST /activityLogForBAH):
  isLogForToday boolean REQUIRED
  productPrescriptions array of objects (array optional):
     description string trim allow(''); morningCheckIns bool REQ; eveningCheckIns bool REQ; bothCheckInsRequired bool REQ;
     image_url object REQ; name string REQ; product_id string REQ; Dosage string REQ; dosageCode string default ''
  unknown keys REJECTED
validateMultipleActivityLogRequest (PUT /multipleActivityLogForBAH):
  isLogForToday bool REQ; logProductDetail array REQ of { productId NUMBER REQ, morningCheckIns bool REQ, eveningCheckIns bool REQ }
Failure → {status:400, message:`Validation error: ${error.details[0].message}`}   (Joi message text, e.g. `"isLogForToday" is required`)
```

---

# 4. EXTERNAL INTEGRATIONS

## 4.1 Shopflo wallet
Base `SHOPFLO_WALLET_API_ENPOINT` (typo kept; must end with `/`). Path `{BASE}issuer/{SHOPFLO_WALLET_ISSUER_ID}/merchant/{SHOPFLO_WALLET_MERCHANT_ID}/user-wallet…`. Header `Authorization: <SHOPFLO_WALLET_API_KEY>` raw (no Bearer), `Content-Type: application/json`.

| Op | Method+Path | Body |
|---|---|---|
| create wallet | `POST …/user-wallet` | `{oid: phoneNumber, initial_balance: 0, wallet_type: 'REWARDS'}` |
| credit | `POST …/user-wallet/credit-transaction` | `{oid: phoneNumber, reference_id: streakId, event_type:'REFERRED', reference:'REFERRAL', amount: reward/10, expiry_at: <epoch ms>}` |
| debit | `POST …/user-wallet/debit-transaction` | `{oid: phoneNumber, reference_id: referenceId, amount: reward/10, transaction_reference: reference || 'coins_debited_manually'}` |
| read | `GET …/user-wallet?phone-number=<urlencoded>` | — |

coins → wallet = /10; wallet → coins = ×10. `+91` prefixed unless present. Read 404 → auto-create wallet (credit path) or 400 `'No record found for this user'` (CRM read). Debit 404 → `'No record found for this user'`, 400 → `'User have less coin balance to debit'`. Credit/create swallow 404/400 into `{}`.

## 4.2 Order-service coin expiry
`GET ${ORDER_SERVICE_BASE_URL}/coin/expiry/month/<userId>` no auth → number of days.

## 4.3 Recommendation service
`RECOMMENDATION_SERVICE_BASE_URL`; `how-to-use/{caseId}`, `routine/{caseId}`; headers `Content-Type: application/json`, `Authorization: Bearer ${V2_FORM_DATA_TOKEN}`, `x-tenant-id: traya`; pass-through status; error body → `{err: <upstream body>}` with upstream status (routes 8/9 respond `res.status(status).json({ err })`).

## 4.5 Other
- Community: `${COMMUNITY_BASE_URL}landing/<authToken>?share=true&streak_count=&reward_coins=&cross=no&preventBack=true`.
- S3/CDN: `S3_IMAGE_BASE_URL` for `bgImg`, icons, `image_url.*`; modal media hardcoded to `https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/…`.
- CMS: `${CMS_SERVICE_BASE_URL}/api/component/<formId>/form/published` (15-day check-in).
- Config service: `${TR_CONFIG_SERVICE_BASE_URL}/static-content/data/OLD_TO_ACTIVE_VARIANT_IDS_MAP`, header `x-tenant-id`.
- Redis (`SERVICES_CACHE_HOST/PORT`): delete `kit-tracker-calendar!<userId>`; auth reads `user!<userId>login!status`.

## 4.6 CCD_UPDATE events
In-process EventEmitter today. Two events: `ACTIVITY_LOG` `{logDate, streakDate, streakCount}` → CCD `last_log_date, last_streak_date, current_streak_count`; `COIN_CREDITED` `{amount}` → `lifetime_earned_coins += amount, available_coins += amount`. Go publishes `{eventType, caseId, payload, tenantId}` to Redis channel `ccd_update`.

---

# 5. AUTH + ENVELOPES

Auth: Bearer JWT (`JWT_SECRET`, HS256), payload `id` → userId, `caseId` → caseId, `email`, `roles`, `tenants`, `first_name`, `phone_number`; Redis gate `user!<id>login!status` truthy. Errors: no header → 401 `{message:'No authorization header provided.'}`; invalid → 401 `{message:'Invalid or expired token.'}`. `checkTokenForV2FormData`: `x-access-token` or `authorization` equals `'Bearer ' + V2_FORM_DATA_TOKEN`. `isUserAdminOrSuperAdmin`: `roles[0] ∈ {ADMIN, SUPER_ADMIN, TEAM_LEAD}` else 403 `{Error:'Only admin and super admin are allowed'}`.

Envelopes used by the routes the Go service keeps:
| Route | Success | Error |
|---|---|---|
| streakMaster, medicinesForHowToUse, activityLogsBAH, streakAndRewardBalance, extraRewardsToUser, bahHistory, bahLogForGivenDate, sync | `200 <result>` | `500 {err: e}` (e serialised; for `{status,message}` factory objects this is `{err:{status,message}}`) |
| activityLogForBAH POST, multipleActivityLogForBAH | `200 <result>` | `res.status(err.status).json({message})` via generateInternalServerErrorRepsonse (400 for validation) |
| latestOrderHowtoUseV2, latestRoutineV2 | `200 <upstream>` | `res.status(status).json({err})` |
| rewardBalance/:caseId, archive/unarchive, scratch-card GET/reveal | `200 <result>` | `res.status(resolved).json({message})` |
| bah/:userId/calendar | `200 <result>` | `400 {error}` / `500 {error:'Internal Server Error', details}` |
| coinTransaction | `200 <result>` | `500 {err: e.message}` |
| kit-tracker-* | `200 <upstream>` | `res.status(upstreamStatus||500).json({err: upstreamBody||message})` |

---

# 6. CONSTANTS

| Name | Value |
|---|---|
| `STREAK_REWARDS` | `[{streak:3,coin:100},{streak:7,coin:400},{streak:21,coin:2000}]` |
| `O8_PLUS_STREAK_REWARDS` | `{3:200, 7:600, 21:2500}` |
| O8+ go-live / window / group / min orders | `'2026-04-10'` / 15 / `['2'..'9']` / 8 |
| Streak restart male | `ENABLED=false`, `COINS=100`, `GROUP=['0','1','a','b','c','d','e','f']` |
| Streak restart female | `ENABLED=true`, `COINS=100`, same group, `ORDER_COUNTS=[1]` |
| Lifelines / pause | 3 / 35 logs |
| Lifeline grace | `HABIT_LIFELINE_GRACE_HOURS || 4` |
| Kit length | 30 days per sub-kit |
| BAH log-access grace | +15 (`(kitCount*30)+15`) |
| Reorder banner | `kitExpireDays - minDaysAfterOrderDelivered <= 9` |
| How-to-use expiry | 50 days; bulk `duration*30 + 20` |
| Coin conversion | `'0.1'`; Shopflo amount = coins/10 |
| Coin discount cap | `{value:25, type:'percentage'}` |
| `autoApplyCoins` | true |
| coinTransaction page | page 1, limit 6 |
| `COINS_800_STREAK_REWARDS_ID` | `'reorder_800_coin_experiment'` → expiry 2 days |
| Feature-update cutoff | `2025-05-20` |
| `COIN_CREDIT_ACCESS_EMAIL_IDS` | `['vipinchauhan@traya.health','agarwalsandip@traya.health','sultantippu@traya.health','bharathputta@traya.health']` |
| Version gates | `>=71` showBahV3/banner/recompute; `>=85` archived filter |
| `FIFTEEN_DAY_CHECKIN_SUBTEXT` | `'You are staying consistent. Let your coach know if you are facing any issues with treatment'` |
| Vitamin dedupe | keep 44396552913074, drop 37547237310642 |
| `VARIANT_IDS_ARRAY` (kit variant ids for kitCount) | copy from `server/utils/config.js` |
| Task names | `build_a_habbit_sticky`, `build_a_habit`, `male_kit_started`, `female_kit_started` |

Media URLs: `Tick.json`, `confetti.json`, `local:YellowCoin.json`, `Error.png`, `streakbroke.png`, `rocket.json`, `Kit_Container.png`, `Missed.png` under `https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/`; `${S3}App/Home/WeeklyChallengeBannerIMG.png`, `${S3}App/bah/new_bah/Coins.svg|Lock.svg|coin.svg`; bahVideo female/male Shopify mp4s above.

## 6.6 Task marking on log
`updateTaskForUserToDisplay({userId, caseId, userTaskName, taskCompletionResponse:'CUSTOMER'})` → Mongo `task_master` (lookup by name) + `user_task_details` (set `is_active:false`, `completed_date`, `completed_by:'CUSTOMER'`, `$inc click_count`). Read `server/components/user_task/handler.js` for exact filters before porting.

---

# 7. PORTING GOTCHAS (decisions recorded in the spec)
1. Two clocks (offset-shift+UTC midnight vs process-local vs Asia/Kolkata) — port per call site.
2. `$gt` vs `$gte` mixes for "log exists this day" — keep per call site.
3. Day-of-month compare in `saveActivityLogs` — FIX (IST date compare).
4. `checkInDate.setDate` mutation in multiple-log — FIX (compute bounds first; same semantics).
5. Streak counters always 0 — KEEP.
6. `hasUserSeenBahUpdatedModalResult` literal true — KEEP.
7. Calendar `endDate = today` — KEEP; path userId IDOR — FIX.
8. O8+ display-only — KEEP.
9. Streak update by `{user_id}` alone — FIX (add `is_active:true`).
10. Dangling populate crash — FIX.
11. Lifeline worker disabled — Go enables behind a flag.
12. `_getPhoneNumberByCaseIdOrUserId` shadowing — FIX (resolve phone from caseId → user properly).
14. `SHOPFLO_WALLET_API_ENPOINT` typo — KEEP the env name.
