ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS duration_days INTEGER;
