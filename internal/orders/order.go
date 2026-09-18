// Package orders holds the kit/order math shared by every economy, plus order sources.
package orders

import (
	"strconv"
	"time"

	pgrepo "traya-bah-service/repositories/pg"
)

// Order is the shared order projection (Traya Postgres rows and mool/acne order-service rows map into it).
type Order = pgrepo.Order

// LineItem is one order_meta.line_items entry.
type LineItem struct {
	VariantID int64
	ProductID string
	Quantity  int
	Name      string
}

// LineItems extracts order_meta.line_items; quantity defaults to 1, name = name || title.
func LineItems(o Order) []LineItem {
	raw, _ := o.OrderMeta["line_items"].([]any)
	out := make([]LineItem, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		li := LineItem{VariantID: toInt64(m["variant_id"]), Quantity: 1}
		if q := toInt64(m["quantity"]); q > 0 {
			li.Quantity = int(q)
		}
		switch v := m["product_id"].(type) {
		case string:
			li.ProductID = v
		case float64:
			li.ProductID = strconv.FormatInt(int64(v), 10)
		}
		if n, ok := m["name"].(string); ok && n != "" {
			li.Name = n
		} else if t, ok := m["title"].(string); ok {
			li.Name = t
		}
		out = append(out, li)
	}
	return out
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

// AnchorDate is delivery_date || created_at.
func AnchorDate(o Order) time.Time {
	if o.DeliveryDate != nil {
		return *o.DeliveryDate
	}
	return o.CreatedAt
}

// LastDelivered mirrors habitTrackerLastDelivered: delivered order with the latest anchor date.
func LastDelivered(orders []Order) *Order {
	var best *Order
	for i := range orders {
		o := &orders[i]
		if o.Status != "delivered" {
			continue
		}
		if best == nil || AnchorDate(*o).After(AnchorDate(*best)) {
			best = o
		}
	}
	return best
}
