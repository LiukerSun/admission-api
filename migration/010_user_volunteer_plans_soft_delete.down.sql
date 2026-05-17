DROP INDEX IF EXISTS idx_user_volunteer_plans_alive;
ALTER TABLE user_volunteer_plans DROP COLUMN IF EXISTS deleted_at;
