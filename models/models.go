// Package models holds the Mongo document shapes shared by every economy.
// bson tags are the exact field names used by the Node services; do not rename.
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ActivityLog is one document of user_activity_logs_for_bah.
type ActivityLog struct {
	ID                   primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	UserID               string             `bson:"user_id" json:"user_id"`
	CheckInsForDate      time.Time          `bson:"check_ins_for_date" json:"check_ins_for_date"`
	ProductPrescriptions []map[string]any   `bson:"product_prescriptions" json:"product_prescriptions"`
	IsValidForStreak     bool               `bson:"is_valid_for_streak" json:"is_valid_for_streak"`
	IsActive             bool               `bson:"is_active" json:"is_active"`
	IsLifeline           *bool              `bson:"is_lifeline,omitempty" json:"is_lifeline,omitempty"`
	LogSource            string             `bson:"log_source,omitempty" json:"log_source,omitempty"`
	CreatedAt            time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt            time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Lifeline reports is_lifeline treating nil as false.
func (a *ActivityLog) Lifeline() bool { return a.IsLifeline != nil && *a.IsLifeline }

// StreakLog is one document of streak_logs.
type StreakLog struct {
	ID                primitive.ObjectID `bson:"_id,omitempty"`
	UserID            string             `bson:"user_id"`
	StreakAchieveDays int                `bson:"streak_achieve_days"`
	LongestStreakDays int                `bson:"longest_streak_days"`
	FirstDateOfLog    time.Time          `bson:"first_date_of_log"`
	LastDateOfLog     time.Time          `bson:"last_date_of_log"`
	IsActive          *bool              `bson:"is_active,omitempty"`
	LogSource         string             `bson:"log_source,omitempty"`
	CreatedAt         time.Time          `bson:"createdAt"`
	UpdatedAt         time.Time          `bson:"updatedAt"`
}

// StreakMaster is one document of streak_masters.
type StreakMaster struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	DisplayName     string             `bson:"display_name"`
	Days            int                `bson:"days"`
	Slug            string             `bson:"slug"`
	IsActive        bool               `bson:"is_active"`
	IsForSuperadmin bool               `bson:"is_for_superadmin"`
	RewardCoins     int                `bson:"reward_coins"`
	CreatedAt       time.Time          `bson:"createdAt,omitempty"`
	UpdatedAt       time.Time          `bson:"updatedAt,omitempty"`
}

// DebitTransaction is an embedded element of reward_transactions.debit_transactions.
type DebitTransaction struct {
	DebitTransactionID          primitive.ObjectID `bson:"debit_transaction_id"`
	DebitCoins                  int                `bson:"debit_coins"`
	TransactionStatus           string             `bson:"transaction_status"`
	DebitRemarks                string             `bson:"debit_remarks"`
	DebitTransactionDate        time.Time          `bson:"debit_transaction_date"`
	DebitTransactionUpdatedDate *time.Time         `bson:"debit_transaction_updated_date,omitempty"`
}

// RewardTransaction is one document of reward_transactions.
type RewardTransaction struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"`
	UserID              string             `bson:"user_id"`
	StreakMasterID      primitive.ObjectID `bson:"streak_master_id"`
	CreditCoins         int                `bson:"credit_coins"`
	IsCreditTransaction bool               `bson:"is_credit_transaction"`
	TotalDebitCoins     int                `bson:"total_debit_coins"`
	IsDebitTransaction  bool               `bson:"is_debit_transaction"`
	DebitTransactions   []DebitTransaction `bson:"debit_transactions"`
	CreditRemarks       string             `bson:"credit_remarks"`
	// IdempotencyKey is set only for credits this service must never duplicate (first log, the
	// 3/7/21 milestones, the habit-tracker daily/ladder rewards). A unique partial index enforces
	// it. It is separate from credit_remarks because that field is human-facing copy and is reused
	// by CRM grants, and because Mongo partial filters cannot match a prefix.
	IdempotencyKey *string    `bson:"idempotency_key,omitempty"`
	AllCoinsUsed   bool       `bson:"all_coins_used"`
	Status         string     `bson:"status"`
	ExpireAt       *time.Time `bson:"expire_at,omitempty"`
	CreatedAt      time.Time  `bson:"createdAt"`
	UpdatedAt      time.Time  `bson:"updatedAt"`
}

