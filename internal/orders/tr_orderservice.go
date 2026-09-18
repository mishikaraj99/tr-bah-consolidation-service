package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TRAppOrder is one row of the tr order-service "orders for app" response.
type TRAppOrder struct {
	ID                string `json:"id"`
	CustomerID        string `json:"customer_id"`
	Status            string `json:"status"`
	DeliveryDate      any    `json:"delivery_date"`
	CreatedAt         any    `json:"created_at"`
	BulkOrderDuration any    `json:"bulk_order_duration"`
}

// Time parses a JSON date field (RFC3339 or epoch ms); zero time when unparsable.
func parseAnyTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t.UTC(), true
			}
		}
	case float64:
		return time.UnixMilli(int64(x)).UTC(), true
	}
	return time.Time{}, false
}

// AsOrder maps a tr order-service row onto the shared Order shape.
func (o TRAppOrder) AsOrder() Order {
	out := Order{ID: o.ID, Status: strings.ToLower(strings.ReplaceAll(o.Status, "-", "_")), BulkOrderDuration: 1}
	if t, ok := parseAnyTime(o.CreatedAt); ok {
		out.CreatedAt = t
	}
	if t, ok := parseAnyTime(o.DeliveryDate); ok {
		out.DeliveryDate = &t
	}
	switch b := o.BulkOrderDuration.(type) {
	case float64:
		if int(b) > 0 {
			out.BulkOrderDuration = int(b)
		}
	case string:
		if n, err := strconv.Atoi(b); err == nil && n > 0 {
			out.BulkOrderDuration = n
		}
	}
	return out
}

// TROrderClient calls the mool/acne order service.
type TROrderClient struct {
	HTTP    *http.Client
	BaseURL string
}

func (c *TROrderClient) get(ctx context.Context, tenantID, path string) ([]byte, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("TR_ORDER_SERVICE_BASE_URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-tenant-id", tenantID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("order-service %s returned %d", path, resp.StatusCode)
	}
	return body, nil
}

// unwrap accepts either a bare array or {data: [...]} / {data: {data: [...]}}.
func unwrapList(body []byte) ([]json.RawMessage, error) {
	var direct []json.RawMessage
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, nil
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	if len(wrapper.Data) == 0 {
		return nil, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(wrapper.Data, &list); err == nil {
		return list, nil
	}
	var inner struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(wrapper.Data, &inner); err != nil {
		return nil, err
	}
	return inner.Data, nil
}

// NonVoidOrdersByCustomer mirrors getAllNonVoidOrdersByCaseId.
func (c *TROrderClient) NonVoidOrdersByCustomer(ctx context.Context, tenantID, customerID string) ([]TRAppOrder, error) {
	body, err := c.get(ctx, tenantID, "/orders/orders/get-all-orders-for-app/"+customerID+"?voidType=nonVoid")
	if err != nil {
		return nil, err
	}
	rows, err := unwrapList(body)
	if err != nil {
		return nil, err
	}
	out := make([]TRAppOrder, 0, len(rows))
	for _, r := range rows {
		var o TRAppOrder
		if json.Unmarshal(r, &o) == nil {
			out = append(out, o)
		}
	}
	return out, nil
}

// OrderDetailColumns is the exact column list tr-consumer-backend requests.
const OrderDetailColumns = "orders.customerId,orders.bulkOrderDuration,orderLineItem.quantity,productVariant.productCode,productVariant.name,productVariant.priceAmount,productUsageInstruction.instructionType,productUsageInstruction.dosageCode,productUsageInstruction.dosageDescription,productMedia.mediaType,productMedia.usageType,productMedia.url,product.shortDescription,product.longDescription"

// OrderDetailJoins is the exact joins list tr-consumer-backend requests.
const OrderDetailJoins = "orderLineItem,productVariant,productUsageInstruction,productMedia,product"

// OrderDetails mirrors getAllOrderDetailsByOrderId; the response shape is walked by the caller.
func (c *TROrderClient) OrderDetails(ctx context.Context, tenantID, orderID string) (map[string]any, error) {
	path := "/orders/orders/get-orders-by-order-ids?columns=" + OrderDetailColumns + "&orderIds=" + orderID + "&joins=" + OrderDetailJoins
	body, err := c.get(ctx, tenantID, path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
