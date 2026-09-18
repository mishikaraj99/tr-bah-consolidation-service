package orders

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(y, m, day int) time.Time  { return time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC) }
func ptr(t time.Time) *time.Time { return &t }

func kitOrder(delivery time.Time, qty int, status string) Order {
	return Order{Status: status, CreatedAt: delivery.AddDate(0, 0, -3), DeliveryDate: ptr(delivery),
		OrderMeta: map[string]any{"line_items": []any{map[string]any{"variant_id": float64(41645770309810), "quantity": float64(qty), "name": "Recap"}}}}
}

func ymd(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// Ported from traya-api-server/server/components/BAH/test/habitKitWindow.test.js
func TestCurrentKitStart(t *testing.T) {
	kc := func(o Order) int { return int(o.BulkOrderDuration) } // test uses explicit kitCount via BulkOrderDuration
	mk := func(del time.Time, kits int) Order {
		return Order{DeliveryDate: ptr(del), BulkOrderDuration: kits}
	}
	assert.Equal(t, "2026-06-09", ymd(CurrentKitStart([]Order{mk(d(2026, 6, 9), 1)}, d(2026, 7, 17), kc)))
	assert.Equal(t, "2026-06-09", ymd(CurrentKitStart([]Order{mk(d(2026, 6, 9), 1)}, d(2026, 6, 20), kc)))
	assert.Equal(t, "2026-07-09", ymd(CurrentKitStart([]Order{mk(d(2026, 7, 8), 1), mk(d(2026, 6, 9), 1)}, d(2026, 7, 17), kc)))
	assert.Equal(t, "2026-07-09", ymd(CurrentKitStart([]Order{mk(d(2026, 6, 9), 3)}, d(2026, 7, 17), kc)))
	assert.Nil(t, CurrentKitStart(nil, d(2026, 7, 17), kc))
	assert.Nil(t, CurrentKitStart([]Order{mk(d(2026, 6, 9), 0)}, d(2026, 7, 17), kc))
}

func TestGetKitDetail_And_LineItems(t *testing.T) {
	now := d(2026, 9, 18)
	o := kitOrder(d(2026, 9, 8), 2, "delivered")
	kd := GetKitDetail(o, now, KitVariantsLegacy)
	assert.Equal(t, 2, kd.KitCount)
	assert.Equal(t, 10, kd.DiffDays)
	assert.True(t, kd.IsOrderDelivered)
	// unknown variant → kitCount 0
	o2 := Order{Status: "delivered", CreatedAt: now, OrderMeta: map[string]any{"line_items": []any{map[string]any{"variant_id": "123", "quantity": float64(3)}}}}
	assert.Equal(t, 0, GetKitDetail(o2, now, KitVariantsLegacy).KitCount)
	// Hair_ras_v3 counts for legacy but not habit
	o3 := Order{Status: "delivered", CreatedAt: now, OrderMeta: map[string]any{"line_items": []any{map[string]any{"variant_id": float64(47446948479154), "quantity": float64(1)}}}}
	assert.Equal(t, 1, GetKitDetail(o3, now, KitVariantsLegacy).KitCount)
	assert.Equal(t, 0, GetKitDetail(o3, now, KitVariantsHabit).KitCount)
}

func TestGetAllDetailsRelatedToNonVoidOrders(t *testing.T) {
	now := d(2026, 9, 18)
	single := GetAllDetailsRelatedToNonVoidOrders([]Order{kitOrder(d(2026, 9, 8), 1, "delivered")}, now)
	assert.Equal(t, 10, single.MinDaysAfterOrderDelivered)
	assert.Equal(t, 30, single.KitExpireDays)
	assert.Equal(t, 2, single.RunningWeekinMonthForHairKit)
	assert.Equal(t, 1, single.RunningMonthForHairKit)
	assert.False(t, single.IsOrderPlaced)

	bulk := GetAllDetailsRelatedToNonVoidOrders([]Order{kitOrder(d(2026, 8, 1), 2, "delivered")}, now)
	assert.Equal(t, 60, bulk.KitExpireDays)
	assert.True(t, bulk.IsLatestOrderBulkKit)

	placed := GetAllDetailsRelatedToNonVoidOrders([]Order{kitOrder(d(2026, 9, 20), 1, "placed"), kitOrder(d(2026, 8, 1), 1, "delivered")}, now)
	assert.True(t, placed.IsOrderPlaced)
	assert.Equal(t, 2, placed.TotalKitCount)

	empty := GetAllDetailsRelatedToNonVoidOrders(nil, now)
	assert.Equal(t, 30, empty.KitExpireDays)
	assert.Equal(t, 0, empty.MinDaysAfterOrderDelivered)
	assert.Equal(t, 1, empty.TotalKitCount)
}

func TestGetAllOrderDetails_AppBackend(t *testing.T) {
	now := d(2026, 9, 18)
	od := GetAllOrderDetails(nil, now)
	assert.Equal(t, -1, od.MinDaysAfterOrderDelivered)
	// first-order fallback: a shampoo-only single order still counts as one kit
	shampoo := Order{Status: "delivered", CreatedAt: d(2026, 9, 1), DeliveryDate: ptr(d(2026, 9, 3)),
		OrderMeta: map[string]any{"line_items": []any{map[string]any{"variant_id": "1", "quantity": float64(1)}}}}
	od = GetAllOrderDetails([]Order{shampoo}, now)
	assert.Equal(t, 1, od.TotalKitCount)
	assert.Equal(t, 15, od.MinDaysAfterOrderDelivered)
	assert.Equal(t, 30, od.KitExpireDays)
	assert.Equal(t, "Delivered", od.LatestOrderStatus)
	assert.Equal(t, 3, od.RunningWeekInMonthForHairKit)
}

func TestKitWindows(t *testing.T) {
	now := d(2026, 9, 18)
	orders := []Order{kitOrder(d(2026, 7, 8), 1, "delivered"), kitOrder(d(2026, 6, 9), 1, "delivered")}
	w := HabitTrackerKitWindows(orders, now)
	require.Len(t, w, 2)
	assert.Equal(t, "2026-06-09", w[0].Start.Format("2006-01-02"))
	assert.Equal(t, "2026-07-09", w[1].Start.Format("2006-01-02"), "second kit queues after the first ends")
	assert.Equal(t, 2, w[1].KitNumber)
	// inside kit 2 (2026-07-09 → 2026-08-08)
	assert.Equal(t, "2026-07-09", ymd(CurrentRunningKitStartDate(orders, d(2026, 7, 20))))
	// past every window → the last sub-kit's start (lapsed user)
	assert.Equal(t, "2026-07-09", ymd(CurrentRunningKitStartDate(orders, d(2026, 8, 20))))
	ld := LastDelivered(orders)
	require.NotNil(t, ld)
	assert.Equal(t, "2026-07-08", ymd(ld.DeliveryDate))
}
