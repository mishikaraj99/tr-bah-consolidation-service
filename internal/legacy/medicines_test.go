package legacy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"traya-bah-service/internal/orders"
	pgrepo "traya-bah-service/repositories/pg"
)

func ptrTime(t time.Time) *time.Time { return &t }

func kitOrder(status string, delivery time.Time, qty int) orders.Order {
	return orders.Order{ID: "o1", Status: status, CreatedAt: delivery.AddDate(0, 0, -3), DeliveryDate: ptrTime(delivery),
		OrderMeta: map[string]any{"line_items": []any{map[string]any{"variant_id": float64(41645770309810), "quantity": float64(qty), "name": "Recap"}}}}
}

func TestGetLatestMedicines(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	po.products = []pgrepo.ProductDesc{{ProductPrincipalID: "41645770309810", MedicineDisplayName: "Recap Serum", MedicineDosage: "1-0-1", MedicineDosageCode: "1-0-1"}}

	// no delivered orders, one placed
	po.byUser = []orders.Order{kitOrder("placed", day(2026, 9, 17), 1)}
	out, err := s.GetLatestMedicines(ctx, "u1", false)
	require.NoError(t, err)
	assert.Equal(t, "Your Order is on its way", out.Text1)
	assert.Equal(t, "You can start logging in your routine after your kit is delivered.", out.Text2)
	assert.False(t, out.IsReorderRequired)
	assert.True(t, out.ShowText)
	assert.Nil(t, out.IsMedicineLocked, "v3 fields absent without showBahV3")

	// delivered 10 days ago, inside the window → no reorder nudge
	po.byUser = []orders.Order{kitOrder("delivered", day(2026, 9, 8), 1)}
	out, err = s.GetLatestMedicines(ctx, "u1", false)
	require.NoError(t, err)
	assert.Equal(t, 35, out.LogDaysLeft, "30 - 10 + 15")
	assert.False(t, out.IsReorderRequired)
	require.Len(t, out.Medicines, 1)
	assert.Equal(t, "Recap Serum", out.Medicines[0].Name)

	// delivered 25 days ago → inside the 9-day reorder window
	po.byUser = []orders.Order{kitOrder("delivered", day(2026, 8, 24), 1)}
	out, err = s.GetLatestMedicines(ctx, "u1", true)
	require.NoError(t, err)
	assert.Equal(t, "25 Days Since Your Last Order - Act Fast! 🚀", out.Text1)
	assert.Equal(t, "Log in access expires in 20 days. Reorder to keep tracking your routine.", out.Text2)
	assert.True(t, out.IsReorderRequired)
	require.NotNil(t, out.BahV3ReorderText)
	assert.Equal(t, "Your next kit order is due.", out.BahV3ReorderText.H1)
	require.NotNil(t, out.IsMedicineLocked)
	assert.False(t, *out.IsMedicineLocked)

	// delivered long ago → locked
	po.byUser = []orders.Order{kitOrder("delivered", day(2026, 5, 1), 1)}
	out, err = s.GetLatestMedicines(ctx, "u1", true)
	require.NoError(t, err)
	assert.Equal(t, "Oops! Your log and earn is locked", out.Text1)
	assert.Equal(t, "Reorder now to unlock the feature and maintain your streak", out.Text2)
	assert.Equal(t, "Save My Streak!", out.CtaText)
	require.NotNil(t, out.IsMedicineLocked)
	assert.True(t, *out.IsMedicineLocked)
	require.NotNil(t, out.BahV3ReorderText)
	assert.Equal(t, "Log & Earn is locked.", out.BahV3ReorderText.H1)
	require.NotNil(t, out.BahV3ReorderText.IsBahLocked)
	assert.True(t, *out.BahV3ReorderText.IsBahLocked)
}

