package orders

import (
	"math"
	"time"

	"traya-bah-service/internal/common"
)

// KitDetail is getKitDetail's result.
type KitDetail struct {
	KitCount         int
	DiffDays         int
	IsOrderDelivered bool
	OrderStatus      string
	ProductDetails   []LineItem
}

// GetKitDetail mirrors kit_calculator.getKitDetail with the given kit-variant list.
func GetKitDetail(o Order, now time.Time, variants []int64) KitDetail {
	d := KitDetail{IsOrderDelivered: o.Status == "delivered", OrderStatus: o.Status,
		DiffDays: common.CalendarDaysDifference(AnchorDate(o), now)}
	byVariant := map[int64]int{}
	for _, li := range LineItems(o) {
		d.ProductDetails = append(d.ProductDetails, li)
		if contains(variants, li.VariantID) {
			byVariant[li.VariantID] = li.Quantity
		}
	}
	for _, q := range byVariant {
		if q > d.KitCount {
			d.KitCount = q
		}
	}
	return d
}

// KitCountLegacy / KitCountHabit are the kitCount callbacks for CurrentKitStart.
func KitCountLegacy(now time.Time) func(Order) int {
	return func(o Order) int { return GetKitDetail(o, now, KitVariantsLegacy).KitCount }
}
func KitCountHabit(now time.Time) func(Order) int {
	return func(o Order) int { return GetKitDetail(o, now, KitVariantsHabit).KitCount }
}

// NonVoidDetails is getAllDetailsRelatedToNonVoidOrders' result.
type NonVoidDetails struct {
	TotalKitCount, TotalDaysForUsingKit, RunningMonthForHairKit, KitExpireDays       int
	MinDaysAfterOrderDelivered, RunningWeek, RunningWeekinMonthForHairKit            int
	IsOrderPlaced, IsAnyOrderDelivered, IsLatestOrderBulkKit                         bool
	LatestOrderWithKitCount, LatestBulkKitCount                                      int
	LatestKitOrderDeliveryDate, FirstKitOrderDeliveryDate, LatestBulkKitDeliveryDate *time.Time
	ShouldConsiderLastPlacedOrder                                                    bool
	AllProductDetails, LatestOrderProducts                                           []LineItem
}

func ceilDiv(a, b int) int { return int(math.Ceil(float64(a) / float64(b))) }

