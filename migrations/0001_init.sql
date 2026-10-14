CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin', 'staff')),
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products (
    id            BIGSERIAL PRIMARY KEY,
    sku           TEXT UNIQUE NOT NULL,
    barcode       TEXT,
    name          TEXT NOT NULL,
    brand         TEXT,
    version       TEXT,
    flavor        TEXT,
    category      TEXT,
    cost_price    NUMERIC(10,2) NOT NULL DEFAULT 0,
    selling_price NUMERIC(10,2) NOT NULL DEFAULT 0,
    minimum_stock INTEGER NOT NULL DEFAULT 0,
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inventory (
    product_id    BIGINT PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    current_stock INTEGER NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS suppliers (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT NOT NULL,
    contact_person TEXT,
    phone          TEXT,
    email          TEXT,
    address        TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS stock_movements (
    id            BIGSERIAL PRIMARY KEY,
    product_id    BIGINT NOT NULL REFERENCES products(id),
    user_id       BIGINT REFERENCES users(id),
    supplier_id   BIGINT REFERENCES suppliers(id),
    type          TEXT NOT NULL CHECK (type IN ('in', 'out', 'adjust')),
    quantity      INTEGER NOT NULL,
    unit_cost     NUMERIC(10,2),
    reference     TEXT,
    notes         TEXT,
    movement_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sales (
    id             BIGSERIAL PRIMARY KEY,
    reference      TEXT UNIQUE NOT NULL,
    user_id        BIGINT REFERENCES users(id),
    sale_date      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    subtotal       NUMERIC(10,2) NOT NULL DEFAULT 0,
    discount       NUMERIC(10,2) NOT NULL DEFAULT 0,
    total          NUMERIC(10,2) NOT NULL DEFAULT 0,
    payment_method TEXT NOT NULL CHECK (payment_method IN ('Cash','GCash','Maribank','Maya')),
    status         TEXT NOT NULL DEFAULT 'completed' CHECK (status IN ('pending','completed','void')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sale_items (
    id           BIGSERIAL PRIMARY KEY,
    sale_id      BIGINT NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    product_id   BIGINT NOT NULL REFERENCES products(id),
    sku          TEXT NOT NULL,
    product_name TEXT NOT NULL,
    quantity     INTEGER NOT NULL,
    unit_price   NUMERIC(10,2) NOT NULL,
    subtotal     NUMERIC(10,2) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sales_sale_date ON sales(sale_date DESC);
CREATE INDEX IF NOT EXISTS idx_stock_movements_product ON stock_movements(product_id);