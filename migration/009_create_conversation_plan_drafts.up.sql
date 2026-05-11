CREATE TABLE IF NOT EXISTS conversation_plan_drafts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'generating',
    input_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    plan_json JSONB,
    algorithm_version VARCHAR(64) NOT NULL DEFAULT '',
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_plan_drafts_user_created
    ON conversation_plan_drafts(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_plan_drafts_conversation_created
    ON conversation_plan_drafts(conversation_id, created_at DESC);

