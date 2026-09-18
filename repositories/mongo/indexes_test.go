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
	mk := func(remark string) *models.RewardTransaction {
		return &models.RewardTransaction{UserID: "u1", StreakMasterID: primitive.NewObjectID(), CreditCoins: 100,
			IsCreditTransaction: true, CreditRemarks: remark, Status: "success"}
	}
	_, err := s.InsertRewardTransaction(ctx, mk("bah-legacy-first-log"))
	require.NoError(t, err)
	_, err = s.InsertRewardTransaction(ctx, mk("bah-legacy-first-log"))
	assert.True(t, IsDuplicateKey(err), "second idempotent remark must collide")
	_, err = s.InsertRewardTransaction(ctx, mk("Manual grant"))
	require.NoError(t, err)
	_, err = s.InsertRewardTransaction(ctx, mk("Manual grant"))
	require.NoError(t, err, "human remarks are not unique")
}
