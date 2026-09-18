package pgrepo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LogEarnStore accesses a mool/acne tenant database.
type LogEarnStore struct {
	Write, Read *pgxpool.Pool
}

// NewLogEarnStore uses write for reads when read is nil.
func NewLogEarnStore(write, read *pgxpool.Pool) *LogEarnStore {
	if read == nil {
		read = write
	}
	return &LogEarnStore{Write: write, Read: read}
}

// DoseLog is a dose_logs row.
type DoseLog struct {
	ID          string
	CustomerID  string
	OrderID     string
	LogDate     time.Time
	StreakDay   int
	ZTier       int
	INRCredited int
	IsBackfill  bool
	CreatedAt   time.Time
}

// LedgerEntry is a cash_ledger row.
type LedgerEntry struct {
	ID           string
	CustomerID   string
	OrderID      *string
	Amount       int
	Reason       string
	BalanceAfter int
	CreatedAt    time.Time
}

// ErrInsufficientBalance is returned by AppendLedger when a debit exceeds the balance.
var ErrInsufficientBalance = errors.New("Insufficient cash balance")

const doseCols = `id::text, customer_id::text, order_id::text, log_date, streak_day, z_tier, COALESCE(inr_credited,0), COALESCE(is_backfill,false), created_at`

// RecentDoseLogs returns the newest logs (log_date DESC).
func (s *LogEarnStore) RecentDoseLogs(ctx context.Context, customerID string, limit int) ([]DoseLog, error) {
	rows, err := s.Read.Query(ctx, `SELECT `+doseCols+` FROM dose_logs WHERE customer_id = $1 ORDER BY log_date DESC LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DoseLog
	for rows.Next() {
		var d DoseLog
		if err := rows.Scan(&d.ID, &d.CustomerID, &d.OrderID, &d.LogDate, &d.StreakDay, &d.ZTier, &d.INRCredited, &d.IsBackfill, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountDoseLogsSince counts logs with log_date >= since.
func (s *LogEarnStore) CountDoseLogsSince(ctx context.Context, customerID string, since time.Time) (int, error) {
	var n int
	err := s.Read.QueryRow(ctx, `SELECT COUNT(*) FROM dose_logs WHERE customer_id = $1 AND log_date >= $2`, customerID, since).Scan(&n)
	return n, err
}

// InsertDoseLog inserts within tx and fills ID.
func (s *LogEarnStore) InsertDoseLog(ctx context.Context, tx pgx.Tx, d *DoseLog) error {
	return tx.QueryRow(ctx, `INSERT INTO dose_logs (customer_id, order_id, log_date, streak_day, z_tier, inr_credited, is_backfill)
 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text, created_at`, d.CustomerID, d.OrderID, d.LogDate, d.StreakDay, d.ZTier, d.INRCredited, d.IsBackfill).Scan(&d.ID, &d.CreatedAt)
}

// UpdateDoseLogStreakDay sets streak_day on one row.
func (s *LogEarnStore) UpdateDoseLogStreakDay(ctx context.Context, tx pgx.Tx, id string, streakDay int) error {
	_, err := tx.Exec(ctx, `UPDATE dose_logs SET streak_day = $2 WHERE id = $1`, id, streakDay)
	return err
}

func scanLedger(row pgx.Row) (*LedgerEntry, error) {
	var e LedgerEntry
	if err := row.Scan(&e.ID, &e.CustomerID, &e.OrderID, &e.Amount, &e.Reason, &e.BalanceAfter, &e.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

const ledgerCols = `id::text, customer_id::text, order_id::text, amount, reason, balance_after, created_at`

// LatestLedger returns the newest ledger row (nil when none).
func (s *LogEarnStore) LatestLedger(ctx context.Context, customerID string) (*LedgerEntry, error) {
	return scanLedger(s.Read.QueryRow(ctx, `SELECT `+ledgerCols+` FROM cash_ledger WHERE customer_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, customerID))
}

// LatestLedgerByReason returns the newest row with the reason.
func (s *LogEarnStore) LatestLedgerByReason(ctx context.Context, customerID, reason string) (*LedgerEntry, error) {
	return scanLedger(s.Read.QueryRow(ctx, `SELECT `+ledgerCols+` FROM cash_ledger WHERE customer_id = $1 AND reason = $2 ORDER BY created_at DESC LIMIT 1`, customerID, reason))
}

// ExistsLedgerReasonSince reports a row with reason and created_at >= since.
func (s *LogEarnStore) ExistsLedgerReasonSince(ctx context.Context, customerID, reason string, since time.Time) (bool, error) {
	var n int
	err := s.Read.QueryRow(ctx, `SELECT 1 FROM cash_ledger WHERE customer_id = $1 AND reason = $2 AND created_at >= $3 LIMIT 1`, customerID, reason, since).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ExistsRedeemForOrder reports a REDEEM row for the order.
func (s *LogEarnStore) ExistsRedeemForOrder(ctx context.Context, customerID, orderID string) (bool, error) {
	var n int
	err := s.Read.QueryRow(ctx, `SELECT 1 FROM cash_ledger WHERE customer_id = $1 AND reason = 'REDEEM' AND order_id = $2 LIMIT 1`, customerID, orderID).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// WithTx runs fn in a transaction on the writer.
func (s *LogEarnStore) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.Write.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// Balance is the authoritative balance: the sum of every ledger amount for the customer.
// Order-independent on purpose — see AppendLedger.
func (s *LogEarnStore) Balance(ctx context.Context, customerID string) (int, error) {
	var n int
	err := s.Read.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0)::int FROM cash_ledger WHERE customer_id = $1`, customerID).Scan(&n)
	return n, err
}

// AppendLedger mirrors creditCash/debitCash: serialise per customer, compute balance_after, insert.
// amount < 0 debits; a debit below zero returns ErrInsufficientBalance.
//
// The balance is summed rather than read from the newest row's balance_after. `created_at` defaults
// to NOW(), which is the TRANSACTION START time, so under concurrency the newest-by-timestamp row is
// not necessarily the last committed one: three concurrent credits could each read the same prior
// balance and write the same balance_after. Summing is order-independent and cannot drift.
// created_at is written with clock_timestamp() so display ordering follows insertion order.
func (s *LogEarnStore) AppendLedger(ctx context.Context, tx pgx.Tx, customerID string, orderID *string, amount int, reason string) (int, error) {
	// Serialise writers for this customer; the ledger may have no rows yet, so a row lock is not enough.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, customerID); err != nil {
		return 0, err
	}
	var balance int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0)::int FROM cash_ledger WHERE customer_id = $1`, customerID).Scan(&balance); err != nil {
		return 0, err
	}
	if amount < 0 && balance+amount < 0 {
		return balance, ErrInsufficientBalance
	}
	after := balance + amount
	if _, err := tx.Exec(ctx, `INSERT INTO cash_ledger (customer_id, order_id, amount, reason, balance_after, created_at)
 VALUES ($1,$2,$3,$4,$5, clock_timestamp())`, customerID, orderID, amount, reason, after); err != nil {
		return 0, err
	}
	return after, nil
}
