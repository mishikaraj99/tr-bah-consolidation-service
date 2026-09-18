// Package habit implements the v85 Habit Tracker economy (kit tracker, scratch cards, badges, lifelines).
// Constants and copy are verbatim from traya-app-backend; see docs/reference/inventory-app-backend-v85-habit-tracker.md.
package habit

// Reward ladder and daily tiers.
const (
	CoinToRupeeDivisor    = 10
	DailyCoinsBase        = 30
	DailyWeeklyStep       = 10
	DailyMaxWeek          = 5
	MaxRewardDay          = 35
	PauseAfterLogs        = 35
	KitWindowDaysPerKit   = 30
	KitWindowBufferDays   = 10
	LifelinesTotal        = 3
	WindowDaysBack        = 7
	WindowDaysForward     = 7
	BadgeThreshold        = 30
	BadgesTotal           = 21
	BadgeImageMaxKit      = 21
	FirstLogRewardCoins   = 100
	CoinExpiryDays        = 90
	ScratchCardWindowDays = 7
	ReorderCTAMinKitAge   = 21
)

// RewardLadder is HABIT_TRACKER_REWARD_LADDER.
var RewardLadder = map[int]int{7: 100, 14: 150, 21: 200, 28: 250, 35: 300}

// LadderDays lists the ladder days in order.
var LadderDays = []int{7, 14, 21, 28, 35}

// DailyTier is one HABIT_TRACKER_DAILY_TIERS entry.
type DailyTier struct {
	Week  int
	Slug  string
	Coins int
}

// DailyTiers is HABIT_TRACKER_DAILY_TIERS.
var DailyTiers = []DailyTier{
	{Week: 1, Slug: "habit-daily-week-1", Coins: 30},
	{Week: 2, Slug: "habit-daily-week-2", Coins: 40},
	{Week: 3, Slug: "habit-daily-week-3", Coins: 50},
	{Week: 4, Slug: "habit-daily-week-4", Coins: 60},
	{Week: 5, Slug: "habit-daily-week-5", Coins: 70},
}

// LadderTier is one HABIT_TRACKER_LADDER entry.
type LadderTier struct {
	Days  int
	Slug  string
	Coins int
}

// Ladder is HABIT_TRACKER_LADDER.
var Ladder = []LadderTier{
	{Days: 7, Slug: "habit-ladder-7", Coins: 100},
	{Days: 14, Slug: "habit-ladder-14", Coins: 150},
	{Days: 21, Slug: "habit-ladder-21", Coins: 200},
	{Days: 28, Slug: "habit-ladder-28", Coins: 250},
	{Days: 35, Slug: "habit-ladder-35", Coins: 300},
}

// Idempotency prefixes for credit_remarks.
const (
	DailyRemarksPrefix  = "habit-tracker-daily-"
	LadderRemarksPrefix = "habit-tracker-ladder-"
	ScratchSource       = "habit_tracker"
)

// Header/CTA/intro copy.
const (
	HeaderOverline     = "KIT TRACKER"
	HeaderHeading      = "Did you use your kit today?"
	CtaLabel           = "Log Now"
	CtaAction          = "habit_tracker"
	IntroHeading       = "Use kit regularly to get discounts"
	IntroCtaLabel      = "Explore Now"
	CoinsDisplayText   = "Bonus Coins Earned!"
	ReorderCtaLabel    = "Order Next Kit"
	KitGoalTitle       = "Hair gets thick & strong"
	KitGoalLogs        = 30
	FeedbackComponent  = "feedback_v3"
	FeedbackOrderCount = 1
)

// Reorder banner offsets.
const (
	ReorderBannerDaysPerKit = 30
	ReorderBannerStartOff   = -9
	ReorderBannerPauseOff   = 5
)

// DayLabels is HABIT_TRACKER_DAY_LABELS (moment().day(): 0=Sun).
var DayLabels = []string{"S", "M", "T", "W", "T", "F", "S"}

const assetPrefix = "https://cdn.shopify.com/s/files/1/0100/1622/7394/files/"