// GetAllDetailsRelatedToNonVoidOrders ports traya-api-server kit_calculator.getAllDetailsRelatedToNonVoidOrders
// (legacy economy). orders must be created_at DESC.
func GetAllDetailsRelatedToNonVoidOrders(orders []Order, now time.Time) NonVoidDetails {
	r := NonVoidDetails{RunningMonthForHairKit: 1, MinDaysAfterOrderDelivered: -1}
	tempKitCountForDays := 0
	kitCountForDays := 30
	if len(orders) > 0 {
		for i, o := range orders {
			kd := GetKitDetail(o, now, KitVariantsLegacy)
			r.TotalKitCount += kd.KitCount
			if kd.KitCount == 0 {
				kitCountForDays = 30
			} else {
				kitCountForDays = kd.KitCount * 30
			}
			r.AllProductDetails = append(r.AllProductDetails, kd.ProductDetails...)
			if i == 0 {
				r.LatestOrderProducts = kd.ProductDetails
			}
			if kd.IsOrderDelivered {
				if kd.KitCount > 1 && !r.IsLatestOrderBulkKit {
					r.LatestBulkKitCount = kd.KitCount
					r.LatestBulkKitDeliveryDate = o.DeliveryDate
					r.IsLatestOrderBulkKit = true
				}
				if r.LatestOrderWithKitCount == 0 {
					r.LatestOrderWithKitCount = kd.KitCount
					r.LatestKitOrderDeliveryDate = o.DeliveryDate
				}
				r.FirstKitOrderDeliveryDate = o.DeliveryDate
				r.IsAnyOrderDelivered = true
				if r.MinDaysAfterOrderDelivered == -1 || tempKitCountForDays-r.MinDaysAfterOrderDelivered <= kitCountForDays-kd.DiffDays {
					tempKitCountForDays = kitCountForDays
					r.MinDaysAfterOrderDelivered = kd.DiffDays
					if kitCountForDays > r.KitExpireDays {
						r.KitExpireDays = kitCountForDays
					}
				}
				if kitCountForDays >= kd.DiffDays {
					if kd.KitCount != 0 {
						if kd.DiffDays == 0 {
							r.TotalDaysForUsingKit++
						} else {
							r.TotalDaysForUsingKit += kd.DiffDays
						}
					}
					switch {
					case kd.DiffDays == 0:
						r.RunningWeek = 1
					case kd.DiffDays%30 == 0:
						r.RunningWeek = 4
					default:
						r.RunningWeek = ceilDiv(kd.DiffDays%30, 7)
					}
					if r.RunningWeek > 4 {
						r.RunningWeek = 4
					}
				} else {
					r.TotalDaysForUsingKit += kd.KitCount * 30
					if r.RunningWeek == 0 {
						r.RunningWeek = 4
					}
				}
			} else {
				r.IsOrderPlaced = true
			}
		}
		if r.TotalDaysForUsingKit == 0 {
			r.RunningMonthForHairKit = 1
		} else {
			r.RunningMonthForHairKit = ceilDiv(r.TotalDaysForUsingKit, 30)
		}
		if r.IsOrderPlaced {
			r.RunningWeekinMonthForHairKit = orInt(r.RunningWeek, 1)
		} else {
			r.RunningWeekinMonthForHairKit = orInt(r.RunningWeek, 4)
		}
		maxDay := common.CalendarDaysDifference(orders[len(orders)-1].CreatedAt, now)
		if maxDay == 0 {
			maxDay = 1
		}
		if r.RunningMonthForHairKit > ceilDiv(maxDay, 30) {
			r.RunningMonthForHairKit = ceilDiv(maxDay, 30)
		}
	}
	if r.KitExpireDays == 0 {
		r.KitExpireDays = kitCountForDays
	}
	if r.MinDaysAfterOrderDelivered == -1 {
		r.MinDaysAfterOrderDelivered = 0
	}
	isFirstKit := r.TotalKitCount == 1
	isExpired := r.MinDaysAfterOrderDelivered > r.KitExpireDays
	if r.IsLatestOrderBulkKit {
		daysSince := 0
		if r.LatestBulkKitDeliveryDate != nil {
			daysSince = common.CalendarDaysDifference(*r.LatestBulkKitDeliveryDate, now)
		}
		daysRemaining := r.LatestBulkKitCount*30 - daysSince
		inLastWeek := daysRemaining <= 7
		if r.TotalKitCount > r.RunningMonthForHairKit && (isFirstKit || isExpired || inLastWeek) {
			if isExpired || inLastWeek {
				r.RunningMonthForHairKit++
			}
			r.RunningWeekinMonthForHairKit = 1
		}
	} else {
		inLastWeek := r.MinDaysAfterOrderDelivered < r.KitExpireDays && r.RunningWeekinMonthForHairKit > 3
		if r.TotalKitCount > r.RunningMonthForHairKit && (isFirstKit || isExpired || inLastWeek) {
			if isExpired || inLastWeek {
				r.RunningMonthForHairKit++
			}
			r.RunningWeekinMonthForHairKit = 1
		}
	}
	if r.FirstKitOrderDeliveryDate != nil && r.LatestKitOrderDeliveryDate != nil && r.LatestOrderWithKitCount > 0 &&
		r.RunningMonthForHairKit >= r.TotalKitCount && common.CalendarDaysDifference(*r.FirstKitOrderDeliveryDate, now) > r.TotalKitCount*30 {
		diff := common.CalendarDaysDifference(*r.LatestKitOrderDeliveryDate, now)
		daysForRunningWeek := diff - (r.LatestOrderWithKitCount-1)*30
		if daysForRunningWeek == 0 {
			r.RunningWeekinMonthForHairKit = 1
		} else {
			r.RunningWeekinMonthForHairKit = ceilDiv(daysForRunningWeek, 7)
		}
	}
	if r.TotalKitCount == 0 {
		r.TotalKitCount = 1
	}
	if r.RunningWeekinMonthForHairKit < 0 {
		r.RunningWeekinMonthForHairKit = -r.RunningWeekinMonthForHairKit
	}
	return r
}

