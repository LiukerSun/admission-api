CREATE TABLE IF NOT EXISTS user_volunteer_plans (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL DEFAULT '',
    source_draft_id BIGINT REFERENCES conversation_plan_drafts(id) ON DELETE SET NULL,
    plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, source_draft_id)
);

CREATE INDEX IF NOT EXISTS idx_user_volunteer_plans_user_created
    ON user_volunteer_plans(user_id, created_at DESC);

