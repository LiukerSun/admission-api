DROP INDEX IF EXISTS idx_membership_plans_sort;

ALTER TABLE membership_plans
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS sort_order;
