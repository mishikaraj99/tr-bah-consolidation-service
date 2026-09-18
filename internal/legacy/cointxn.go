package legacy

import (
	"context"
	"math"
	"strconv"

	mongorepo "traya-bah-service/repositories/mongo"
)

// Pagination is the /coinTransaction pagination block.
type Pagination struct {
	Page        int  `json:"page"`
	Limit       int  `json:"limit"`
	Total       int  `json:"total"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
	HasPrevPage bool `json:"hasPrevPage"`
}

// CoinTransactionPage is the GET /coinTransaction response.
type CoinTransactionPage struct {
	RewardHistory []mongorepo.CoinTxnRow `json:"rewardHistory"`
	Pagination    Pagination             `json:"pagination"`
}

// GetRewardCoinHistoryPaginated ports handler.js getRewardCoinHistoryPaginated.
func (s *Service) GetRewardCoinHistoryPaginated(ctx context.Context, userID string, page, limit int) (*CoinTransactionPage, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 6
	}
	rows, total, err := s.Store.CoinTransactionPage(ctx, userID, s.now(), page, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []mongorepo.CoinTxnRow{}
	}
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	return &CoinTransactionPage{RewardHistory: rows, Pagination: Pagination{
		Page: page, Limit: limit, Total: total, TotalPages: totalPages,
		HasNextPage: page < totalPages, HasPrevPage: page > 1,
	}}, nil
}

// RewardCoinHistory is the CRM coin history payload (no streakName, no synthesised Expired rows).
type RewardCoinHistory struct {
	RewardHistory []map[string]any `json:"rewardHistory"`
	Text1         string           `json:"text1"`
	Text2         string           `json:"text2"`
}

// GetRewardCoinHistory ports handler.js getRewardCoinHistory.
func (s *Service) GetRewardCoinHistory(ctx context.Context, userID string, isCredit, isDebit bool) (*RewardCoinHistory, error) {
	out := &RewardCoinHistory{RewardHistory: []map[string]any{}, Text1: MsgRewardHistoryText1}
	if isCredit {
		txns, err := s.Store.FindRewardTransactionsByUser(ctx, userID, true, nil)
		if err != nil {
			return nil, err
		}
		for _, t := range txns {
			var expiry any = ""
			if t.ExpireAt != nil {
				expiry = *t.ExpireAt
			}
			out.RewardHistory = append(out.RewardHistory, map[string]any{
				"txnCoinAmount": t.CreditCoins, "txnType": "Credit", "txnStatus": t.Status,
				"remarks": t.CreditRemarks, "expiryDate": expiry, "txnDate": t.CreatedAt,
				"showCoins": true, "text": "",
			})
		}
	}
	if isDebit {
		redeems, err := s.Store.FindRedeemTransactionsByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, r := range redeems {
			text := ""
			if r.Status == "failure" {
				text = strconv.Itoa(r.RedeemedCoins) + " coins redeemed on this order added back to wallet"
			}
			out.RewardHistory = append(out.RewardHistory, map[string]any{
				"txnCoinAmount": r.RedeemedCoins, "txnType": "Debit", "txnStatus": r.Status,
				"remarks": r.Remarks, "expiryDate": "", "txnDate": r.CreatedAt,
				"showCoins": r.Status != "failure", "text": text,
			})
		}
	}
	sortByTxnDateDesc(out.RewardHistory)
	return out, nil
}
