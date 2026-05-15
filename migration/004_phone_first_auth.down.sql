-- Revert 002_phone_first_auth. Restoring NOT NULL fails if any row has NULL
-- email; callers must backfill before rolling back.

DROP INDEX IF EXISTS idx_users_email;

ALTER TABLE users
    ALTER COLUMN email SET NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT users_email_key UNIQUE (email);
