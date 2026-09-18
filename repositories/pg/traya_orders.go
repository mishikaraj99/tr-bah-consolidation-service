// Package pgrepo holds Postgres access: Traya's api-server database and the mool/acne tenant databases.
package pgrepo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TrayaStore reads the api-server Postgres database.
type TrayaStore struct{ Pool *pgxpool.Pool }

// Order is an orders row projection shared by every economy.
type Order struct {
	ID                string
	UserID            string
	CaseID            string
	Status            string
	CreatedAt         time.Time
	DeliveryDate      *time.Time
	OrderMeta         map[string]any
	IsBulkOrder       bool
	BulkOrderDuration int
	OrderDisplayID    string
}

const orderCols = `id, COALESCE(user_id::text,''), COALESCE(case_id::text,''), COALESCE(status,''), created_at, delivery_date, order_meta, COALESCE(is_bulk_order,false), COALESCE(bulk_order_duration,0), COALESCE(order_display_id::text,'')`

func scanOrders(rows pgx.Rows) ([]Order, error) {
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		var meta []byte
		if err := rows.Scan(&o.ID, &o.UserID, &o.CaseID, &o.Status, &o.CreatedAt, &o.DeliveryDate, &meta, &o.IsBulkOrder, &o.BulkOrderDuration, &o.OrderDisplayID); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &o.OrderMeta)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// NonVoidOrdersByUser mirrors getAllNonVoidOrdersByUserIdWithSelectedColumns (status != void, created_at DESC).
func (s *TrayaStore) NonVoidOrdersByUser(ctx context.Context, userID string) ([]Order, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE user_id = $1 AND status <> 'void' ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanOrders(rows)
}

// NonVoidOrdersByCase is the same list keyed by case.
func (s *TrayaStore) NonVoidOrdersByCase(ctx context.Context, caseID string) ([]Order, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE case_id = $1 AND status <> 'void' ORDER BY created_at DESC`, caseID)
	if err != nil {
		return nil, err
	}
	return scanOrders(rows)
}

// LatestNonVoidUnknownOrders mirrors getLatestNonVoidUnknowOrdersByUserId (status NOT IN void/unknown/ghost).
func (s *TrayaStore) LatestNonVoidUnknownOrders(ctx context.Context, userID string) ([]Order, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE user_id = $1 AND status NOT IN ('void','unknown','ghost') ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanOrders(rows)
}

// FirstDeliveredDate returns the earliest delivery_date of a delivered order (nil when none).
func (s *TrayaStore) FirstDeliveredDate(ctx context.Context, userID string) (*time.Time, error) {
	var d *time.Time
	err := s.Pool.QueryRow(ctx, `SELECT delivery_date FROM orders WHERE user_id = $1 AND status = 'delivered' ORDER BY delivery_date ASC NULLS LAST LIMIT 1`, userID).Scan(&d)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// LatestOrderAndCount mirrors hasUserSeenbahUpdatedModal's two queries.
func (s *TrayaStore) LatestOrderAndCount(ctx context.Context, userID string) (*Order, int, error) {
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE user_id = $1`, userID).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE user_id = $1 AND status <> 'void' ORDER BY created_at DESC LIMIT 1`, userID)
	if err != nil {
		return nil, 0, err
	}
	list, err := scanOrders(rows)
	if err != nil || len(list) == 0 {
		return nil, count, err
	}
	return &list[0], count, nil
}
