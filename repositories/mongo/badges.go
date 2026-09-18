package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// FindEarnedBadges lists a user's earned badges.
func (s *Store) FindEarnedBadges(ctx context.Context, userID string) ([]models.Badge, error) {
	cur, err := s.C(CollBadges).Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	var out []models.Badge
	return out, cur.All(ctx, &out)
}

// UpsertEarnedBadge mirrors updateOne({user_id,kit_number}, {$setOnInsert:{...}}, {upsert:true}).
func (s *Store) UpsertEarnedBadge(ctx context.Context, userID string, kit int, now time.Time, source string) error {
	_, err := s.C(CollBadges).UpdateOne(ctx, bson.M{"user_id": userID, "kit_number": kit},
		bson.M{"$setOnInsert": bson.M{"user_id": userID, "kit_number": kit, "source": source, "earned_at": now, "createdAt": now, "updatedAt": now}},
		options.Update().SetUpsert(true))
	if IsDuplicateKey(err) {
		return nil
	}
	return err
}
