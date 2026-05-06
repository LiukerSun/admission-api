-- Extend users.role CHECK constraint to include planner, candidate, merchant
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'premium', 'admin', 'planner', 'candidate', 'merchant'));
