-- Log & Earn tables for mool/acne tenant databases (mirrors tr-consumer-backend TypeORM entities).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS dose_logs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id   uuid NOT NULL,
    order_id      uuid NOT NULL,
    log_date      timestamp NOT NULL,
    streak_day    integer NOT NULL,
    z_tier        integer NOT NULL,
    inr_credited  integer DEFAULT 0,
    is_backfill   boolean DEFAULT false,
    created_at    timestamp DEFAULT NOW()
);
COMMENT ON COLUMN dose_logs.log_date IS 'Stored in UTC. IST conversion done in application code.';
COMMENT ON COLUMN dose_logs.created_at IS 'Stored in UTC.';
CREATE UNIQUE INDEX IF NOT EXISTS "UQ_dose_logs_customer_date" ON dose_logs (customer_id, log_date);

CREATE TABLE IF NOT EXISTS cash_ledger (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id    uuid NOT NULL,
    order_id       uuid NULL,
    amount         integer NOT NULL,
    reason         varchar NOT NULL,
    balance_after  integer NOT NULL,
    created_at     timestamp DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS "IDX_cash_ledger_customer_id" ON cash_ledger (customer_id);
CREATE INDEX IF NOT EXISTS "IDX_cash_ledger_customer_reason" ON cash_ledger (customer_id, reason);
-- spec §4: one REDEEM per order
CREATE UNIQUE INDEX IF NOT EXISTS uq_cash_ledger_redeem_order ON cash_ledger (customer_id, order_id) WHERE reason = 'REDEEM' AND order_id IS NOT NULL;
