package mongorepo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
)

func testStore(t *testing.T) *Store {
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := client.Database("bah_test_" + primitive.NewObjectID().Hex()[18:])
	t.Cleanup(func() { _ = db.Drop(context.Background()); _ = client.Disconnect(context.Background()) })
	require.NoError(t, EnsureIndexes(context.Background(), db))
	return NewStore(db, common.RealClock{})
}

func TestEnsureIndexes_IdempotencyPartialIndex(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	require.NoError(t, EnsureIndexes(ctx, s.DB)) // second run is a no-op
	mk := func(remark string, key *string) *models.RewardTransaction {
		return &models.RewardTransaction{UserID: "u1", StreakMasterID: primitive.NewObjectID(), CreditCoins: 100,
			IsCreditTransaction: true, CreditRemarks: remark, IdempotencyKey: key, Status: "success"}
	}
	firstLog := "bah-legacy-first-log"
	_, err := s.InsertRewardTransaction(ctx, mk(firstLog, &firstLog))
	require.NoError(t, err)
	_, err = s.InsertRewardTransaction(ctx, mk(firstLog, &firstLog))
	assert.True(t, IsDuplicateKey(err), "a repeated idempotency key must collide")

	// A credit with no key (a manual CRM grant) may repeat, even with identical remarks.
	_, err = s.InsertRewardTransaction(ctx, mk("Manual grant", nil))
	require.NoError(t, err)
	_, err = s.InsertRewardTransaction(ctx, mk("Manual grant", nil))
	require.NoError(t, err, "unkeyed credits are not unique")

	// The key is scoped per user.
	other := &models.RewardTransaction{UserID: "u2", StreakMasterID: primitive.NewObjectID(), CreditCoins: 100,
		IsCreditTransaction: true, CreditRemarks: firstLog, IdempotencyKey: &firstLog, Status: "success"}
	_, err = s.InsertRewardTransaction(ctx, other)
	require.NoError(t, err, "another user may hold the same key")
}
