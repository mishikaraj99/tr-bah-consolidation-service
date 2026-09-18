package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// CreateLifelineLog inserts a lifeline log; a duplicate (user_id, date_covered) returns (false, nil).
func (s *Store) CreateLifelineLog(ctx context.Context, doc *models.LifelineLog) (bool, error) {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	if doc.Source == "" {
		doc.Source = "auto"
	}
	if _, err := s.C(CollLifelineLogs).InsertOne(ctx, doc); err != nil {
		if IsDuplicateKey(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// LifelineDates returns every date_covered for the user, ascending.
func (s *Store) LifelineDates(ctx context.Context, userID string) ([]time.Time, error) {
	cur, err := s.C(CollLifelineLogs).Find(ctx, bson.M{"user_id": userID},
		options.Find().SetProjection(bson.M{"date_covered": 1}).SetSort(bson.D{{Key: "date_covered", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var rows []models.LifelineLog
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.DateCovered)
	}
	return out, nil
}

// CountLifelinesSince counts lifelines with date_covered >= since.
func (s *Store) CountLifelinesSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	return s.C(CollLifelineLogs).CountDocuments(ctx, bson.M{"user_id": userID, "date_covered": bson.M{"$gte": since}})
}