func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// AppBackendNonVoidDetails ports traya-app-backend orderDataService.getAllDetailsRelatedToNonVoidOrders
// (v85 economy; differs from the api-server version: delivered non-kit orders are skipped and the
// week clock is derived from totalDaysForUsingKit). Returns TotalKitCount possibly 0.
func AppBackendNonVoidDetails(orders []Order, now time.Time) NonVoidDetails {
	r := NonVoidDetails{RunningMonthForHairKit: 1, MinDaysAfterOrderDelivered: -1}
	tempKitCountForDays := 0
	kitCountForDays := 0
	if len(orders) > 0 {
		for _, o := range orders {
			kd := GetKitDetail(o, now, KitVariantsHabit)
			r.TotalKitCount += kd.KitCount
			if kd.KitCount == 0 {
				kitCountForDays = 30
			} else {
				kitCountForDays = kd.KitCount * 30
			}
			if kd.IsOrderDelivered {
				if kd.KitCount == 0 {
					continue
				}
				if kd.KitCount > 1 && !r.IsLatestOrderBulkKit {
					r.LatestBulkKitCount = kd.KitCount
					r.LatestBulkKitDeliveryDate = o.DeliveryDate
					r.IsLatestOrderBulkKit = true
				}
				if r.LatestOrderWithKitCount == 0 {
					r.LatestOrderWithKitCount = kd.KitCount
					r.LatestKitOrderDeliveryDate = o.DeliveryDate
				}
				r.FirstKitOrderDeliveryDate = o.DeliveryDate
				r.IsAnyOrderDelivered = true
				if r.MinDaysAfterOrderDelivered == -1 || tempKitCountForDays-r.MinDaysAfterOrderDelivered <= kitCountForDays-kd.DiffDays {
					tempKitCountForDays = kitCountForDays
					r.MinDaysAfterOrderDelivered = kd.DiffDays
					if kitCountForDays > r.KitExpireDays {
						r.KitExpireDays = kitCountForDays
					}
				}
				if kitCountForDays >= kd.DiffDays {
					if kd.DiffDays == 0 {
						r.TotalDaysForUsingKit++
					} else {
						r.TotalDaysForUsingKit += kd.DiffDays
					}
				} else {
					r.TotalDaysForUsingKit += kd.KitCount * 30
				}
			} else {
				r.IsOrderPlaced = true
			}
		}
		if r.TotalDaysForUsingKit == 0 {
			r.RunningMonthForHairKit = 1
			if r.IsOrderPlaced {
				r.RunningWeekinMonthForHairKit = 1
			} else {
				r.RunningWeekinMonthForHairKit = 4
			}
		} else {
			r.RunningMonthForHairKit = ceilDiv(r.TotalDaysForUsingKit, 30)
			days := r.TotalDaysForUsingKit % 30
			if days == 0 {
				r.RunningWeekinMonthForHairKit = 4
			} else {
				r.RunningWeekinMonthForHairKit = ceilDiv(days, 7)
				if r.RunningWeekinMonthForHairKit > 4 {
					r.RunningWeekinMonthForHairKit = 4
				}
			}
		}
		maxDay := common.CalendarDaysDifference(orders[len(orders)-1].CreatedAt, now)
		if maxDay == 0 {
			maxDay = 1
		}
		if r.RunningMonthForHairKit > ceilDiv(maxDay, 30) {
			r.RunningMonthForHairKit = ceilDiv(maxDay, 30)
		}
	}
	if r.KitExpireDays == 0 {
		r.KitExpireDays = kitCountForDays
	}
	if r.MinDaysAfterOrderDelivered == -1 {
		r.MinDaysAfterOrderDelivered = 0
	}
	isFirstKit := r.TotalKitCount == 1
	isExpired := r.MinDaysAfterOrderDelivered > r.KitExpireDays
	if r.IsLatestOrderBulkKit {
		daysSince := 0
		if r.LatestBulkKitDeliveryDate != nil {
			daysSince = common.CalendarDaysDifference(*r.LatestBulkKitDeliveryDate, now)
		}
		inLastWeek := r.LatestBulkKitCount*30-daysSince <= 7
		if r.TotalKitCount > r.RunningMonthForHairKit && (isFirstKit || isExpired || inLastWeek) {
			if isExpired || inLastWeek {
				r.RunningMonthForHairKit++
			}
			r.RunningWeekinMonthForHairKit = 1
		}
	} else {
		inLastWeek := r.MinDaysAfterOrderDelivered < r.KitExpireDays && r.RunningWeekinMonthForHairKit > 3
		if r.TotalKitCount > r.RunningMonthForHairKit && (isFirstKit || isExpired || inLastWeek) {
			if isExpired || inLastWeek {
				r.RunningMonthForHairKit++
			}
			r.RunningWeekinMonthForHairKit = 1
		}
	}
	r.ShouldConsiderLastPlacedOrder = r.RunningMonthForHairKit >= r.TotalKitCount && r.IsOrderPlaced
	if r.FirstKitOrderDeliveryDate != nil && r.LatestKitOrderDeliveryDate != nil &&
		r.RunningMonthForHairKit >= r.TotalKitCount && common.CalendarDaysDifference(*r.FirstKitOrderDeliveryDate, now) > r.TotalKitCount*30 {
		diff := common.CalendarDaysDifference(*r.LatestKitOrderDeliveryDate, now)
		daysForRunningWeek := diff - (r.LatestOrderWithKitCount-1)*30
		if daysForRunningWeek == 0 {
			r.RunningWeekinMonthForHairKit = 1
		} else {
			r.RunningWeekinMonthForHairKit = ceilDiv(daysForRunningWeek, 7)
		}
	}
	if r.RunningWeekinMonthForHairKit < 0 {
		r.RunningWeekinMonthForHairKit = -r.RunningWeekinMonthForHairKit
	}
	return r
}

