package habit

import (
	"context"
	"fmt"
	"math"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

// RedeemInput is POST /bah/coin/redeem's payload.
type RedeemInput struct {
	RedeemedAmount float64 `json:"redeemedAmount"`
	UserID         string  `json:"userId"`
	Remarks        string  `json:"remarks"`
	ShopFloTxnID   string  `json:"shopFloTxnId"`
	CaseID         string  `json:"caseId"`
	OrderID        string  `json:"orderId"`
	OrderDisplayID string  `json:"orderDisplayId"`
	Currency       string  `json:"currency"`
	IsJusPay       *bool   `json:"isJusPay"`
}

// RedeemCoinsNonOrder ports redeemCoinService.saveTheRedeemRewardTransactionForNonOrderTxn.
// Unlike the JS, failures are returned to the caller instead of being swallowed into a 200 (spec §6.1.9).
func (s *Service) RedeemCoinsNonOrder(ctx context.Context, in RedeemInput) (map[string]string, error) {
	userID := in.UserID
	phone := ""
	if userID == "" && in.CaseID != "" {
		uc, err := s.PG.UserCaseByCaseID(ctx, in.CaseID)
		if err != nil {
			return nil, err
		}
		if uc == nil || uc.UserID == "" {
			return nil, common.BadRequest(MsgUserNotFound)
		}
		userID, phone = uc.UserID, uc.PhoneNumber
	} else if userID != "" {
		uc, err := s.PG.UserCaseByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if uc == nil || uc.UserID == "" {
			return nil, common.BadRequest(MsgUserNotFound)
		}
		phone = uc.PhoneNumber
	}
	if userID == "" {
		return nil, common.BadRequest(MsgUserNotFound)
	}
	remarks := in.Remarks
	if remarks == "" {
		remarks = "Coin redeemed - " + in.ShopFloTxnID
	}
	currency := in.Currency
	if currency == "" {
		currency = "INR"
	}
	isJusPay := true
	if in.IsJusPay != nil {
		isJusPay = *in.IsJusPay
	}
	return s.saveRedeemTransaction(ctx, redeemArgs{
		UserID: userID, Phone: phone, RedeemedAmount: in.RedeemedAmount, Remarks: remarks,
		ShopFloTxnID: in.ShopFloTxnID, OrderID: in.OrderID, OrderDisplayID: in.OrderDisplayID,
		Currency: currency, IsJusPay: isJusPay,
	})
}

type redeemArgs struct {
	UserID, Phone                  string
	RedeemedAmount                 float64
	Remarks, ShopFloTxnID, OrderID string
	OrderDisplayID, Currency       string
	IsJusPay                       bool
}

// saveRedeemTransaction ports saveTheRedeemRewardTransaction (FIFO drain + Shopflo debit).
func (s *Service) saveRedeemTransaction(ctx context.Context, a redeemArgs) (map[string]string, error) {
	now := s.now()
	redeemedCoins := int(math.Round(a.RedeemedAmount * CoinToRupeeDivisor))
	credits, err := s.Store.FIFOOpenCredits(ctx, a.UserID, now, false)
	if err != nil {
		return nil, err
	}
	balance := 0
	for _, c := range credits {
		balance += c.CreditCoins - c.TotalDebitCoins
	}
	if balance < redeemedCoins {
		return nil, common.BadRequest(MsgNotEnoughCoins)
	}
	if a.OrderID != "" {
		dup, err := s.Store.ExistsRedemptionForOrder(ctx, a.UserID, a.OrderID)
		if err != nil {
			return nil, err
		}
		if dup {
			return nil, common.BadRequest(MsgOrderAlreadyRedeemed)
		}
	} else if a.ShopFloTxnID != "" {
		dup, err := s.Store.ExistsRedemptionForShopfloTxn(ctx, a.UserID, a.ShopFloTxnID)
		if err != nil {
			return nil, err
		}
		if dup {
			return nil, common.BadRequest(MsgRewardAlreadyRedeemed)
		}
	}

	doc := &models.RedeemTransaction{
		UserID: a.UserID, RedeemedCoins: redeemedCoins, RedeemedAmount: a.RedeemedAmount,
		Currency: a.Currency, Status: "success", Remarks: a.Remarks,
	}
	if a.OrderID != "" {
		doc.OrderID = &a.OrderID
	}
	if a.OrderDisplayID != "" {
		doc.OrderDisplayID = &a.OrderDisplayID
	}
	if a.ShopFloTxnID != "" {
		doc.ShopFloTxnID = &a.ShopFloTxnID
	}
	redeemTxn, err := s.Store.InsertRedeemTransaction(ctx, doc)
	if err != nil {
		if isDuplicate(err) {
			if a.OrderID != "" {
				return nil, common.BadRequest(MsgOrderAlreadyRedeemed)
			}
			return nil, common.BadRequest(MsgRewardAlreadyRedeemed)
		}
		return nil, err
	}

	remaining := redeemedCoins
	for _, c := range credits {
		if remaining <= 0 {
			break
		}
		usable := c.CreditCoins - c.TotalDebitCoins
		totalDebit := c.TotalDebitCoins
		allUsed := false
		if remaining >= usable {
			remaining -= usable
			totalDebit += usable
			allUsed = true
		} else {
			totalDebit += remaining
			usable = remaining
			remaining = 0
		}
		if err := s.Store.PushDebit(ctx, c.ID, models.DebitTransaction{
			DebitTransactionID: redeemTxn.ID, DebitCoins: usable, TransactionStatus: redeemTxn.Status,
			DebitRemarks: redeemTxn.Remarks, DebitTransactionDate: redeemTxn.CreatedAt,
		}, allUsed, totalDebit); err != nil {
			return nil, err
		}
	}

	if a.IsJusPay && s.Shopflo != nil && a.Phone != "" {
		reference := a.OrderDisplayID
		if reference == "" {
			reference = a.ShopFloTxnID
		}
		if err := s.Shopflo.Debit(ctx, redeemedCoins, redeemTxn.ID.Hex(), a.Phone, reference); err != nil {
			s.Log.Warn("shopflo debit failed", "userId", a.UserID, "error", err.Error())
		}
	}
	return map[string]string{"message": MsgRedeemSuccess}, nil
}

func isDuplicate(err error) bool { return mongorepo.IsDuplicateKey(err) }

var _ = fmt.Sprintf
var _ = time.Now
