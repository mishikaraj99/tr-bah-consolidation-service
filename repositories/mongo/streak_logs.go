package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// FindActiveStreak returns the user's active streak (nil when none).
func (s *Store) FindActiveStreak(ctx context.Context, userID string) (*models.StreakLog, error) {
	var out models.StreakLog
	err := s.C(CollStreakLogs).FindOne(ctx, bson.M{"user_id": userID, "is_active": true},
		options.FindOne().SetSort(bson.D{{Key: "updatedAt", Value: -1}})).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// CreateStreak inserts a new streak document.
func (s *Store) CreateStreak(ctx context.Context, doc *models.StreakLog) (*models.StreakLog, error) {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	res, err := s.C(CollStreakLogs).InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc.ID = res.InsertedID.(primitive.ObjectID)
	return doc, nil
}

// UpdateActiveStreak applies $set to the active streak and returns the updated doc.
func (s *Store) UpdateActiveStreak(ctx context.Context, userID string, set bson.M) (*models.StreakLog, error) {
	set["updatedAt"] = s.Clock.Now()
	var out models.StreakLog
	err := s.C(CollStreakLogs).FindOneAndUpdate(ctx, bson.M{"user_id": userID, "is_active": true}, bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// BreakStreak sets streak_achieve_days to 0 on the active streak.
func (s *Store) BreakStreak(ctx context.Context, userID string) error {
	_, err := s.C(CollStreakLogs).UpdateOne(ctx, bson.M{"user_id": userID, "is_active": true},
		bson.M{"$set": bson.M{"streak_achieve_days": 0, "updatedAt": s.Clock.Now()}})
	return err
}

// AdvanceStreakForDate mirrors applyLifeline's pipeline update.
func (s *Store) AdvanceStreakForDate(ctx context.Context, userID string, dateCovered time.Time) error {
	pipeline := bson.A{
		bson.M{"$set": bson.M{"last_date_of_log": dateCovered, "streak_achieve_days": bson.M{"$add": bson.A{"$streak_achieve_days", 1}}, "updatedAt": s.Clock.Now()}},
		bson.M{"$set": bson.M{"longest_streak_days": bson.M{"$max": bson.A{"$longest_streak_days", "$streak_achieve_days"}}}},
	}
	_, err := s.C(CollStreakLogs).UpdateOne(ctx, bson.M{"user_id": userID, "is_active": true, "last_date_of_log": bson.M{"$lt": dateCovered}}, pipeline)
	return err
}

// FindActiveStreaksWithDays returns active streaks with streak_achieve_days > 0 (user_id, last_date_of_log).
func (s *Store) FindActiveStreaksWithDays(ctx context.Context) ([]models.StreakLog, error) {
	cur, err := s.C(CollStreakLogs).Find(ctx, bson.M{"is_active": true, "streak_achieve_days": bson.M{"$gt": 0}},
		options.Find().SetProjection(bson.M{"user_id": 1, "last_date_of_log": 1}))
	if err != nil {
		return nil, err
	}
	var out []models.StreakLog
	return out, cur.All(ctx, &out)
}
