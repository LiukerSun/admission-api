-- Membership plan admin management:
-- - sort_order lets the admin manually reorder how plans are displayed to users
--   (the public-facing list now orders by sort_order, then duration_days as
--   tiebreaker, so existing seed rows render identically until sort_order is
--   edited).
-- - description holds the marketing copy admins curate per plan (shown on the
--   pricing card / paywall modal); empty string default keeps existing rows
--   valid without a backfill.

ALTER TABLE membership_plans
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_membership_plans_sort
    ON membership_plans(sort_order, duration_days);
