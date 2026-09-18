package pgrepo

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CustomerStore reads the tenant `customer` table (mool/acne).
type CustomerStore struct{ Pool *pgxpool.Pool }

// Gender mirrors getGenderByCustomerId: customer.gender or "M".
func (s *CustomerStore) Gender(ctx context.Context, customerID string) (string, error) {
	var g *string
	err := s.Pool.QueryRow(ctx, `SELECT gender FROM customer WHERE id = $1 LIMIT 1`, customerID).Scan(&g)
	if err == pgx.ErrNoRows || (err == nil && (g == nil || *g == "")) {
		return "M", nil
	}
	if err != nil {
		return "M", err
	}
	return *g, nil
}
