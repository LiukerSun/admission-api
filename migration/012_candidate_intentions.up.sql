CREATE TABLE IF NOT EXISTS candidate_intentions (
    id              BIGSERIAL PRIMARY KEY,
    profile_id      BIGINT NOT NULL REFERENCES candidate_profiles(id) ON DELETE CASCADE,
    intention_type  VARCHAR(16) NOT NULL,
    target_id       VARCHAR(50) NOT NULL,
    target_name     VARCHAR(100),
    priority        INTEGER NOT NULL DEFAULT 0,
    notes           VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, intention_type, target_id)
);

CREATE INDEX idx_intentions_profile
    ON candidate_intentions(profile_id);

CREATE INDEX idx_intentions_profile_type
    ON candidate_intentions(profile_id, intention_type);

CREATE INDEX idx_intentions_target
    ON candidate_intentions(intention_type, target_id);
