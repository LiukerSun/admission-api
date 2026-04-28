ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_duration_days_check;

ALTER TABLE payment_orders
    ALTER COLUMN duration_days DROP NOT NULL;