func TestGetPrescriptionForMedicinesByOrders_VitaminDedupeAndNewlyAdded(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	po.products = []pgrepo.ProductDesc{
		{ProductPrincipalID: "44396552913074", MedicineDisplayName: "Hair Vitamin"},
		{ProductPrincipalID: "37547237310642", MedicineDisplayName: "Discontinued Vitamin"},
	}
	mk := func(variants ...int64) orders.Order {
		items := make([]any, 0, len(variants))
		for _, v := range variants {
			items = append(items, map[string]any{"variant_id": float64(v), "quantity": float64(1)})
		}
		return orders.Order{CreatedAt: now, OrderMeta: map[string]any{"line_items": items}}
	}
	// both vitamins present → the discontinued one is dropped before the product lookup
	meds, err := s.GetPrescriptionForMedicinesByOrders(ctx, []orders.Order{mk(44396552913074, 37547237310642)})
	require.NoError(t, err)
	assert.Len(t, meds, 2, "the stub returns both rows; the id list is what gets deduped")

	po.products = []pgrepo.ProductDesc{{ProductPrincipalID: "1", MedicineDisplayName: "A"}, {ProductPrincipalID: "2", MedicineDisplayName: "B"}}
	meds, err = s.GetPrescriptionForMedicinesByOrders(ctx, []orders.Order{mk(1, 2), mk(2)})
	require.NoError(t, err)
	require.Len(t, meds, 2)
	assert.True(t, meds[0].NewlyAdded, "newly added sorts first")
	assert.Equal(t, "A", meds[0].Name)
}

func TestGetMedicinesForHowToUsePurpose(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, po := newTestService(t, now)
	ctx := context.Background()
	po.products = []pgrepo.ProductDesc{{ProductPrincipalID: "41645770309810", MedicineDisplayName: "Recap Serum"}}

	po.byUser = nil
	out, err := s.GetMedicinesForHowToUsePurpose(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, out.IsPrescriptionLocked)
	assert.Empty(t, out.MedicinesPrescription)
	assert.True(t, out.ShowNew)

	o := kitOrder("delivered", day(2026, 9, 10), 1)
	o.OrderDisplayID = "TR-1"
	po.byUser = []orders.Order{o}
	out, err = s.GetMedicinesForHowToUsePurpose(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, out.IsPrescriptionLocked)
	assert.Equal(t, "TR-1", out.LatestOrderDisplayID)
	assert.Equal(t, "delivered", out.LatestOrderStatus)
	assert.Empty(t, out.HowToUseText)
	require.Len(t, out.MedicinesPrescription, 1)

	// single order older than 50 days → Expired
	po.byUser = []orders.Order{kitOrder("delivered", day(2026, 6, 1), 1)}
	out, err = s.GetMedicinesForHowToUsePurpose(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, "Expired", out.HowToUseText)

	// nothing shipped/delivered → locked
	po.byUser = []orders.Order{kitOrder("placed", day(2026, 9, 17), 1)}
	out, err = s.GetMedicinesForHowToUsePurpose(ctx, "u1")
	require.NoError(t, err)
	assert.True(t, out.IsPrescriptionLocked)
}

func TestRecommendationProxies(t *testing.T) {
	now := day(2026, 9, 18)
	s, _, _ := newTestService(t, now)
	var gotPath, gotAuth, gotTenant, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotTenant, gotQuery = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("x-tenant-id"), r.URL.RawQuery
		if r.URL.Path == "/how-to-use/c1" {
			_, _ = w.Write([]byte(`{"products":[1,2]}`))
			return
		}
		if r.URL.Path == "/routine/c404" {
			w.WriteHeader(410)
			_, _ = w.Write([]byte(`{"message":"gone"}`))
			return
		}
		_, _ = w.Write([]byte(`{"routine":true}`))
	}))
	defer srv.Close()
	s.Cfg.RecommendationBaseURL = srv.URL
	s.Cfg.V2FormDataToken = "v2tok"
	s.HTTP.Recommendation = srv.Client()

	res, err := s.LatestOrderHowToUseV2(context.Background(), "c1", url.Values{"orderId": {"o9"}, "junk": {"x"}})
	require.NoError(t, err)
	assert.Equal(t, 200, res.Status)
	assert.Equal(t, "/how-to-use/c1", gotPath)
	assert.Equal(t, "Bearer v2tok", gotAuth)
	assert.Equal(t, "traya", gotTenant)
	assert.Equal(t, "orderId=o9", gotQuery, "only forwarded params are passed")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(res.Body, &payload))
	assert.Contains(t, payload, "reminderInfo")
	assert.Nil(t, payload["reminderInfo"], "null when the case has no reminder log")

	res, err = s.LatestRoutineV2(context.Background(), "c2", url.Values{"productId": {"p1"}})
	require.NoError(t, err)
	assert.Equal(t, 200, res.Status)
	assert.JSONEq(t, `{"routine":true}`, string(res.Body))

	res, err = s.LatestRoutineV2(context.Background(), "c404", nil)
	require.NoError(t, err)
	assert.Equal(t, 410, res.Status, "upstream status passes through")
}
