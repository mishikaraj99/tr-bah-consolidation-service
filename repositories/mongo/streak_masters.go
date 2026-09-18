package mongorepo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

func (s *Store) findMaster(ctx context.Context, f bson.M) (*models.StreakMaster, error) {
	var out models.StreakMaster
	if err := s.C(CollStreakMasters).FindOne(ctx, f).Decode(&out); err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// FindStreakMasterBySlug finds a master by slug.
func (s *Store) FindStreakMasterBySlug(ctx context.Context, slug string, activeOnly bool) (*models.StreakMaster, error) {
	f := bson.M{"slug": slug}
	if activeOnly {
		f["is_active"] = true
	}
	return s.findMaster(ctx, f)
}

// FindStreakMasterByDays finds the active master for a milestone day count.
func (s *Store) FindStreakMasterByDays(ctx context.Context, days int) (*models.StreakMaster, error) {
	return s.findMaster(ctx, bson.M{"is_active": true, "days": days})
}

// FindStreakMasterByID finds by _id, optionally requiring is_for_superadmin.
func (s *Store) FindStreakMasterByID(ctx context.Context, id primitive.ObjectID, superadminOnly bool) (*models.StreakMaster, error) {
	f := bson.M{"_id": id}
	if superadminOnly {
		f["is_for_superadmin"] = true
	}
	return s.findMaster(ctx, f)
}

// FindSuperadminStreakMasters lists active superadmin masters.
func (s *Store) FindSuperadminStreakMasters(ctx context.Context) ([]models.StreakMaster, error) {
	cur, err := s.C(CollStreakMasters).Find(ctx, bson.M{"is_active": true, "is_for_superadmin": true})
	if err != nil {
		return nil, err
	}
	var out []models.StreakMaster
	return out, cur.All(ctx, &out)
}

// UpsertStreakMasterBySlug mirrors app-backend upsertStreakMasterBySlug ($setOnInsert, return after).
func (s *Store) UpsertStreakMasterBySlug(ctx context.Context, slug string, defaults *models.StreakMaster) (*models.StreakMaster, error) {
	now := s.Clock.Now()
	var out models.StreakMaster
	err := s.C(CollStreakMasters).FindOneAndUpdate(ctx, bson.M{"slug": slug},
		bson.M{"$setOnInsert": bson.M{"slug": slug, "display_name": defaults.DisplayName, "days": defaults.Days, "is_active": defaults.IsActive,
			"is_for_superadmin": defaults.IsForSuperadmin, "reward_coins": defaults.RewardCoins, "createdAt": now, "updatedAt": now}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// StreakMasterNames maps ids to display_name.
func (s *Store) StreakMasterNames(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]string, error) {
	out := map[primitive.ObjectID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.C(CollStreakMasters).Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"display_name": 1}))
	if err != nil {
		return nil, err
	}
	var rows []models.StreakMaster
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.DisplayName
	}
	return out, nil
}