// RedeemTransaction is one document of redeem_reward_transactions.
type RedeemTransaction struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	UserID         string             `bson:"user_id"`
	RedeemedCoins  int                `bson:"redeemed_coins"`
	RedeemedAmount float64            `bson:"redeemed_amount"`
	OrderID        *string            `bson:"order_id"`
	OrderDisplayID *string            `bson:"order_display_id"`
	ShopFloTxnID   *string            `bson:"shop_flo_txn_id,omitempty"`
	Currency       string             `bson:"currency"`
	Status         string             `bson:"status"`
	Remarks        string             `bson:"remarks"`
	Meta           map[string]any     `bson:"meta,omitempty"`
	CreatedAt      time.Time          `bson:"createdAt"`
	UpdatedAt      time.Time          `bson:"updatedAt"`
}

// ArchivedProducts is one document of user_bah_archived_products.
type ArchivedProducts struct {
	UserID             string    `bson:"user_id"`
	ArchivedProductIDs []string  `bson:"archived_product_ids"`
	UpdatedAt          time.Time `bson:"updated_at"`
}

// LifelineLog is one document of habit_tracker_lifeline_logs.
type LifelineLog struct {
	UserID           string    `bson:"user_id"`
	DateCovered      time.Time `bson:"date_covered"`
	KitStart         time.Time `bson:"kit_start"`
	AppliedAt        time.Time `bson:"applied_at"`
	Source           string    `bson:"source"`
	StreakDayAtApply int       `bson:"streak_day_at_apply"`
	CreatedAt        time.Time `bson:"createdAt"`
	UpdatedAt        time.Time `bson:"updatedAt"`
}

// ScratchCardMeta is the embedded reward_meta of scratch_cards.
type ScratchCardMeta struct {
	Slug      string `bson:"slug"`
	StreakDay int    `bson:"streak_day"`
	Tier      string `bson:"tier"`
}

// ScratchCard is one document of scratch_cards.
type ScratchCard struct {
	ID                   primitive.ObjectID  `bson:"_id,omitempty"`
	UserID               string              `bson:"user_id"`
	Source               string              `bson:"source"`
	RewardType           string              `bson:"reward_type"`
	RewardValue          int                 `bson:"reward_value"`
	RewardMeta           ScratchCardMeta     `bson:"reward_meta"`
	RewardDayDate        string              `bson:"reward_day_date"`
	PhoneNumber          string              `bson:"phone_number"`
	Status               string              `bson:"status"`
	ExpiresAt            time.Time           `bson:"expires_at"`
	ClaimedAt            *time.Time          `bson:"claimed_at"`
	RewardTransactionRef *primitive.ObjectID `bson:"reward_transaction_ref"`
	CreditRemarks        string              `bson:"credit_remarks"`
	CreatedAt            time.Time           `bson:"createdAt"`
	UpdatedAt            time.Time           `bson:"updatedAt"`
}

// Badge is one document of user_habit_tracker_badges.
type Badge struct {
	UserID    string    `bson:"user_id"`
	KitNumber int       `bson:"kit_number"`
	BadgeID   string    `bson:"badge_id,omitempty"`
	Source    string    `bson:"source"`
	EarnedAt  time.Time `bson:"earned_at"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// CustomerActivityLog is one document of customeractivitylogs.
type CustomerActivityLog struct {
	CaseID       string    `bson:"case_id"`
	ActionDate   time.Time `bson:"action_date"`
	Event        string    `bson:"event"`
	OrderID      *string   `bson:"order_id"`
	ReminderDays []int     `bson:"reminder_days"`
	CreatedAt    time.Time `bson:"createdAt"`
}

// TaskMaster is one document of task_masters.
type TaskMaster struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	TaskName string             `bson:"task_name"`
	IsActive bool               `bson:"is_active"`
	Type     string             `bson:"type"`
}

// UserTaskDetail is one document of user_task_details.
type UserTaskDetail struct {
	ID                     primitive.ObjectID `bson:"_id,omitempty"`
	UserID                 string             `bson:"user_id"`
	CaseID                 string             `bson:"case_id"`
	TaskID                 primitive.ObjectID `bson:"task_id"`
	IsActive               bool               `bson:"is_active"`
	StartDate              time.Time          `bson:"start_date"`
	DueDate                *time.Time         `bson:"due_date"`
	TaskCompletionResponse string             `bson:"task_completion_response,omitempty"`
	CompletedDate          *time.Time         `bson:"completed_date,omitempty"`
	CompletedBy            string             `bson:"completed_by,omitempty"`
	ClickCount             int                `bson:"click_count,omitempty"`
	OrderID                string             `bson:"order_id,omitempty"`
	CreatedAt              time.Time          `bson:"createdAt"`
	UpdatedAt              time.Time          `bson:"updatedAt"`
}

// Tenant is one document of master.tenant.
type Tenant struct {
	TenantID   string `bson:"tenant_id"`
	TenantName string `bson:"tenant_name"`
}
