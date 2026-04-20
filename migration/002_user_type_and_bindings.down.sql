DROP INDEX IF EXISTS idx_user_bindings_student_id;
DROP INDEX IF EXISTS idx_user_bindings_parent_id;
DROP TABLE IF EXISTS user_bindings;
ALTER TABLE users DROP COLUMN IF EXISTS user_type;
