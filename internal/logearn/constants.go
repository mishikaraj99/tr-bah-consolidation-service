// Package logearn implements the MOOL/ACNE Log & Earn rupee ledger.
// Ported from tr-consumer-backend src/modules/logearn; see docs/reference/inventory-consumer-backend-mool-logearn.md.
package logearn

// ZTier is one z-tier (₹ per day up to a streak day).
type ZTier struct {
	UpTo   *int `json:"upTo"`
	Amount int  `json:"amount"`
}

// Bonuses is the bonus configuration.
type Bonuses struct {
	FirstEver       int `json:"firstEver"`
	DayThree        int `json:"dayThree"`
	ReorderWelcome  int `json:"reorderWelcome"`
	MilestoneEvery  int `json:"milestoneEvery"`
	MilestoneAmount int `json:"milestoneAmount"`
}

func intPtr(v int) *int { return &v }

// DefaultZTiers is DEFAULT_Z_TIERS.
var DefaultZTiers = []ZTier{{UpTo: intPtr(7), Amount: 2}, {UpTo: intPtr(14), Amount: 3}, {UpTo: nil, Amount: 4}}

// DefaultBonuses is DEFAULT_BONUSES.
var DefaultBonuses = Bonuses{FirstEver: 5, DayThree: 5, ReorderWelcome: 5, MilestoneEvery: 7, MilestoneAmount: 10}

// DeadOrderStatuses is DEAD_ORDER_STATUSES.
var DeadOrderStatuses = []string{"void", "cancelled", "returned", "rto", "lost", "damaged", "ghost"}

// Ledger reasons.
const (
	ReasonDoseLog           = "DOSE_LOG"
	ReasonBackfill          = "BACKFILL"
	ReasonFirstEverBonus    = "FIRST_EVER_BONUS"
	ReasonDay3Bonus         = "DAY3_BONUS"
	ReasonMilestoneBonus    = "MILESTONE_BONUS"
	ReasonReorderWelcome    = "REORDER_WELCOME"
	ReasonLifelineRecovered = "LIFELINE_RECOVERED"
	ReasonRedeem            = "REDEEM"
)

// Cap defaults (DEFAULT_CAP_INFO).
const (
	DefaultBulkX          = 1
	DefaultEarningCapDays = 30
	DefaultTotalWindow    = 45
	KitDaysPerBulkUnit    = 30
	GraceWindowDays       = 15
	RedeemSubtotalShare   = 0.5
)

// Messages returned verbatim.
const (
	MsgAlreadyLoggedToday   = "Already logged today"
	MsgKitOnTheWay          = "Your kit is on the way! Dose logging starts once it is delivered."
	MsgNoActiveOrder        = "No active order found. Please place an order first."
	MsgWindowExpired        = "Earning window has expired. Please reorder to continue."
	MsgLogTodayFirst        = "Log today first before backfilling."
	MsgYesterdayLogged      = "Yesterday already logged"
	MsgStreakBrokenBackfill = "Your streak has broken, so there is nothing to backfill. Use your lifeline to restore it, or log today to start a new streak."
	MsgNoStreakToRecover    = "No streak to recover"
	MsgStreakNotBroken      = "Streak is not broken; lifeline not needed"
	MsgLifelineAlreadyUsed  = "Lifeline already used for this order"
	MsgLifelineApplied      = "Lifeline applied. Streak recovered."
	MsgNothingToRedeem      = "Nothing to redeem"
	MsgOrderAlreadyRedeemed = "This order is already redeemed"
	MsgCustomerIDRequired   = "customerId is required"
)
