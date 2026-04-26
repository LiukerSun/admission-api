CREATE TABLE IF NOT EXISTS candidate_activity_logs (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_type   VARCHAR(32) NOT NULL,
    target_type     VARCHAR(32),
    target_id       BIGINT,
    metadata        JSONB NOT NULL DEFAULT '{}',
    ip_address      INET,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_logs_user_created ON candidate_activity_logs(user_id, created_at DESC);
CREATE INDEX idx_activity_logs_type_created ON candidate_activity_logs(activity_type, created_at DESC);
CREATE INDEX idx_activity_logs_target ON candidate_activity_logs(target_type, target_id) WHERE target_type IS NOT NULL;
