-- Revert 006_user_profiles. Safe to re-run.
DROP INDEX IF EXISTS idx_user_profiles_completed;
DROP TABLE IF EXISTS user_profiles;
