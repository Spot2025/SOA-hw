DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders;
DROP TRIGGER IF EXISTS trg_products_updated_at ON products;
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS user_operations;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS promo_codes;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS users;
