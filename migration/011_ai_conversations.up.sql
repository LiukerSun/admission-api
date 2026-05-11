CREATE TABLE IF NOT EXISTS ai_conversations (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       VARCHAR(200) NOT NULL DEFAULT '',
    model_name  VARCHAR(100) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_conversations_user_updated ON ai_conversations(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS ai_conversation_messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            VARCHAR(16) NOT NULL CHECK (role IN ('user','assistant','system','tool')),
    content         TEXT NOT NULL DEFAULT '',
    tool_calls      JSONB NOT NULL DEFAULT '[]'::jsonb,
    tool_results    JSONB NOT NULL DEFAULT '[]'::jsonb,
    widgets         JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_messages_conv_created_id ON ai_conversation_messages(conversation_id, created_at, id);
