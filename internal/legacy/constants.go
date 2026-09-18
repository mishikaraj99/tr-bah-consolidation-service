// Package legacy implements the Traya 3/7/21 coin economy and the CRM surfaces.
// Every constant and copy string here is verbatim from traya-api-server (server/config.js and
// server/components/BAH/handler.js). See docs/reference/inventory-api-server-legacy-bah.md.
package legacy

import "fmt"

// Streak reward ladders.
type StreakReward struct {
	Streak int `json:"streak"`
	Coin   int `json:"coin"`
}

// StreakRewards is config.js STREAK_REWARDS.
var StreakRewards = []StreakReward{{Streak: 3, Coin: 100}, {Streak: 7, Coin: 400}, {Streak: 21, Coin: 2000}}

// O8PlusStreakRewards is config.js O8_PLUS_STREAK_REWARDS.
var O8PlusStreakRewards = map[int]int{3: 200, 7: 600, 21: 2500}

// O8+ experiment.
const (
	O8PlusGoLiveDate       = "2026-04-10"
	O8PlusExperimentWindow = 15
	O8PlusMinOrderCount    = 8
)

// O8PlusVariationGroup is the enhanced-rewards caseId prefix set.
var O8PlusVariationGroup = []string{"2", "3", "4", "5", "6", "7", "8", "9"}

// Streak-restart bonus experiment.
const (
	StreakRestartBonusEnabled       = false
	StreakRestartBonusCoins         = 100
	StreakRestartBonusFemaleEnabled = true
	StreakRestartBonusFemaleCoins   = 100
)

// StreakRestartVariationGroup is the caseId prefix set for both cohorts.
var StreakRestartVariationGroup = []string{"0", "1", "a", "b", "c", "d", "e", "f"}

// StreakRestartFemaleOrderCounts gates the female cohort to exactly one non-void order.
var StreakRestartFemaleOrderCounts = []int{1}

// Response constants.
const (
	CoinDiscountCapValue = 25
	CoinDiscountCapType  = "percentage"
	CoinConversionRatio  = "0.1"
	AutoApplyCoins       = true
	BahTitle             = "Log And Earn"
	BahBannerNewTitle    = "Log & Earn"
	ReminderOnBahPage    = "top"
)

// Reward expiry / experiment ids.
const (
	Coins800StreakRewardsID  = "reorder_800_coin_experiment"
	Coins800ExpiryDays       = 2
	SlugFirstCheckinExtra    = "first-checkin-extra-reward"
	SlugExistingCoinsStreak  = "existing_coins_streak"
	FeatureUpdateCutoffDate  = "2025-05-20"
	BahFeatureUpdateEvent    = "BAH_FEATURE_UPDATE"
	BahMissedLogYesterdayEvt = "BAH_MISSED_LOG_FOR_YESTERDAY"
)

// CoinCreditAccessEmailIDs may grant coins from the CRM.
var CoinCreditAccessEmailIDs = []string{
	"vipinchauhan@traya.health", "agarwalsandip@traya.health", "sultantippu@traya.health", "bharathputta@traya.health",
}

// 15-day check-in nudge.
const (
	FifteenDayCheckinFormID   = "component-1782457562767-VdwcN0"
	FifteenDayCheckinFormName = "15_Day_Checkin"
	FifteenDayCheckinSubtext  = "You are staying consistent. Let your coach know if you are facing any issues with treatment"
	FeedbackUIDomainProd      = "https://feedback.traya.health"
	FeedbackUIDomainDev       = "https://feedback-ui.dev.hav-g.in"
)

// Version gates.
const (
	VersionGateBahV3          = 71
	VersionGateArchivedFilter = 85
)

// Task names marked on a successful log ("build_a_habbit_sticky" typo is intentional).
const (
	TaskBuildAHabbitSticky = "build_a_habbit_sticky"
	TaskBuildAHabit        = "build_a_habit"
	TaskMaleKitStarted     = "male_kit_started"
	TaskFemaleKitStarted   = "female_kit_started"
	TaskCompletionCustomer = "CUSTOMER"
)

// BahTaskIDs is config.js BAH_TASK_ID (duplicate-cleanup set for build_a_habit).
var BahTaskIDs = []string{"651546c76a06665a750fb637", "693130a9c53f8c10456ff422"}

// Media URLs (hardcoded CloudFront paths in handler.js).
const (
	CDNBase                = "https://dvv8w2q8s3qot.cloudfront.net/App/bah/new_bah/"
	MediaTick              = CDNBase + "Tick.json"
	MediaConfetti          = CDNBase + "confetti.json"
	MediaYellowCoin        = "local:YellowCoin.json"
	MediaError             = CDNBase + "Error.png"
	MediaStreakBroke       = CDNBase + "streakbroke.png"
	MediaRocket            = CDNBase + "rocket.json"
	MediaKitContainer      = CDNBase + "Kit_Container.png"
	MediaMissed            = CDNBase + "Missed.png"
	BahVideoFemale         = "https://cdn.shopify.com/videos/c/o/v/639cdb2a63a245c2a46de9aa67bef697.mp4"
	BahVideoMale           = "https://cdn.shopify.com/videos/c/o/v/856c2a9f4dfb433b8dd99d3738621fc7.mp4"
	ChallengeBannerImgPath = "App/Home/WeeklyChallengeBannerIMG.png"
	IconCoinsSVG           = "App/bah/new_bah/Coins.svg"
	IconLockSVG            = "App/bah/new_bah/Lock.svg"
	IconCoinSVG            = "App/bah/new_bah/coin.svg"
)

