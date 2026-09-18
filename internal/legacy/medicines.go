package legacy

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"traya-bah-service/internal/common"
	"traya-bah-service/internal/orders"
	pgrepo "traya-bah-service/repositories/pg"
)

// ImageURL is the medicine image_url payload.
type ImageURL struct {
	ProductURL       string `json:"productUrl"`
	CartImgURL       string `json:"cartImgUrl"`
	MobileImgURL     string `json:"mobileImgUrl"`
	SingleHalfImages string `json:"singleHalfImages"`
}

// Medicine is one prescription row.
type Medicine struct {
	ProductID       string   `json:"product_id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Dosage          string   `json:"Dosage"`
	DosageCode      string   `json:"dosageCode"`
	Info            string   `json:"info"`
	Description     string   `json:"description"`
	Composition     string   `json:"composition"`
	Price           float64  `json:"price"`
	ItemCount       int      `json:"itemCount"`
	ImageURL        ImageURL `json:"image_url"`
	CartDisplayName string   `json:"cartDisplayName"`
	NewlyAdded      bool     `json:"newlyAdded"`
}

// BahV3ReorderText is the bahV3ReorderText payload ({} when empty).
type BahV3ReorderText struct {
	H1          string `json:"h1,omitempty"`
	H2          string `json:"h2,omitempty"`
	IsBahLocked *bool  `json:"isBahLocked,omitempty"`
}

// LatestMedicines is getLatestMedicines' result.
type LatestMedicines struct {
	Medicines         []Medicine        `json:"medicines"`
	LogDaysLeft       int               `json:"logDaysLeft"`
	Text1             string            `json:"text1"`
	Text2             string            `json:"text2"`
	IsReorderRequired bool              `json:"isReorderRequired"`
	ShowText          bool              `json:"showText"`
	CtaText           string            `json:"ctaText"`
	IsMedicineLocked  *bool             `json:"isMedicineLocked,omitempty"`
	BahV3ReorderText  *BahV3ReorderText `json:"bahV3ReorderText,omitempty"`
}

// GetLatestMedicines ports handler.js getLatestMedicines (the 45-day window).
func (s *Service) GetLatestMedicines(ctx context.Context, userID string, showBahV3 bool) (*LatestMedicines, error) {
	now := s.now()
	nonVoid, err := s.PG.NonVoidOrdersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &LatestMedicines{CtaText: "Order Next Kit", Medicines: []Medicine{}}
	locked := false
	var reorder *BahV3ReorderText
	if showBahV3 {
		reorder = &BahV3ReorderText{}
	}

	delivered := make([]orders.Order, 0, len(nonVoid))
	for _, o := range nonVoid {
		if o.Status == "delivered" {
			delivered = append(delivered, o)
		}
	}

	if len(delivered) == 0 {
		if len(nonVoid) > 0 {
			out.LogDaysLeft = 0
			out.Text1 = "Your Order is on its way"
			out.Text2 = "You can start logging in your routine after your kit is delivered."
			out.IsReorderRequired = false
			out.ShowText = true
			if showBahV3 {
				meds, err := s.GetPrescriptionForMedicinesByOrders(ctx, []orders.Order{nonVoid[len(nonVoid)-1]})
				if err != nil {
					return nil, err
				}
				out.Medicines = meds
				locked = true
			}
		}
	} else {
		details := orders.GetAllDetailsRelatedToNonVoidOrders(delivered, now)
		if details.KitExpireDays+15 >= details.MinDaysAfterOrderDelivered {
			out.LogDaysLeft = details.KitExpireDays - details.MinDaysAfterOrderDelivered + 15
		}
		var within []orders.Order
		for _, o := range delivered {
			kd := orders.GetKitDetail(o, now, orders.KitVariantsLegacy)
			kitCountForBAH := kd.KitCount
			if kitCountForBAH == 0 {
				kitCountForBAH = 1
			}
			if out.LogDaysLeft >= 0 && (kitCountForBAH*30)+15 >= kd.DiffDays {
				within = append(within, o)
				if details.KitExpireDays-details.MinDaysAfterOrderDelivered <= 9 {
					out.Text1 = fmt.Sprintf("%d Days Since Your Last Order - Act Fast! 🚀", details.MinDaysAfterOrderDelivered)
					out.Text2 = fmt.Sprintf("Log in access expires in %d days. Reorder to keep tracking your routine.", out.LogDaysLeft)
					out.IsReorderRequired = true
					out.ShowText = true
					if showBahV3 {
						f := false
						reorder = &BahV3ReorderText{H1: "Your next kit order is due.",
							H2: "Redeem your coins and get discount upto 20% on your next order.", IsBahLocked: &f}
					}
				}
			}
		}
		latestDelivered := len(nonVoid) > 0 && nonVoid[0].Status == "delivered"
		if len(within) == 0 {
			if !latestDelivered {
				out.Text1 = "Your Order is on its way"
				out.Text2 = "You can start logging in your routine after your kit is delivered."
				out.IsReorderRequired = false
				out.ShowText = true
				if showBahV3 && len(nonVoid) > 0 {
					meds, err := s.GetPrescriptionForMedicinesByOrders(ctx, []orders.Order{nonVoid[0]})
					if err != nil {
						return nil, err
					}
					out.Medicines = meds
					locked = true
				}
			} else {
				if showBahV3 && len(nonVoid) > 0 {
					tr := true
					reorder = &BahV3ReorderText{H1: "Log & Earn is locked.",
						H2: "Order now to use this feature and keep earning coins.", IsBahLocked: &tr}
					meds, err := s.GetPrescriptionForMedicinesByOrders(ctx, []orders.Order{nonVoid[0]})
					if err != nil {
						return nil, err
					}
					out.Medicines = meds
					locked = true
				}
				out.Text1 = "Oops! Your log and earn is locked"
				out.Text2 = "Reorder now to unlock the feature and maintain your streak"
				out.IsReorderRequired = true
				out.ShowText = true
				out.CtaText = "Save My Streak!"
			}
		} else if !latestDelivered {
			out.Text1 = "Your Next Order is on its way"
			out.Text2 = "You are currently logging in for your existing routine. You’ll be able to log in for your upcoming kit after it is delivered."
			out.IsReorderRequired = false
			out.ShowText = true
			if showBahV3 {
				reorder = &BahV3ReorderText{}
			}
		}
		if len(within) > 0 {
			meds, err := s.GetPrescriptionForMedicinesByOrders(ctx, within)
			if err != nil {
				return nil, err
			}
			out.Medicines = meds
		}
	}
	if showBahV3 {
		out.IsMedicineLocked = &locked
		out.BahV3ReorderText = reorder
	}
	return out, nil
}

// GetPrescriptionForMedicinesByOrders ports getMedicineIds + getProductDesc.
func (s *Service) GetPrescriptionForMedicinesByOrders(ctx context.Context, ords []orders.Order) ([]Medicine, error) {
	if len(ords) == 0 {
		return []Medicine{}, nil
	}
	variantMap := map[string]map[string]any{}
	if s.ConfigClient != nil {
		variantMap = s.ConfigClient.OldToActiveVariantMap(ctx, s.TenantID)
	}
	mapped := func(o orders.Order) []int64 {
		var out []int64
		for _, li := range orders.LineItems(o) {
			if li.VariantID == 0 {
				continue
			}
			out = append(out, orders.ActiveVariantID(li.VariantID, variantMap))
		}
		return out
	}
	var all []int64
	var newlyAdded []int64
	for i, o := range ords {
		ids := mapped(o)
		if i > 0 && i == len(ords)-1 {
			first := map[int64]bool{}
			for _, v := range ids {
				first[v] = true
			}
			for _, v := range mapped(ords[0]) {
				if !first[v] {
					newlyAdded = append(newlyAdded, v)
				}
			}
		}
		all = append(all, ids...)
	}
	seen := map[int64]bool{}
	var unique []int64
	for _, v := range all {
		if v != 0 && !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	if seen[orders.HairVitaminVariant] && seen[orders.DiscontinuedVitaminVariant] {
		filtered := unique[:0]
		for _, v := range unique {
			if v != orders.DiscontinuedVitaminVariant {
				filtered = append(filtered, v)
			}
		}
		unique = filtered
	}
	ids := make([]string, 0, len(unique))
	for _, v := range unique {
		ids = append(ids, strconv.FormatInt(v, 10))
	}
	descs, err := s.PG.ProductsByPrincipalIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	newly := map[int64]bool{}
	for _, v := range newlyAdded {
		newly[v] = true
	}
	s3 := s.Cfg.S3ImageBaseURL
	out := make([]Medicine, 0, len(descs))
	for _, d := range descs {
		pid, _ := strconv.ParseInt(d.ProductPrincipalID, 10, 64)
		out = append(out, Medicine{
			ProductID: d.ProductPrincipalID, Name: d.MedicineDisplayName, Type: d.MedicineType,
			Dosage: d.MedicineDosage, DosageCode: d.MedicineDosageCode, Info: d.MedicineInfo,
			Description: d.MedicineDescription, Composition: d.MedicineComposition,
			Price: d.ProductPrice, ItemCount: 1,
			ImageURL: ImageURL{ProductURL: s3 + d.ImageCDNPath, CartImgURL: s3 + d.CartCDNImages,
				MobileImgURL: s3 + d.MobileImagePath, SingleHalfImages: s3 + d.SingleHalfImages},
			CartDisplayName: d.MedicineDetailedDisplayName, NewlyAdded: newly[pid],
		})
	}
	// sortBy('newlyAdded').reverse() → newly added first, otherwise stable
	sort.SliceStable(out, func(i, j int) bool { return out[i].NewlyAdded && !out[j].NewlyAdded })
	return out, nil
}

// HowToUse is getMedicinesForHowToUsePurpose' result.
type HowToUse struct {
	MedicinesPrescription []Medicine `json:"medicinesPrescription"`
	IsPrescriptionLocked  bool       `json:"isPrescriptionLocked"`
	LatestOrderID         string     `json:"latestOrderId"`
	LatestOrderDisplayID  string     `json:"latestOrderDisplayId"`
	LatestOrderStatus     string     `json:"latestOrderStatus"`
	LatestOrderDate       *time.Time `json:"latestOrderDate"`
	HowToUseText          string     `json:"howToUseText"`
	ShowNew               bool       `json:"showNew"`
}

// GetMedicinesForHowToUsePurpose ports handler.js getMedicinesForHowToUsePurpose.
func (s *Service) GetMedicinesForHowToUsePurpose(ctx context.Context, userID string) (*HowToUse, error) {
	now := s.now()
	ords, err := s.PG.LatestNonVoidUnknownOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &HowToUse{ShowNew: true, MedicinesPrescription: []Medicine{}}
	if len(ords) == 0 {
		return out, nil
	}
	shippedOrDelivered := false
	for _, o := range ords {
		if o.Status == "shipped" || o.Status == "delivered" {
			shippedOrDelivered = true
			break
		}
	}
	out.IsPrescriptionLocked = !shippedOrDelivered
	out.LatestOrderID = ords[0].ID
	out.LatestOrderDisplayID = ords[0].OrderDisplayID
	out.LatestOrderStatus = ords[0].Status
	out.LatestOrderDate = ords[0].DeliveryDate

	var included []orders.Order
	for _, o := range ords {
		canSeeDays := 50
		daysDifference := common.CalendarDaysDifference(o.CreatedAt, now)
		if len(ords) > 1 {
			last, secondLast := ords[0].Status, ords[1].Status
			isLastDone := last == "delivered" || last == "shipped"
			firstOrderAge := common.CalendarDaysDifference(ords[0].CreatedAt, now)
			switch {
			case !isLastDone && secondLast == "delivered":
				out.HowToUseText = "Last Delivered"
			case last == "delivered" && firstOrderAge > 50:
				out.HowToUseText = "Expired"
			case !isLastDone && firstOrderAge > 50:
				out.HowToUseText = "Last Delivered"
			}
		} else if daysDifference > 50 {
			out.HowToUseText = "Expired"
		}
		if o.IsBulkOrder {
			canSeeDays = o.BulkOrderDuration*30 + 20
		}
		if daysDifference < canSeeDays {
			included = append(included, o)
		} else if len(included) == 0 {
			included = append(included, o)
		}
	}
	meds, err := s.GetPrescriptionForMedicinesByOrders(ctx, included)
	if err != nil {
		return nil, err
	}
	out.MedicinesPrescription = meds
	return out, nil
}

var _ = pgrepo.ProductDesc{}
