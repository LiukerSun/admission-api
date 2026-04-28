UPDATE payment_orders po
SET duration_days = mp.duration_days
FROM membership_plans mp
WHERE po.product_ref_id = mp.id
  AND po.duration_days IS NULL;

ALTER TABLE payment_orders
    ALTER COLUMN duration_days SET NOT NULL;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_duration_days_check CHECK (duration_days > 0);