// OrderDetails is app-backend orderDataService.getAllOrderDetails' result (kit-tracker banner / reorder CTA).
type OrderDetails struct {
	TotalKitCount, KitExpireDays, MinDaysAfterOrderDelivered int
	IsOrderPlaced, IsOrderDelivered                          bool
	LatestOrderStatus                                        string
	RunningMonthForHairKit, RunningWeekInMonthForHairKit     int
	OrderCount                                               int
}

// GetAllOrderDetails ports app-backend getAllOrderDetails (first-order fallback, kit-only anchoring).
func GetAllOrderDetails(orders []Order, now time.Time) OrderDetails {
	if len(orders) == 0 {
		return OrderDetails{MinDaysAfterOrderDelivered: -1, RunningMonthForHairKit: 1, RunningWeekInMonthForHairKit: 1}
	}
	r := OrderDetails{MinDaysAfterOrderDelivered: -1, OrderCount: len(orders), LatestOrderStatus: "Placed"}
	if orders[0].Status == "delivered" {
		r.LatestOrderStatus = "Delivered"
	}
	isFirstOrder := len(orders) == 1
	totalDays := 0
	latestRunningWeek := 0
	for _, o := range orders {
		kd := GetKitDetail(o, now, KitVariantsHabit)
		kitCount := kd.KitCount
		if kitCount == 0 && isFirstOrder {
			kitCount = 1
		}
		r.TotalKitCount += kitCount
		kitDays := maxInt(kitCount, 1) * 30
		if kd.IsOrderDelivered {
			r.IsOrderDelivered = true
			if kitCount > 0 && (r.MinDaysAfterOrderDelivered == -1 || kd.DiffDays < r.MinDaysAfterOrderDelivered) {
				r.MinDaysAfterOrderDelivered = kd.DiffDays
				r.KitExpireDays = maxInt(r.KitExpireDays, kitDays)
			}
			daysUsed := minInt(kd.DiffDays, kitDays)
			totalDays += daysUsed
			latestRunningWeek = maxInt(latestRunningWeek, minInt(ceilDiv(daysUsed%30, 7), 4))
		} else {
			r.IsOrderPlaced = true
		}
	}
	maxDays := common.CalendarDaysDifference(orders[len(orders)-1].CreatedAt, now)
	r.TotalKitCount = maxInt(r.TotalKitCount, 1)
	r.RunningMonthForHairKit = minInt(ceilDiv(totalDays, 30), ceilDiv(maxDays, 30))
	if r.RunningMonthForHairKit == 0 {
		r.RunningMonthForHairKit = 1
	}
	if r.IsOrderPlaced {
		r.RunningWeekInMonthForHairKit = orInt(latestRunningWeek, 1)
	} else {
		r.RunningWeekInMonthForHairKit = orInt(latestRunningWeek, 4)
	}
	return r
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CurrentKitStart mirrors habitKitWindow.currentKitStart / getCurrentRunningKitStartDate.
// orders must be created_at DESC. Returns nil when there are no kit orders.
func CurrentKitStart(orders []Order, today time.Time, kitCount func(Order) int) *time.Time {
	var prevKitEnd, kitStart *time.Time
	for i := len(orders) - 1; i >= 0; i-- {
		o := orders[i]
		kc := kitCount(o)
		if kc <= 0 {
			continue
		}
		orderDate := AnchorDate(o)
		subKitStart := orderDate
		if prevKitEnd != nil && prevKitEnd.After(orderDate) {
			subKitStart = *prevKitEnd
		}
		for k := 0; k < kc; k++ {
			subKitEnd := subKitStart.AddDate(0, 0, 30)
			ks := subKitStart
			kitStart = &ks
			if !today.Before(subKitStart) && today.Before(subKitEnd) {
				return kitStart
			}
			subKitStart = subKitEnd
		}
		pk := subKitStart
		prevKitEnd = &pk
	}
	return kitStart
}

// KitWindow is one 30-day sub-kit window.
type KitWindow struct {
	KitNumber  int
	Start, End time.Time
}

// HabitTrackerKitWindows mirrors app-backend getHabitTrackerKitWindows (oldest-first, bulk expanded).
func HabitTrackerKitWindows(orders []Order, now time.Time) []KitWindow {
	var windows []KitWindow
	var prevKitEnd *time.Time
	n := 0
	for i := len(orders) - 1; i >= 0; i-- {
		o := orders[i]
		kc := GetKitDetail(o, now, KitVariantsHabit).KitCount
		if kc <= 0 {
			continue
		}
		orderDate := AnchorDate(o)
		subKitStart := orderDate
		if prevKitEnd != nil && prevKitEnd.After(orderDate) {
			subKitStart = *prevKitEnd
		}
		for k := 0; k < kc; k++ {
			subKitEnd := subKitStart.AddDate(0, 0, 30)
			n++
			windows = append(windows, KitWindow{KitNumber: n, Start: subKitStart, End: subKitEnd})
			subKitStart = subKitEnd
		}
		pk := subKitStart
		prevKitEnd = &pk
	}
	return windows
}

// CurrentRunningKitStartDate mirrors app-backend getCurrentRunningKitStartDate.
func CurrentRunningKitStartDate(orders []Order, now time.Time) *time.Time {
	return CurrentKitStart(orders, now, KitCountHabit(now))
}
