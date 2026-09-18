package mongorepo

import (
	"context"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// Balance is the result of the active-coin aggregation.
type Balance struct {
	BalanceCoins   float64
	EarliestExpiry *time.Time
}

// ActiveCoinBalance mirrors the api-server aggregation:
// match {user_id, status:'success', expire_at:{$gt:now}, all_coins_used:false} → Σcredit − Σdebit (+ min expire_at).
func (s *Store) ActiveCoinBalance(ctx context.Context, userID string, now time.Time) (Balance, error) {
	pipeline := bson.A{
		bson.M{"$match": bson.M{"user_id": userID, "status": "success", "expire_at": bson.M{"$gt": now}, "all_coins_used": false}},
		bson.M{"$group": bson.M{"_id": nil, "totalCredits": bson.M{"$sum": "$credit_coins"}, "totalDebits": bson.M{"$sum": "$total_debit_coins"}, "expireAt": bson.M{"$min": "$expire_at"}}},
		bson.M{"$project": bson.M{"_id": 0, "balanceCoins": bson.M{"$subtract": bson.A{"$totalCredits", "$totalDebits"}}, "expireAt": 1}},
	}
	cur, err := s.C(CollRewardTransactions).Aggregate(ctx, pipeline)
	if err != nil {
		return Balance{}, err
	}
	var rows []struct {
		BalanceCoins float64    `bson:"balanceCoins"`
		ExpireAt     *time.Time `bson:"expireAt"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return Balance{}, err
	}
	if len(rows) == 0 {
		return Balance{}, nil
	}
	return Balance{BalanceCoins: rows[0].BalanceCoins, EarliestExpiry: rows[0].ExpireAt}, nil
}

// InsertRewardTransaction inserts doc; callers check IsDuplicateKey for idempotent remarks.
func (s *Store) InsertRewardTransaction(ctx context.Context, doc *models.RewardTransaction) (*models.RewardTransaction, error) {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	if doc.DebitTransactions == nil {
		doc.DebitTransactions = []models.DebitTransaction{}
	}
	res, err := s.C(CollRewardTransactions).InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc.ID = res.InsertedID.(primitive.ObjectID)
	return doc, nil
}

// FindCreditTransactionByRemarks finds a successful credit with the given remarks.
func (s *Store) FindCreditTransactionByRemarks(ctx context.Context, userID, remarks string) (*models.RewardTransaction, error) {
	var out models.RewardTransaction
	err := s.C(CollRewardTransactions).FindOne(ctx, bson.M{"user_id": userID, "credit_remarks": remarks, "is_credit_transaction": true, "status": "success"}).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// ExistsCreditTransactionWithRemarks reports whether FindCreditTransactionByRemarks would match.
func (s *Store) ExistsCreditTransactionWithRemarks(ctx context.Context, userID, remarks string) (bool, error) {
	doc, err := s.FindCreditTransactionByRemarks(ctx, userID, remarks)
	return doc != nil, err
}

// FindRewardTransactionsByUser lists a user's reward transactions.
func (s *Store) FindRewardTransactionsByUser(ctx context.Context, userID string, newestFirst bool, projection bson.M) ([]models.RewardTransaction, error) {
	dir := 1
	if newestFirst {
		dir = -1
	}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: dir}})
	if projection != nil {
		opts.SetProjection(projection)
	}
	cur, err := s.C(CollRewardTransactions).Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	var out []models.RewardTransaction
	return out, cur.All(ctx, &out)
}

// ExistsRewardTxnForMasters mirrors `findOne({user_id, $or:[{streak_master_id: a},{streak_master_id: b}]})`.
func (s *Store) ExistsRewardTxnForMasters(ctx context.Context, userID string, masterIDs []primitive.ObjectID) (bool, error) {
	n, err := s.C(CollRewardTransactions).CountDocuments(ctx, bson.M{"user_id": userID, "streak_master_id": bson.M{"$in": masterIDs}}, options.Count().SetLimit(1))
	return n > 0, err
}

// EarliestExpiringUnusedCredit mirrors getEarliestExpiringUnusedCoins.
func (s *Store) EarliestExpiringUnusedCredit(ctx context.Context, userID string, dayStart time.Time) (*models.RewardTransaction, error) {
	var out models.RewardTransaction
	err := s.C(CollRewardTransactions).FindOne(ctx, bson.M{
		"user_id": userID, "is_credit_transaction": true, "status": "success",
		"expire_at": bson.M{"$gte": dayStart},
		"$expr":     bson.M{"$gt": bson.A{"$credit_coins", "$total_debit_coins"}},
	}, options.FindOne().SetSort(bson.D{{Key: "expire_at", Value: 1}})).Decode(&out)
	if err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// RewardTxnsCreatedBetween returns createdAt of successful transactions in [from, to].
func (s *Store) RewardTxnsCreatedBetween(ctx context.Context, userID string, from, to time.Time) ([]time.Time, error) {
	cur, err := s.C(CollRewardTransactions).Find(ctx, bson.M{"user_id": userID, "status": "success", "createdAt": bson.M{"$gte": from, "$lte": to}},
		options.Find().SetProjection(bson.M{"createdAt": 1}))
	if err != nil {
		return nil, err
	}
	var rows []models.RewardTransaction
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.CreatedAt)
	}
	return out, nil
}

// FIFOOpenCredits lists open credits (success, unexpired, not fully used) in FIFO order.
// sortByExpiry=false → createdAt asc (app-backend); true → expire_at asc (consumer-backend).
func (s *Store) FIFOOpenCredits(ctx context.Context, userID string, now time.Time, sortByExpiry bool) ([]models.RewardTransaction, error) {
	key := "createdAt"
	if sortByExpiry {
		key = "expire_at"
	}
	cur, err := s.C(CollRewardTransactions).Find(ctx, bson.M{"user_id": userID, "status": "success", "expire_at": bson.M{"$gt": now}, "all_coins_used": false},
		options.Find().SetSort(bson.D{{Key: key, Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.RewardTransaction
	return out, cur.All(ctx, &out)
}

// PushDebit appends a debit sub-document and updates the totals.
func (s *Store) PushDebit(ctx context.Context, txnID primitive.ObjectID, d models.DebitTransaction, allUsed bool, totalDebit int) error {
	_, err := s.C(CollRewardTransactions).UpdateOne(ctx, bson.M{"_id": txnID}, bson.M{
		"$push": bson.M{"debit_transactions": d},
		"$set":  bson.M{"is_debit_transaction": true, "all_coins_used": allUsed, "total_debit_coins": totalDebit, "updatedAt": s.Clock.Now()},
	})
	return err
}

// RewardTxnExpiriesByIDs maps transaction ids to expire_at.
func (s *Store) RewardTxnExpiriesByIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]*time.Time, error) {
	out := map[primitive.ObjectID]*time.Time{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.C(CollRewardTransactions).Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"expire_at": 1}))
	if err != nil {
		return nil, err
	}
	var rows []models.RewardTransaction
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.ExpireAt
	}
	return out, nil
}

// CoinTxnRow is one row of GET /coinTransaction.
type CoinTxnRow struct {
	TxnCoinAmount int       `json:"txnCoinAmount"`
	TxnType       string    `json:"txnType"`
	TxnStatus     string    `json:"txnStatus"`
	Remarks       string    `json:"remarks"`
	ExpiryDate    any       `json:"expiryDate"`
	TxnDate       time.Time `json:"txnDate"`
	ShowCoins     bool      `json:"showCoins"`
	Text          string    `json:"text"`
	StreakName    string    `json:"streakName"`
}

// CoinTransactionPage mirrors getRewardCoinHistoryPaginated: credits (with a $lookup for the streak
// display name), debits, and synthesised Expired rows, sorted by txnDate desc and sliced.
func (s *Store) CoinTransactionPage(ctx context.Context, userID string, now time.Time, page, limit int) ([]CoinTxnRow, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 6
	}
	pipeline := bson.A{
		bson.M{"$match": bson.M{"user_id": userID}},
		bson.M{"$lookup": bson.M{"from": CollStreakMasters, "localField": "streak_master_id", "foreignField": "_id", "as": "sm"}},
		bson.M{"$addFields": bson.M{"streakName": bson.M{"$ifNull": bson.A{bson.M{"$arrayElemAt": bson.A{"$sm.display_name", 0}}, ""}}}},
		bson.M{"$project": bson.M{"credit_coins": 1, "total_debit_coins": 1, "status": 1, "credit_remarks": 1, "expire_at": 1, "createdAt": 1, "all_coins_used": 1, "streakName": 1}},
	}
	cur, err := s.C(CollRewardTransactions).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	var credits []struct {
		CreditCoins     int        `bson:"credit_coins"`
		TotalDebitCoins int        `bson:"total_debit_coins"`
		Status          string     `bson:"status"`
		CreditRemarks   string     `bson:"credit_remarks"`
		ExpireAt        *time.Time `bson:"expire_at"`
		CreatedAt       time.Time  `bson:"createdAt"`
		AllCoinsUsed    bool       `bson:"all_coins_used"`
		StreakName      string     `bson:"streakName"`
	}
	if err := cur.All(ctx, &credits); err != nil {
		return nil, 0, err
	}
	debits, err := s.FindRedeemTransactionsByUser(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]CoinTxnRow, 0, len(credits)*2+len(debits))
	for _, d := range debits {
		text := ""
		if d.Status == "failure" {
			text = itoa(d.RedeemedCoins) + " coins redeemed on this order added back to wallet"
		}
		rows = append(rows, CoinTxnRow{TxnCoinAmount: d.RedeemedCoins, TxnType: "Debit", TxnStatus: d.Status, Remarks: d.Remarks,
			ExpiryDate: "", TxnDate: d.CreatedAt, ShowCoins: d.Status != "failure", Text: text, StreakName: ""})
	}
	for _, c := range credits {
		var expiry any = ""
		if c.ExpireAt != nil {
			expiry = *c.ExpireAt
		}
		rows = append(rows, CoinTxnRow{TxnCoinAmount: c.CreditCoins, TxnType: "Credit", TxnStatus: c.Status, Remarks: c.CreditRemarks,
			ExpiryDate: expiry, TxnDate: c.CreatedAt, ShowCoins: true, Text: "", StreakName: c.StreakName})
		if c.ExpireAt != nil && c.ExpireAt.Before(now) && !c.AllCoinsUsed && c.Status != "failure" {
			rows = append(rows, CoinTxnRow{TxnCoinAmount: c.CreditCoins - c.TotalDebitCoins, TxnType: "Expired", TxnStatus: "expired", Remarks: "Coins Expired",
				ExpiryDate: *c.ExpireAt, TxnDate: *c.ExpireAt, ShowCoins: true, Text: "", StreakName: c.StreakName})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].TxnDate.After(rows[j].TxnDate) })
	total := len(rows)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return rows[start:end], total, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
