-- 0007_sales_void_reason.sql
-- Records who voided a sale, when, and why.
-- All three columns are nullable so existing void sales stay valid.

ALTER TABLE sales ADD COLUMN IF NOT EXISTS void_reason  text;
ALTER TABLE sales ADD COLUMN IF NOT EXISTS voided_by    bigint REFERENCES users(id);
ALTER TABLE sales ADD COLUMN IF NOT EXISTS voided_at    timestamptz;

-- Small index for "show me all voided sales" style reporting later.
CREATE INDEX IF NOT EXISTS idx_sales_voided_at ON sales (voided_at DESC)
    WHERE voided_at IS NOT NULL;