package setup

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectPG builds a pgx pool for c and pings it.
func ConnectPG(ctx context.Context, c PGConfig) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=prefer",
		url.QueryEscape(c.User), url.QueryEscape(c.Password), c.Host, c.Port, c.Database)
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if c.MinConns > 0 {
		pc.MinConns = c.MinConns
	}
	if c.MaxConns > 0 {
		pc.MaxConns = c.MaxConns
	}
	pc.MaxConnIdleTime = 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