// Assets is HABIT_TRACKER_ASSETS.
var Assets = map[string]string{
	"coins":                   assetPrefix + "Coin.svg?v=1783944580",
	"streak":                  assetPrefix + "Streak.svg?v=1783925135",
	"lifeline":                assetPrefix + "lifeline.svg?v=1783925135",
	"lifelineUsed":            assetPrefix + "lifeline-used.svg?v=1783925135",
	"lifelineDisabled":        assetPrefix + "lifeline-disabled.svg?v=1784804191",
	"rewardUpcoming":          assetPrefix + "Property_1_gift_new.svg?v=1783925135",
	"rewardReceived":          assetPrefix + "reward-received.svg?v=1783925135",
	"rewardLocked":            assetPrefix + "reward-grey.svg?v=1783925134",
	"locked":                  assetPrefix + "Property_1_locked.svg?v=1783925135",
	"streakStaysAlive":        assetPrefix + "First_Rewards_Container.svg?v=1785332255",
	"streakBreaksBS":          assetPrefix + "Streak_breaks.svg?v=1785331824",
	"coinsLocked":             assetPrefix + "Property_1_coins.svg?v=1783925135",
	"streakBreak":             assetPrefix + "streak-break-svg.svg?v=1785332818",
	"streakDisabled":          assetPrefix + "Streak-disabled.svg?v=1784804060",
	"kitBox":                  assetPrefix + "KitArriving.png?v=1784719927",
	"kitImage":                assetPrefix + "KitImage_893677a2-53da-4752-99c2-c04637eaf92d.png?v=1784719525",
	"scratchTileCover":        assetPrefix + "all-cards-scratch.png?v=1784719526",
	"expiredTileCover":        assetPrefix + "all-cards-expired.png?v=1784719524",
	"scratchRevealThumbnail":  assetPrefix + "ScratchRevealThumbnail_1.png?v=1785163313",
	"noRewardScreen":          assetPrefix + "noRewardScreen.png?v=1785333032",
	"streakBreakMainCalender": assetPrefix + "streakBreakMainCalender.svg?v=1785339424",
}

// Badge images by gender key ({n} is the kit number).
var BadgeImages = map[string]string{
	"M":        assetPrefix + "Male_badges_{n}.png?v=1784111714",
	"F":        assetPrefix + "Female_badges_{n}.png?v=1784111743",
	"disabled": assetPrefix + "Disable_Badge_{n}.png?v=1784111725",
}

// Error messages returned verbatim by the controllers.
const (
	MsgInvalidUserID           = "Invalid or missing userId"
	MsgInvalidCaseID           = "Invalid or missing caseId"
	MsgInvalidStreakDay        = "Invalid or missing streakDay"
	MsgMissingPhoneNumber      = "Missing phoneNumber"
	MsgMissingCheckInDate      = "Missing checkInDate"
	MsgProductIDRequired       = "productId is required"
	MsgCannotRemoveLast        = "Cannot remove the last product"
	MsgCardNotFound            = "Scratch card not found"
	MsgCardExpired             = "Scratch card has expired"
	MsgUnsupportedReward       = "Unsupported reward type"
	MsgKitTrackerDataUnavail   = "Kit tracker data unavailable"
	MsgKitTrackerCalUnavail    = "Kit tracker calendar unavailable"
	MsgKitTrackerBadgesUnavail = "Kit tracker badges unavailable"
	MsgRedeemSuccess           = "Redeem coin transaction executed successfully"
	MsgUserNotFound            = "User not found"
	MsgNotEnoughCoins          = "User have not enough coins"
	MsgOrderAlreadyRedeemed    = "This order is already redeemed"
	MsgRewardAlreadyRedeemed   = "This reward is already redeemed"
	CalendarCacheTTLSeconds    = 300
)

// CalendarCacheKey is the Redis key app-backend owns (the write path must invalidate it).
func CalendarCacheKey(userID string) string { return "kit-tracker-calendar!" + userID }
