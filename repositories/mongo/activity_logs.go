package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// DayRange expresses the exact comparison operators a call site used in the Node code.
type DayRange struct {
	From, To                   time.Time
	FromInclusive, ToInclusive bool
}

func (r DayRange) filter() bson.M {
	f := bson.M{}
	if r.FromInclusive {
		f["$gte"] = r.From
	} else {
		f["$gt"] = r.From
	}
	if r.ToInclusive {
		f["$lte"] = r.To
	} else {
		f["$lt"] = r.To
	}
	return f
}

func activeFilter(userID string, activeOnly bool) bson.M {
	f := bson.M{"user_id": userID}
	if activeOnly {
		f["is_active"] = true
	}
	return f
}

// FindActivityLogInRange returns the first log whose check_ins_for_date falls in r.
func (s *Store) FindActivityLogInRange(ctx context.Context, userID string, r DayRange, activeOnly bool) (*models.ActivityLog, error) {
	f := activeFilter(userID, activeOnly)
	f["check_ins_for_date"] = r.filter()
	var out models.ActivityLog
	if err := s.C(CollActivityLogs).FindOne(ctx, f).Decode(&out); err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// ExistsActivityLogInRange reports whether any log exists in r.
func (s *Store) ExistsActivityLogInRange(ctx context.Context, userID string, r DayRange, activeOnly bool) (bool, error) {
	doc, err := s.FindActivityLogInRange(ctx, userID, r, activeOnly)
	return doc != nil, err
}

// CreateActivityLog inserts doc, stamping timestamps.
func (s *Store) CreateActivityLog(ctx context.Context, doc *models.ActivityLog) (*models.ActivityLog, error) {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	if doc.ProductPrescriptions == nil {
		doc.ProductPrescriptions = []map[string]any{}
	}
	res, err := s.C(CollActivityLogs).InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc.ID = res.InsertedID.(primitive.ObjectID)
	return doc, nil
}

// FindActivityLogsBetween returns logs with from <= check_ins_for_date <= to.
func (s *Store) FindActivityLogsBetween(ctx context.Context, userID string, from, to time.Time, activeOnly bool, projection bson.M) ([]models.ActivityLog, error) {
	f := activeFilter(userID, activeOnly)
	f["check_ins_for_date"] = bson.M{"$gte": from, "$lte": to}
	opts := options.Find().SetSort(bson.D{{Key: "check_ins_for_date", Value: 1}})
	if projection != nil {
		opts.SetProjection(projection)
	}
	return s.findActivityLogs(ctx, f, opts)
}

// FindAllActivityLogs returns every active log for the user, oldest first.
func (s *Store) FindAllActivityLogs(ctx context.Context, userID string, projection bson.M) ([]models.ActivityLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "check_ins_for_date", Value: 1}})
	if projection != nil {
		opts.SetProjection(projection)
	}
	return s.findActivityLogs(ctx, bson.M{"user_id": userID, "is_active": true}, opts)
}

func (s *Store) findActivityLogs(ctx context.Context, f bson.M, opts *options.FindOptions) ([]models.ActivityLog, error) {
	cur, err := s.C(CollActivityLogs).Find(ctx, f, opts)
	if err != nil {
		return nil, err
	}
	var out []models.ActivityLog
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LatestActivityLogPrescriptions returns the newest active log's product_prescriptions (nil when none).
func (s *Store) LatestActivityLogPrescriptions(ctx context.Context, userID string) ([]map[string]any, error) {
	var out models.ActivityLog
	err := s.C(CollActivityLogs).FindOne(ctx, bson.M{"user_id": userID, "is_active": true},
		options.FindOne().SetSort(bson.D{{Key: "check_ins_for_date", Value: -1}}).SetProjection(bson.M{"product_prescriptions": 1})).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return out.ProductPrescriptions, nil
}

// FirstRealActivityLogOnOrAfter returns the earliest non-lifeline active log on/after from.
func (s *Store) FirstRealActivityLogOnOrAfter(ctx context.Context, userID string, from time.Time) (*models.ActivityLog, error) {
	var out models.ActivityLog
	err := s.C(CollActivityLogs).FindOne(ctx, bson.M{"user_id": userID, "is_active": true, "is_lifeline": bson.M{"$ne": true},
		"check_ins_for_date": bson.M{"$gte": from}}, options.FindOne().SetSort(bson.D{{Key: "check_ins_for_date", Value: 1}})).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// CountValidStreakLogs counts active + valid logs, optionally since a date.
func (s *Store) CountValidStreakLogs(ctx context.Context, userID string, since *time.Time) (int64, error) {
	f := bson.M{"user_id": userID, "is_active": true, "is_valid_for_streak": true}
	if since != nil {
		f["check_ins_for_date"] = bson.M{"$gte": *since}
	}
	return s.C(CollActivityLogs).CountDocuments(ctx, f)
}

// FindValidStreakLogDates returns check_ins_for_date of active+valid logs.
func (s *Store) FindValidStreakLogDates(ctx context.Context, userID string) ([]time.Time, error) {
	logs, err := s.findActivityLogs(ctx, bson.M{"user_id": userID, "is_active": true, "is_valid_for_streak": true},
		options.Find().SetProjection(bson.M{"check_ins_for_date": 1}))
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(logs))
	for _, l := range logs {
		out = append(out, l.CheckInsForDate)
	}
	return out, nil
}

// SetProductCheckIns mirrors the positional `$` update on product_prescriptions.
func (s *Store) SetProductCheckIns(ctx context.Context, logID primitive.ObjectID, productID string, morning, evening bool) error {
	_, err := s.C(CollActivityLogs).UpdateOne(ctx,
		bson.M{"_id": logID, "product_prescriptions.product_id": productID},
		bson.M{"$set": bson.M{"product_prescriptions.$.morningCheckIns": morning, "product_prescriptions.$.eveningCheckIns": evening}})
	return err
}

// BulkSetProductCheckIns applies SetProductCheckIns for many products in one bulkWrite.
func (s *Store) BulkSetProductCheckIns(ctx context.Context, logID primitive.ObjectID, updates map[string][2]bool) error {
	if len(updates) == 0 {
		return nil
	}
	ops := make([]mongo.WriteModel, 0, len(updates))
	for pid, me := range updates {
		ops = append(ops, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": logID, "product_prescriptions.product_id": pid}).
			SetUpdate(bson.M{"$set": bson.M{"product_prescriptions.$.morningCheckIns": me[0], "product_prescriptions.$.eveningCheckIns": me[1]}}))
	}
	_, err := s.C(CollActivityLogs).BulkWrite(ctx, ops)
	return err
}

// UpsertLifelineActivityLog mirrors app-backend upsertLifelineActivityLog ($setOnInsert).
func (s *Store) UpsertLifelineActivityLog(ctx context.Context, userID string, dateCovered time.Time, prescriptions []map[string]any) error {
	if prescriptions == nil {
		prescriptions = []map[string]any{}
	}
	now := s.Clock.Now()
	_, err := s.C(CollActivityLogs).UpdateOne(ctx,
		bson.M{"user_id": userID, "is_active": true, "check_ins_for_date": dateCovered},
		bson.M{"$setOnInsert": bson.M{"product_prescriptions": prescriptions, "is_valid_for_streak": true, "is_lifeline": true,
			"createdAt": now, "updatedAt": now}},
		options.Update().SetUpsert(true))
	return err
}
