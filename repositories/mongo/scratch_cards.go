package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// CreateScratchCard inserts a card; callers check IsDuplicateKey.
func (s *Store) CreateScratchCard(ctx context.Context, doc *models.ScratchCard) (*models.ScratchCard, error) {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	res, err := s.C(CollScratchCards).InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc.ID = res.InsertedID.(primitive.ObjectID)
	return doc, nil
}

func (s *Store) findCard(ctx context.Context, f bson.M, opts ...*options.FindOneOptions) (*models.ScratchCard, error) {
	var out models.ScratchCard
	if err := s.C(CollScratchCards).FindOne(ctx, f, opts...).Decode(&out); err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// FindMintedCardForRewardDay finds the card minted for a reward day.
func (s *Store) FindMintedCardForRewardDay(ctx context.Context, userID, source, day string) (*models.ScratchCard, error) {
	return s.findCard(ctx, bson.M{"user_id": userID, "source": source, "reward_day_date": day})
}

// FindScratchCardsForUser lists cards newest first.
func (s *Store) FindScratchCardsForUser(ctx context.Context, userID string) ([]models.ScratchCard, error) {
	cur, err := s.C(CollScratchCards).Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.ScratchCard
	return out, cur.All(ctx, &out)
}

// FindCardByIDForUser finds a card by id scoped to the user.
func (s *Store) FindCardByIDForUser(ctx context.Context, id primitive.ObjectID, userID string) (*models.ScratchCard, error) {
	return s.findCard(ctx, bson.M{"_id": id, "user_id": userID})
}

// ClaimActiveCard is the credit lock: flips an active, unexpired card to claimed. nil when not matched.
func (s *Store) ClaimActiveCard(ctx context.Context, id primitive.ObjectID, userID string, now time.Time) (*models.ScratchCard, error) {
	var out models.ScratchCard
	err := s.C(CollScratchCards).FindOneAndUpdate(ctx,
		bson.M{"_id": id, "user_id": userID, "status": "active", "expires_at": bson.M{"$gt": now}},
		bson.M{"$set": bson.M{"status": "claimed", "claimed_at": now, "updatedAt": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// LinkRewardTransaction sets reward_transaction_ref.
func (s *Store) LinkRewardTransaction(ctx context.Context, cardID, txnID primitive.ObjectID) error {
	_, err := s.C(CollScratchCards).UpdateOne(ctx, bson.M{"_id": cardID}, bson.M{"$set": bson.M{"reward_transaction_ref": txnID, "updatedAt": s.Clock.Now()}})
	return err
}

// ReopenClaimedCard reverts a claim after a failed credit.
func (s *Store) ReopenClaimedCard(ctx context.Context, cardID primitive.ObjectID) error {
	_, err := s.C(CollScratchCards).UpdateOne(ctx, bson.M{"_id": cardID}, bson.M{"$set": bson.M{"status": "active", "claimed_at": nil, "updatedAt": s.Clock.Now()}})
	return err
}
