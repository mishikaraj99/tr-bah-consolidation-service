package habit

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/legacy"
	"traya-bah-service/models"
	mongorepo "traya-bah-service/repositories/mongo"
)

// Scratch card assets (scratchCardService.SCRATCH_CARD_ASSETS).
func scratchCoverAsset() string  { return Assets["rewardUpcoming"] }
func scratchRevealAsset() string { return Assets["rewardReceived"] }
func noRewardScreen() string     { return Assets["noRewardScreen"] }

// CardReward is the reward block of a card view.
type CardReward struct {
	Type        string `json:"type"`
	Value       int    `json:"value"`
	DisplayText string `json:"displayText,omitempty"`
}

// ActiveCardView is toActiveCardView's shape.
type ActiveCardView struct {
	ID          primitive.ObjectID `json:"id"`
	Reward      CardReward         `json:"reward"`
	CoverAsset  string             `json:"coverAsset"`
	RevealAsset string             `json:"revealAsset"`
	ExpiresAt   time.Time          `json:"expires_at"`
	Expired     bool               `json:"expired"`
	Status      string             `json:"status"`
}

func toActiveCardView(c *models.ScratchCard) ActiveCardView {
	return ActiveCardView{
		ID:         c.ID,
		Reward:     CardReward{Type: c.RewardType, Value: c.RewardValue, DisplayText: CoinsDisplayText},
		CoverAsset: scratchCoverAsset(), RevealAsset: scratchRevealAsset(),
		ExpiresAt: c.ExpiresAt, Expired: false, Status: c.Status,
	}
}

// MintHabitTrackerScratchCard mirrors scratchCardService.mintHabitTrackerScratchCard.
func (s *Service) MintHabitTrackerScratchCard(ctx context.Context, in legacy.HabitCreditInput) (legacy.HabitCreditResult, error) {
	reward := ResolveReward(in.StreakDay)
	if reward == nil {
		return legacy.HabitCreditResult{Success: true, Minted: boolPtr(false), Paused: boolPtr(true)}, nil
	}
	dateStr := common.FormatMoment(in.CheckInDate.UTC(), "YYYY-MM-DD")
	expiresAt := in.CheckInDate.UTC().AddDate(0, 0, ScratchCardWindowDays)
	remarks := RemarksFor(*reward, in.CheckInDate)

	credited, err := s.Store.ExistsCreditTransactionWithRemarks(ctx, in.UserID, remarks)
	if err != nil {
		return legacy.HabitCreditResult{}, err
	}
	if credited {
		return legacy.HabitCreditResult{Success: true, Minted: boolPtr(false), AlreadyCredited: boolPtr(true)}, nil
	}
	card, err := s.Store.CreateScratchCard(ctx, &models.ScratchCard{
		UserID: in.UserID, Source: ScratchSource, RewardType: "coins", RewardValue: reward.Coins,
		RewardMeta:    models.ScratchCardMeta{Slug: reward.Slug, StreakDay: in.StreakDay, Tier: reward.Type},
		RewardDayDate: dateStr, PhoneNumber: in.PhoneNumber, Status: "active", ExpiresAt: expiresAt, CreditRemarks: remarks,
	})
	if err != nil {
		if !mongorepo.IsDuplicateKey(err) {
			return legacy.HabitCreditResult{}, err
		}
		existing, ferr := s.Store.FindMintedCardForRewardDay(ctx, in.UserID, ScratchSource, dateStr)
		if ferr != nil {
			return legacy.HabitCreditResult{}, ferr
		}
		out := legacy.HabitCreditResult{Success: true, Minted: boolPtr(false), AlreadyExists: boolPtr(true)}
		if existing != nil && existing.Status == "active" && existing.ExpiresAt.After(s.now()) {
			out.Card = toActiveCardView(existing)
		}
		return out, nil
	}
	return legacy.HabitCreditResult{Success: true, Minted: boolPtr(true), CardID: card.ID.Hex(),
		Coins: intPtr(reward.Coins), Type: reward.Type, Card: toActiveCardView(card)}, nil
}

