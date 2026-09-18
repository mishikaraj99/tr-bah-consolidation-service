package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// FindCustomerActivityLog finds one event for a case, optionally within a createdAt window [from, to).
func (s *Store) FindCustomerActivityLog(ctx context.Context, caseID, event string, createdBetween *[2]time.Time) (*models.CustomerActivityLog, error) {
	f := bson.M{"case_id": caseID, "event": event}
	if createdBetween != nil {
		f["createdAt"] = bson.M{"$gte": createdBetween[0], "$lt": createdBetween[1]}
	}
	var out models.CustomerActivityLog
	if err := s.C(CollCustomerActivityLogs).FindOne(ctx, f).Decode(&out); err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// LatestReminderInfo returns the newest customer activity log carrying reminder_days.
func (s *Store) LatestReminderInfo(ctx context.Context, caseID string) (*models.CustomerActivityLog, error) {
	var out models.CustomerActivityLog
	err := s.C(CollCustomerActivityLogs).FindOne(ctx, bson.M{"case_id": caseID, "reminder_days": bson.M{"$exists": true, "$ne": bson.A{}}},
		options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}})).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}