// Special-case user ids preserved from handler.js.
const (
	OrderEligibleForLogCaseID = "02a883d7-8c25-43e0-975d-16ed335ce439"
	OrderEligibleForLogUntil  = "2026-04-20"
)

// Dosage codes (config.js DOSAGE_CODE).
const (
	DosageOnceAWeek          = "ONCE_A_WEEK"
	DosageTwiceAWeek         = "TWICE_A_WEEK"
	DosageThriceAWeek        = "THRICE_A_WEEK"
	DosageTwiceOrThriceAWeek = "TWICE_OR_THRICE_A_WEEK"
	Dosage100                = "1-0-0"
	Dosage200                = "2-0-0"
	Dosage010                = "0-1-0"
	Dosage001                = "0-0-1"
	Dosage002                = "0-0-2"
	Dosage101                = "1-0-1"
	Dosage202                = "2-0-2"
	Dosage111                = "1-1-1"
	Dosage1ml00              = "1ml-0-0"
	Dosage001ml              = "0-0-1ml"
	Dosage1ml01ml            = "1ml-0-1ml"
	DosageAsDirected         = "AS_DIRECTED"
)

// StreakPopup is one popupText entry.
type StreakPopup struct {
	H1 string `json:"h1"`
	H2 string `json:"h2"`
}

// PopupTextT is the ordered popupText payload.
type PopupTextT struct {
	OneDay      StreakPopup `json:"_1_day"`
	ThreeDays   StreakPopup `json:"_3_days"`
	SevenDays   StreakPopup `json:"_7_days"`
	TwentyOneD  StreakPopup `json:"_21_days"`
	CtaText     string      `json:"ctaText"`
	DismissText string      `json:"dismissText"`
}

// PopupText is config.js popupText.
var PopupText = PopupTextT{
	OneDay:      StreakPopup{H1: "YOU WON 100 COINS! 🎊", H2: "Redeem the coins before making final payment for next order. 10 coins = ₹1"},
	ThreeDays:   StreakPopup{H1: "3 DAY STREAK 💰", H2: "You won 100 coins! Stay regular and win bigger rewards."},
	SevenDays:   StreakPopup{H1: "7 DAY STREAK 💎", H2: "You won 400 coins! An even bigger reward awaits!"},
	TwentyOneD:  StreakPopup{H1: "21 DAY STREAK 🔥", H2: "You won 2000 coins! Your hair is grateful for your dedication."},
	CtaText:     "HOW TO REDEEM COINS?",
	DismissText: "DISMISS",
}

// CoinInfo is the coinApplied/coinNotApplied payload.
type CoinInfo struct {
	Title    string `json:"title"`
	SubTitle string `json:"subTitle"`
}

// CoinNotApplied and CoinApplied are identical in handler.js.
var (
	CoinNotApplied = CoinInfo{Title: "Apply Traya Coins", SubTitle: "Total coins: {TotalCoins}, Max usable: {ClaimableCoin}"}
	CoinApplied    = CoinInfo{Title: "Apply Traya Coins", SubTitle: "Total coins: {TotalCoins}, Max usable: {ClaimableCoin}"}
)

// Idempotency keys for credits (spec §4). credit_remarks carries these.
const (
	RemarkFirstLog        = "bah-legacy-first-log"
	RemarkStreakRestart   = "Streak restart bonus"
	RemarkSyncBalance     = "Credit reward to sync balance"
	remarkMilestonePrefix = "bah-legacy-"
	remarkRestartPrefix   = "bah-legacy-restart-"
)

// RemarkMilestone builds the idempotent credit_remarks for a 3/7/21 milestone credit.
func RemarkMilestone(slug, istDate string) string {
	return fmt.Sprintf("%s%s-%s", remarkMilestonePrefix, slug, istDate)
}

// RemarkRestart builds the idempotency key used to guard the streak-restart bonus for an IST day.
func RemarkRestart(istDate string) string { return remarkRestartPrefix + istDate }

// Messages returned verbatim by the write paths.
const (
	MsgCheckedInSuccess          = "User checked in successfully for date %s"
	MsgCannotLogForDate          = "User cannot log for date %s"
	MsgMultipleLogSuccess        = "Medicine logged successfully for your products"
	MsgBlankPrescriptionArray    = "Product prescription cannot be a blank array"
	MsgBlankPrescriptionExisting = "Product prescription cannot be blank array. Please check record saved successfully or not."
	MsgNotEligibleToCreateStreak = "Not eligible to create streak"
	MsgAllFieldsMandatory        = "All fields are mandatory"
	MsgInvalidStreakMaster       = "Invalid streak master id"
	MsgInvalidStreakMasterCustom = "Invalid streak master id for custom amount"
	MsgSyncSuccess               = "Reward balance synced successfully"
	MsgNoShopfloCoins            = "No shopflo coins found for this user"
	MsgInvalidDateFormat         = "Invalid date format."
	MsgNoActiveStreak            = "No active streak found for this user."
	MsgInvalidMode               = `Invalid mode. Use "calendar" or "streak".`
	MsgCoinsCreditedSuccess      = "%d coins credited successfully"
	MsgFailedToCreditCoins       = "Failed to credit coins"
	MsgRewardHistoryText1        = "You can redeem your coins in the the checkout stage at payment page - only eligible redeemable coins will be visible there!"
)
