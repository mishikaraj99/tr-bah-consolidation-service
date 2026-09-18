package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// ArchivedProductIDs returns the user's archived product ids as a set.
func (s *Store) ArchivedProductIDs(ctx context.Context, userID string) (map[string]bool, error) {
	var doc models.ArchivedProducts
	err := s.C(CollArchivedProducts).FindOne(ctx, bson.M{"user_id": userID}).Decode(&doc)
	out := map[string]bool{}
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return out, err
		}
	}
	for _, id := range doc.ArchivedProductIDs {
		out[id] = true
	}
	return out, nil
}

// AddArchivedProduct mirrors findOneAndUpdate({user_id}, {$addToSet, $set updated_at}, {upsert, new}).
func (s *Store) AddArchivedProduct(ctx context.Context, userID, productID string, now time.Time) ([]string, error) {
	var doc models.ArchivedProducts
	err := s.C(CollArchivedProducts).FindOneAndUpdate(ctx, bson.M{"user_id": userID},
		bson.M{"$addToSet": bson.M{"archived_product_ids": productID}, "$set": bson.M{"updated_at": now}, "$setOnInsert": bson.M{"createdAt": now}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&doc)
	if err != nil {
		return nil, err
	}
	if doc.ArchivedProductIDs == nil {
		doc.ArchivedProductIDs = []string{}
	}
	return doc.ArchivedProductIDs, nil
}

// RemoveArchivedProduct mirrors findOneAndUpdate({user_id}, {$pull, $set}, {new}) with no upsert.
func (s *Store) RemoveArchivedProduct(ctx context.Context, userID, productID string, now time.Time) ([]string, error) {
	var doc models.ArchivedProducts
	err := s.C(CollArchivedProducts).FindOneAndUpdate(ctx, bson.M{"user_id": userID},
		bson.M{"$pull": bson.M{"archived_product_ids": productID}, "$set": bson.M{"updated_at": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
	if err != nil {
		if none, err := noDoc(err); none {
			return []string{}, nil
		} else if err != nil {
			return nil, err
		}
	}
	if doc.ArchivedProductIDs == nil {
		doc.ArchivedProductIDs = []string{}
	}
	return doc.ArchivedProductIDs, nil
}
