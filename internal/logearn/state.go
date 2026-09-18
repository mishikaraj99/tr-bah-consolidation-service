package logearn

import (
	"context"
	"strings"

	"traya-bah-service/internal/common"
)

// Last7 is one recent-log row.
type Last7 struct {
	Date        string `json:"date"`
	StreakDay   int    `json:"streakDay"`
	InrCredited int    `json:"inrCredited"`
	IsBackfill  bool   `json:"isBackfill"`
}

// ProductImage is the product image block.
type ProductImage struct {
	CartImgURL   string `json:"cartImgUrl"`
	ImageCdnURL  string `json:"imageCdnUrl"`
	SingleImages string `json:"singleImages"`
}

// Product is one kit product shown on the log screen.
type Product struct {
	Name            string       `json:"name"`
	CartDisplayName string       `json:"cartDisplayName"`
	Dosage          string       `json:"dosage"`
	ImageURL        ProductImage `json:"image_url"`
}

// State is the GET /consumers/logearn/state response.
type State struct {
	CustomerID           string    `json:"customerId"`
	Balance              int       `json:"balance"`
	StreakDay            int       `json:"streakDay"`
	LogDoneToday         bool      `json:"logDoneToday"`
	ZForNext             int       `json:"zForNext"`
	ZTier                int       `json:"zTier"`
	Last7                []Last7   `json:"last7"`
	BackfillAvailable    bool      `json:"backfillAvailable"`
	LifelineAvailable    bool      `json:"lifelineAvailable"`
	BulkX                int       `json:"bulkX"`
	TreatmentDay         int       `json:"treatmentDay"`
	EarningCapDays       int       `json:"earningCapDays"`
	TotalWindowDays      int       `json:"totalWindowDays"`
	EarningCapHit        bool      `json:"earningCapHit"`
	WindowExpired        bool      `json:"windowExpired"`
	EarningDaysConsumed  int       `json:"earningDaysConsumed"`
	EarningDaysRemaining int       `json:"earningDaysRemaining"`
	InGracePeriod        bool      `json:"inGracePeriod"`
	OrderID              *string   `json:"orderId"`
	Products             []Product `json:"products"`
}

// GetState ports logearn.service state.
func (s *Service) GetState(ctx context.Context, customerID string) (*State, error) {
	now := s.now()
	logs, err := s.Store.RecentDoseLogs(ctx, customerID, 14)
	if err != nil {
		return nil, err
	}
	cap := s.GetEarningCapInfo(ctx, customerID)
	cfg := s.Config.Get(ctx, s.TenantID)

	out := &State{
		CustomerID: customerID, Balance: s.balance(ctx, customerID), Last7: []Last7{}, Products: []Product{},
		BulkX: cap.BulkX, TreatmentDay: cap.TreatmentDay, EarningCapDays: cap.EarningCapDays,
		TotalWindowDays: cap.TotalWindowDays, EarningCapHit: cap.EarningCapHit, WindowExpired: cap.WindowExpired,
		EarningDaysConsumed: cap.EarningDaysConsumed, EarningDaysRemaining: cap.EarningDaysRemaining,
		InGracePeriod: cap.InGracePeriod, OrderID: cap.OrderID,
	}

	todayKey, yKey := common.ISTDateString(now), common.ISTDateString(now.AddDate(0, 0, -1))
	yesterdayLogged := false
	for i, l := range logs {
		key := common.ISTDateString(l.LogDate)
		if key == todayKey {
			out.LogDoneToday = true
		}
		if key == yKey {
			yesterdayLogged = true
		}
		if i == 0 {
			out.StreakDay = l.StreakDay
		}
		if len(out.Last7) < 7 {
			out.Last7 = append(out.Last7, Last7{Date: key, StreakDay: l.StreakDay, InrCredited: l.INRCredited, IsBackfill: l.IsBackfill})
		}
	}
	tier, amount := ZAmountForDay(out.StreakDay+1, cfg.ZTiers)
	out.ZTier = tier
	if !cap.EarningCapHit {
		out.ZForNext = amount
	}
	if len(logs) > 0 {
		gap := common.ISTDaysBetween(logs[0].LogDate, now)
		out.BackfillAvailable = gap <= 2 && !yesterdayLogged
		if !out.LogDoneToday && gap > 2 {
			used := false
			if cap.DeliveryDate != nil {
				u, err := s.Store.ExistsLedgerReasonSince(ctx, customerID, ReasonLifelineRecovered, *cap.DeliveryDate)
				if err == nil {
					used = u
				}
			}
			out.LifelineAvailable = !used
		}
	}
	if cap.OrderID != nil && *cap.OrderID != "" {
		out.Products = s.productsForOrder(ctx, *cap.OrderID)
	}
	return out, nil
}

// productsForOrder walks the order-details payload for the line-item product display data.
func (s *Service) productsForOrder(ctx context.Context, orderID string) []Product {
	out := []Product{}
	details, err := s.Orders.OrderDetails(ctx, s.TenantID, orderID)
	if err != nil {
		s.Log.Error("Failed to fetch order products for dose log", "orderId", orderID, "error", err.Error())
		return out
	}
	base := ""
	if s.Cfg != nil {
		base = s.Cfg.S3ImageBaseURL
	}
	for _, item := range findLineItems(details) {
		variant, _ := item["productVariant"].(map[string]any)
		if variant == nil {
			continue
		}
		p := Product{Name: asString(variant["name"])}
		if prod, ok := item["product"].(map[string]any); ok {
			p.CartDisplayName = asString(prod["name"])
		}
		if p.CartDisplayName == "" {
			p.CartDisplayName = p.Name
		}
		if instructions, ok := variant["productUsageInstructions"].([]any); ok && len(instructions) > 0 {
			if first, ok := instructions[0].(map[string]any); ok {
				p.Dosage = asString(first["dosageDescription"])
			}
		}
		medias, _ := variant["productMedias"].([]any)
		p.ImageURL = ProductImage{
			CartImgURL:   mediaURL(base, medias, "cart_cdn_images"),
			ImageCdnURL:  mediaURL(base, medias, "image_cdn_path"),
			SingleImages: mediaURL(base, medias, "single_images"),
		}
		out = append(out, p)
	}
	return out
}

func findLineItems(payload map[string]any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if k == "orderLineItems" || k == "lineItems" || k == "line_items" {
					if arr, ok := val.([]any); ok {
						for _, item := range arr {
							if m, ok := item.(map[string]any); ok {
								out = append(out, m)
							}
						}
					}
				}
				walk(val)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(payload)
	return out
}

func mediaURL(base string, medias []any, usageType string) string {
	for _, m := range medias {
		media, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if asString(media["usageType"]) != usageType {
			continue
		}
		url := asString(media["url"])
		if url == "" {
			return ""
		}
		return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(url, "/")
	}
	return ""
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
