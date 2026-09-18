package orders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTROrderClient(t *testing.T) {
	var gotPath, gotTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotTenant = r.Header.Get("x-tenant-id")
		_, _ = w.Write([]byte(`{"data":[{"id":"o1","status":"Delivered","delivery_date":"2026-09-01T00:00:00.000Z","created_at":"2026-08-25T00:00:00.000Z","bulk_order_duration":2}]}`))
	}))
	defer srv.Close()
	c := &TROrderClient{HTTP: srv.Client(), BaseURL: srv.URL}
	rows, err := c.NonVoidOrdersByCustomer(context.Background(), "mool", "cust1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/orders/orders/get-all-orders-for-app/cust1?voidType=nonVoid", gotPath)
	assert.Equal(t, "mool", gotTenant)
	o := rows[0].AsOrder()
	assert.Equal(t, "delivered", o.Status, "status lowercased and - → _")
	assert.Equal(t, 2, o.BulkOrderDuration)
	require.NotNil(t, o.DeliveryDate)
	assert.Equal(t, "2026-09-01", o.DeliveryDate.Format("2006-01-02"))

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) }))
	defer bad.Close()
	c2 := &TROrderClient{HTTP: bad.Client(), BaseURL: bad.URL}
	_, err = c2.NonVoidOrdersByCustomer(context.Background(), "mool", "c")
	assert.Error(t, err)
}

func TestConfigServiceClient_CacheAndMapping(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		assert.Equal(t, "/static-content/data/OLD_TO_ACTIVE_VARIANT_IDS_MAP", r.URL.Path)
		_, _ = w.Write([]byte(`{"message":"success","data":{"111":{"associatedTo":"222"}}}`))
	}))
	defer srv.Close()
	c := &ConfigServiceClient{HTTP: srv.Client(), BaseURL: srv.URL, TTL: time.Minute}
	m := c.OldToActiveVariantMap(context.Background(), "traya")
	_ = c.OldToActiveVariantMap(context.Background(), "traya")
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "cached within TTL")
	assert.Equal(t, int64(222), ActiveVariantID(111, m))
	assert.Equal(t, int64(46466574942386), ActiveVariantID(37547358453938, m), "falls back to VARIANT_ID_MAPPING")
	assert.Equal(t, int64(999), ActiveVariantID(999, m), "unmapped id passes through")

	down := &ConfigServiceClient{HTTP: srv.Client(), BaseURL: "http://127.0.0.1:1"}
	assert.Empty(t, down.OldToActiveVariantMap(context.Background(), "traya"), "unreachable → empty map, no error")
}
