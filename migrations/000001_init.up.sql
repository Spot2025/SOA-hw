CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    username   VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role       VARCHAR(10) NOT NULL DEFAULT 'USER',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE products (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description VARCHAR(4000),
    price       DECIMAL(12,2) NOT NULL,
    stock       INTEGER NOT NULL DEFAULT 0,
    category    VARCHAR(100) NOT NULL,
    status      VARCHAR(10) NOT NULL DEFAULT 'ACTIVE',
    seller_id   BIGINT REFERENCES users(id),
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_status   ON products(status);
CREATE INDEX idx_products_category ON products(category);

CREATE TABLE promo_codes (
    id               BIGSERIAL PRIMARY KEY,
    code             VARCHAR(20) UNIQUE NOT NULL,
    discount_type    VARCHAR(15) NOT NULL,
    discount_value   DECIMAL(12,2) NOT NULL,
    min_order_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    max_uses         INTEGER NOT NULL,
    current_uses     INTEGER NOT NULL DEFAULT 0,
    valid_from       TIMESTAMP NOT NULL,
    valid_until      TIMESTAMP NOT NULL,
    active           BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE orders (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'CREATED',
    promo_code_id   BIGINT REFERENCES promo_codes(id),
    total_amount    DECIMAL(12,2) NOT NULL DEFAULT 0,
    discount_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_user_status ON orders(user_id, status);

CREATE TABLE order_items (
    id             BIGSERIAL PRIMARY KEY,
    order_id       BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id     BIGINT NOT NULL REFERENCES products(id),
    quantity       INTEGER NOT NULL,
    price_at_order DECIMAL(12,2) NOT NULL
);

CREATE TABLE user_operations (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    operation_type VARCHAR(15) NOT NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_operations_user_type ON user_operations(user_id, operation_type);

-- Auto-update updated_at on row modification
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