// CoinExpiryFields mirrors scratchCardService.coinExpiryFields (note the D MMMM vs D MMM asymmetry).
func CoinExpiryFields(expiresAt *time.Time, now time.Time) map[string]any {
	if expiresAt == nil || expiresAt.IsZero() {
		return map[string]any{}
	}
	expired := !expiresAt.After(now)
	label := "Expires on " + common.FormatMoment(*expiresAt, "D MMM, YYYY")
	if expired {
		label = "Expired on " + common.FormatMoment(*expiresAt, "D MMMM, YYYY")
	}
	return map[string]any{"coinExpiresAt": *expiresAt, "coinExpired": expired, "coinExpiryLabel": label}
}

// ScratchCardsResponse is GET /bah/scratch-card.
type ScratchCardsResponse struct {
	Active         []ActiveCardView `json:"active"`
	Expired        []map[string]any `json:"expired"`
	History        []map[string]any `json:"history"`
	EmptyTitle     string           `json:"emptyTitle"`
	EmptySubtitle  string           `json:"emptySubtitle"`
	Title          string           `json:"title"`
	NoRewardScreen string           `json:"noRewardScreen"`
}

// GetScratchCards mirrors scratchCardService.getScratchCards.
func (s *Service) GetScratchCards(ctx context.Context, userID string) (*ScratchCardsResponse, error) {
	now := s.now()
	cards, err := s.Store.FindScratchCardsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var refs []primitive.ObjectID
	for _, c := range cards {
		if c.Status == "claimed" && c.RewardType == "coins" && c.RewardTransactionRef != nil {
			refs = append(refs, *c.RewardTransactionRef)
		}
	}
	expiries := map[primitive.ObjectID]*time.Time{}
	if len(refs) > 0 {
		expiries, err = s.Store.RewardTxnExpiriesByIDs(ctx, refs)
		if err != nil {
			s.Log.Error("getScratchCards: failed to load reward transactions for coin expiry", "userId", userID, "error", err.Error())
			expiries = map[primitive.ObjectID]*time.Time{}
		}
	}
	out := &ScratchCardsResponse{
		Active: []ActiveCardView{}, Expired: []map[string]any{}, History: []map[string]any{},
		EmptyTitle: "No scratch cards yet", EmptySubtitle: "Log consistently to earn coin scratch cards",
		Title: "Scratch Cards", NoRewardScreen: noRewardScreen(),
	}
	for i := range cards {
		c := cards[i]
		switch {
		case c.Status == "claimed":
			row := map[string]any{"id": c.ID, "reward": CardReward{Type: c.RewardType, Value: c.RewardValue}, "claimed_at": c.ClaimedAt}
			if c.RewardType == "coins" {
				var exp *time.Time
				if c.RewardTransactionRef != nil {
					exp = expiries[*c.RewardTransactionRef]
				}
				if exp == nil && c.ClaimedAt != nil {
					fallback := c.ClaimedAt.AddDate(0, 0, CoinExpiryDays)
					exp = &fallback
				}
				for k, v := range CoinExpiryFields(exp, now) {
					row[k] = v
				}
			}
			out.History = append(out.History, row)
		case c.Status == "active" && !c.ExpiresAt.After(now):
			out.Expired = append(out.Expired, map[string]any{
				"id": c.ID, "reward": CardReward{Type: c.RewardType, Value: c.RewardValue},
				"expires_at": c.ExpiresAt, "expired": true, "status": "expired",
				"expiryLabel": "Expired on " + common.FormatMoment(c.ExpiresAt, "D MMMM, YYYY"),
			})
		default:
			out.Active = append(out.Active, toActiveCardView(&c))
		}
	}
	return out, nil
}

