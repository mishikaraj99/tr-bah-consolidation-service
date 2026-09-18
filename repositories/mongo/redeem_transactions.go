package mongorepo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// InsertRedeemTransaction inserts a redemption; unique partial indexes guard duplicates.
func (s *Store) InsertRedeemTransaction(ctx context.Context, doc *models.RedeemTransaction) (*models.RedeemTransaction, error) {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	res, err := s.C(CollRedeemTransactions).InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc.ID = res.InsertedID.(primitive.ObjectID)
	return doc, nil
}

// FindRedeemTransactionsByUser lists redemptions newest first.
func (s *Store) FindRedeemTransactionsByUser(ctx context.Context, userID string) ([]models.RedeemTransaction, error) {
	cur, err := s.C(CollRedeemTransactions).Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.RedeemTransaction
	return out, cur.All(ctx, &out)
}

// ExistsRedemptionForOrder reports a successful redemption for the order.
func (s *Store) ExistsRedemptionForOrder(ctx context.Context, userID, orderID string) (bool, error) {
	n, err := s.C(CollRedeemTransactions).CountDocuments(ctx, bson.M{"user_id": userID, "order_id": orderID, "status": "success"}, options.Count().SetLimit(1))
	return n > 0, err
}

// ExistsRedemptionForShopfloTxn reports a redemption for the Shopflo transaction id.
func (s *Store) ExistsRedemptionForShopfloTxn(ctx context.Context, userID, txn string) (bool, error) {
	n, err := s.C(CollRedeemTransactions).CountDocuments(ctx, bson.M{"user_id": userID, "shop_flo_txn_id": txn}, options.Count().SetLimit(1))
	return n > 0, err
}
