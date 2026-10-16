-- 0006_sales_payment_methods.sql
-- Widen the sales.payment_method CHECK to match the real payment options:
--   GCash, Maribank, Cash, Bank Transfer
-- Removes the old "Maya" value which is no longer offered.

ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_payment_method_check;

ALTER TABLE sales ADD CONSTRAINT sales_payment_method_check
  CHECK (payment_method IN ('Cash','GCash','Maribank','Bank Transfer'));