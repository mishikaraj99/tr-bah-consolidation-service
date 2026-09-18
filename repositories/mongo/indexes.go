package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// IdempotentRemarkPattern matches credit_remarks that are idempotency keys (spec §4).
const IdempotentRemarkPattern = "^(bah-legacy-|habit-tracker-)"

// EnsureIndexes creates every index the service relies on. Idempotent; safe to call per tenant DB.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	uniq := options.Index().SetUnique(true)
	specs := map[string][]mongo.IndexModel{
		CollRewardTransactions: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "status", Value: 1}, {Key: "expire_at", Value: 1}, {Key: "all_coins_used", Value: 1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "credit_remarks", Value: 1}},
				Options: options.Index().SetUnique(true).SetName("uq_user_idempotent_credit_remarks").SetPartialFilterExpression(bson.M{
					"is_credit_transaction": true,
					"credit_remarks":        bson.M{"$regex": IdempotentRemarkPattern},
				})},
		},
		CollRedeemTransactions: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "order_id", Value: 1}},
				Options: options.Index().SetUnique(true).SetName("uq_user_success_order").SetPartialFilterExpression(bson.M{
					"order_id": bson.M{"$type": "string"}, "status": "success",
				})},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "shop_flo_txn_id", Value: 1}},
				Options: options.Index().SetUnique(true).SetName("uq_user_shopflo_txn").SetPartialFilterExpression(bson.M{
					"shop_flo_txn_id": bson.M{"$type": "string"},
				})},
		},
		CollActivityLogs:  {{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "is_active", Value: 1}, {Key: "check_ins_for_date", Value: -1}}}},
		CollStreakLogs:    {{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "is_active", Value: 1}}}},
		CollStreakMasters: {{Keys: bson.D{{Key: "slug", Value: 1}}, Options: uniq}},
		CollScratchCards: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "source", Value: 1}, {Key: "reward_day_date", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "status", Value: 1}}},
		},
		CollBadges:               {{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "kit_number", Value: 1}}, Options: options.Index().SetUnique(true)}},
		CollLifelineLogs:         {{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "date_covered", Value: 1}}, Options: options.Index().SetUnique(true)}},
		CollArchivedProducts:     {{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)}},
		CollCustomerActivityLogs: {{Keys: bson.D{{Key: "case_id", Value: 1}, {Key: "event", Value: 1}, {Key: "createdAt", Value: -1}}}},
	}
	for coll, models := range specs {
		if _, err := db.Collection(coll).Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}
