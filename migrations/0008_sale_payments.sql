CREATE TABLE IF NOT EXISTS sale_payments (
    id         BIGSERIAL PRIMARY KEY,
    sale_id    BIGINT NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    method     TEXT NOT NULL CHECK (method IN ('Cash','GCash','Maribank','Bank Transfer')),
    amount     NUMERIC(10,2) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sale_payments_sale_id ON sale_payments(sale_id);