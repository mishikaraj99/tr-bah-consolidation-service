package mongorepo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"traya-bah-service/models"
)

func TestActiveCoinBalance_And_CoinTransactionPage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	master, err := s.UpsertStreakMasterBySlug(ctx, "first-checkin-extra-reward", &models.StreakMaster{DisplayName: "First Log", Days: 0, IsActive: true, RewardCoins: 100})
	require.NoError(t, err)
	future := now.Add(48 * time.Hour)
	past := now.Add(-48 * time.Hour)
	mk := func(coins, debit int, exp time.Time, used bool, remark string, created time.Time) {
		doc := &models.RewardTransaction{UserID: "u1", StreakMasterID: master.ID, CreditCoins: coins, TotalDebitCoins: debit, IsCreditTransaction: true,
			CreditRemarks: remark, Status: "success", ExpireAt: &exp, AllCoinsUsed: used}
		_, err := s.InsertRewardTransaction(ctx, doc)
		require.NoError(t, err)
		_, err = s.C(CollRewardTransactions).UpdateByID(ctx, doc.ID, map[string]any{"$set": map[string]any{"createdAt": created}})
		require.NoError(t, err)
	}
	mk(100, 30, future, false, "A", now.Add(-3*time.Hour)) // active: 70
	mk(400, 0, past, false, "B", now.Add(-72*time.Hour))   // expired unused → Expired row 400
	mk(50, 50, future, true, "C", now.Add(-2*time.Hour))   // fully used → excluded
	orphan := &models.RewardTransaction{UserID: "u1", StreakMasterID: primitive.NewObjectID(), CreditCoins: 10, IsCreditTransaction: true, Status: "success", ExpireAt: &future}
	_, err = s.InsertRewardTransaction(ctx, orphan)
	require.NoError(t, err)
	oid := "o1"
	_, err = s.InsertRedeemTransaction(ctx, &models.RedeemTransaction{UserID: "u1", RedeemedCoins: 30, RedeemedAmount: 3, OrderID: &oid, Status: "failure", Remarks: "r"})
	require.NoError(t, err)

	bal, err := s.ActiveCoinBalance(ctx, "u1", now)
	require.NoError(t, err)
	assert.Equal(t, float64(80), bal.BalanceCoins) // 70 + orphan 10
	require.NotNil(t, bal.EarliestExpiry)

	rows, total, err := s.CoinTransactionPage(ctx, "u1", now, 1, 6)
	require.NoError(t, err)
	assert.Equal(t, 6, total) // 4 credits + 1 expired + 1 debit
	assert.Len(t, rows, 6)
	var expired, debit, orphanRow *CoinTxnRow
	for i := range rows {
		switch {
		case rows[i].TxnType == "Expired":
			expired = &rows[i]
		case rows[i].TxnType == "Debit":
			debit = &rows[i]
		case rows[i].TxnCoinAmount == 10:
			orphanRow = &rows[i]
		}
	}
	require.NotNil(t, expired)
	assert.Equal(t, 400, expired.TxnCoinAmount)
	assert.Equal(t, "Coins Expired", expired.Remarks)
	assert.Equal(t, "expired", expired.TxnStatus)
	require.NotNil(t, debit)
	assert.False(t, debit.ShowCoins)
	assert.Equal(t, "30 coins redeemed on this order added back to wallet", debit.Text)
	require.NotNil(t, orphanRow)
	assert.Equal(t, "", orphanRow.StreakName)
	for i := 1; i < len(rows); i++ {
		assert.False(t, rows[i].TxnDate.After(rows[i-1].TxnDate), "sorted desc")
	}
	page2, _, err := s.CoinTransactionPage(ctx, "u1", now, 2, 4)
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	fifo, err := s.FIFOOpenCredits(ctx, "u1", now, false)
	require.NoError(t, err)
	require.Len(t, fifo, 2)
	assert.Equal(t, 100, fifo[0].CreditCoins, "oldest createdAt first")
}

func TestClaimActiveCard_Race(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	card, err := s.CreateScratchCard(ctx, &models.ScratchCard{UserID: "u1", Source: "habit_tracker", RewardType: "coins", RewardValue: 100,
		RewardDayDate: "2026-09-18", Status: "active", ExpiresAt: now.Add(time.Hour)})
	require.NoError(t, err)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := s.ClaimActiveCard(ctx, card.ID, "u1", now)
			require.NoError(t, err)
			if c != nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, wins)
	dup, err := s.CreateScratchCard(ctx, &models.ScratchCard{UserID: "u1", Source: "habit_tracker", RewardDayDate: "2026-09-18", Status: "active", ExpiresAt: now})
	assert.Nil(t, dup)
	assert.True(t, IsDuplicateKey(err))
}

func TestLifelineLog_Duplicate_And_Archived(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	d := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	created, err := s.CreateLifelineLog(ctx, &models.LifelineLog{UserID: "u1", DateCovered: d, KitStart: d, AppliedAt: now})
	require.NoError(t, err)
	assert.True(t, created)
	created, err = s.CreateLifelineLog(ctx, &models.LifelineLog{UserID: "u1", DateCovered: d, KitStart: d, AppliedAt: now})
	require.NoError(t, err)
	assert.False(t, created)

	ids, err := s.AddArchivedProduct(ctx, "u1", "p1", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"p1"}, ids)
	ids, err = s.AddArchivedProduct(ctx, "u1", "p1", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"p1"}, ids)
	ids, err = s.RemoveArchivedProduct(ctx, "u1", "p1", now)
	require.NoError(t, err)
	assert.Equal(t, []string{}, ids)
	ids, err = s.RemoveArchivedProduct(ctx, "nobody", "p1", now)
	require.NoError(t, err)
	assert.Equal(t, []string{}, ids)
}
