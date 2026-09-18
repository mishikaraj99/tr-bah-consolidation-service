// Package mongorepo holds tenant-scoped Mongo access for the BAH collections.
package mongorepo

import (
	"errors"

	"go.mongodb.org/mongo-driver/mongo"

	"traya-bah-service/internal/common"
)

// Collection names (Mongoose model names → collection names).
const (
	CollActivityLogs         = "user_activity_logs_for_bah"
	CollStreakLogs           = "streak_logs"
	CollStreakMasters        = "streak_masters"
	CollRewardTransactions   = "reward_transactions"
	CollRedeemTransactions   = "redeem_reward_transactions"
	CollArchivedProducts     = "user_bah_archived_products"
	CollLifelineLogs         = "habit_tracker_lifeline_logs"
	CollScratchCards         = "scratch_cards"
	CollBadges               = "user_habit_tracker_badges"
	CollCustomerActivityLogs = "customeractivitylogs"
	CollTaskMaster           = "task_masters"
	CollUserTaskDetails      = "user_task_details"
)

// Store is a per-tenant handle on the BAH collections.
type Store struct {
	DB    *mongo.Database
	Clock common.Clock
}

// NewStore binds a store to one tenant database.
func NewStore(db *mongo.Database, clk common.Clock) *Store {
	if clk == nil {
		clk = common.RealClock{}
	}
	return &Store{DB: db, Clock: clk}
}

// C returns the named collection.
func (s *Store) C(name string) *mongo.Collection { return s.DB.Collection(name) }

// IsDuplicateKey reports a unique-index violation (E11000).
func IsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) && ce.Code == 11000 {
		return true
	}
	var bwe mongo.BulkWriteException
	if errors.As(err, &bwe) {
		for _, e := range bwe.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	return mongo.IsDuplicateKeyError(err)
}

// noDoc maps ErrNoDocuments to (false, nil).
func noDoc(err error) (bool, error) {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return true, nil
	}
	return false, err
}