// RevealScratchCard mirrors scratchCardService.revealScratchCard.
func (s *Service) RevealScratchCard(ctx context.Context, userID string, cardID primitive.ObjectID) (map[string]any, error) {
	now := s.now()
	claimed, err := s.Store.ClaimActiveCard(ctx, cardID, userID, now)
	if err != nil {
		return nil, err
	}
	if claimed == nil || claimed.ID.IsZero() {
		existing, err := s.Store.FindCardByIDForUser(ctx, cardID, userID)
		if err != nil {
			return nil, err
		}
		if existing == nil || existing.ID.IsZero() {
			return nil, common.NotFound(MsgCardNotFound)
		}
		if existing.Status == "claimed" {
			out := map[string]any{"revealed": true, "alreadyClaimed": true,
				"reward": CardReward{Type: existing.RewardType, Value: existing.RewardValue}}
			var exp *time.Time
			if existing.RewardTransactionRef != nil {
				if m, err := s.Store.RewardTxnExpiriesByIDs(ctx, []primitive.ObjectID{*existing.RewardTransactionRef}); err == nil {
					exp = m[*existing.RewardTransactionRef]
				}
			}
			for k, v := range CoinExpiryFields(exp, now) {
				out[k] = v
			}
			return out, nil
		}
		return nil, common.Gone(MsgCardExpired)
	}

	result, err := s.creditRevealedCard(ctx, userID, claimed, now)
	if err != nil {
		if rerr := s.Store.ReopenClaimedCard(ctx, claimed.ID); rerr != nil {
			s.Log.Error("failed to reopen claimed card after credit failure", "cardId", claimed.ID.Hex(), "error", rerr.Error())
		}
		return nil, err
	}
	return result, nil
}

func (s *Service) creditRevealedCard(ctx context.Context, userID string, card *models.ScratchCard, now time.Time) (map[string]any, error) {
	if card.RewardType != "coins" {
		return nil, common.WithStatus(501, MsgUnsupportedReward)
	}
	reward := CardReward{Type: card.RewardType, Value: card.RewardValue}
	existingTxn, err := s.Store.FindCreditTransactionByRemarks(ctx, userID, card.CreditRemarks)
	if err != nil {
		return nil, err
	}
	if existingTxn != nil && !existingTxn.ID.IsZero() {
		if err := s.Store.LinkRewardTransaction(ctx, card.ID, existingTxn.ID); err != nil {
			s.Log.Warn("linking existing reward transaction failed", "cardId", card.ID.Hex(), "error", err.Error())
		}
		out := map[string]any{"revealed": true, "reward": reward, "alreadyCredited": true}
		for k, v := range CoinExpiryFields(existingTxn.ExpireAt, now) {
			out[k] = v
		}
		return out, nil
	}
	master, err := s.EnsureStreakMaster(ctx, Reward{Type: card.RewardMeta.Tier, Slug: card.RewardMeta.Slug, Coins: card.RewardValue})
	if err != nil {
		return nil, err
	}
	ref, dup, err := s.SaveRewardTransaction90(ctx, userID, master, card.CreditRemarks, card.RewardValue)
	if err != nil {
		return nil, err
	}
	if dup {
		again, err := s.Store.FindCreditTransactionByRemarks(ctx, userID, card.CreditRemarks)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"revealed": true, "reward": reward, "alreadyCredited": true}
		if again != nil {
			_ = s.Store.LinkRewardTransaction(ctx, card.ID, again.ID)
			for k, v := range CoinExpiryFields(again.ExpireAt, now) {
				out[k] = v
			}
		}
		return out, nil
	}
	if err := s.Store.LinkRewardTransaction(ctx, card.ID, ref.ID); err != nil {
		s.Log.Warn("linking reward transaction failed", "cardId", card.ID.Hex(), "error", err.Error())
	}
	out := map[string]any{"revealed": true, "reward": reward}
	for k, v := range CoinExpiryFields(ref.ExpireAt, now) {
		out[k] = v
	}
	return out, nil
}

// ActiveCardRef is the minimal card reference the kit-tracker page embeds.
type ActiveCardRef struct {
	ID     primitive.ObjectID `json:"id"`
	Status string             `json:"status"`
}

// GetActiveCardForReveal mirrors scratchCardService.getActiveCardForReveal.
func (s *Service) GetActiveCardForReveal(ctx context.Context, userID, rewardDayDate string) *ActiveCardRef {
	card, err := s.Store.FindMintedCardForRewardDay(ctx, userID, ScratchSource, rewardDayDate)
	if err != nil || card == nil || card.ID.IsZero() {
		return nil
	}
	if card.Status != "active" || !card.ExpiresAt.After(s.now()) {
		return nil
	}
	return &ActiveCardRef{ID: card.ID, Status: card.Status}
}
