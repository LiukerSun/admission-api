CREATE TABLE IF NOT EXISTS candidate_profiles (
    id                      BIGSERIAL PRIMARY KEY,

    user_id                 BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    real_name               VARCHAR(50) NOT NULL,
    candidate_id_card_enc   BYTEA,
    candidate_id_card_hash  CHAR(64),
    candidate_phone         VARCHAR(20),

    province_id             INTEGER NOT NULL,
    city_id                 INTEGER,
    county_id               INTEGER,

    graduation_school_name  VARCHAR(128),

    grade                   SMALLINT NOT NULL DEFAULT 3,
    candidate_type          VARCHAR(16) NOT NULL DEFAULT 'regular',

    gender                  VARCHAR(8),
    ethnicity               VARCHAR(32),
    color_vision            VARCHAR(16),

    status                  VARCHAR(16) NOT NULL DEFAULT 'active',
    is_deleted              BOOLEAN NOT NULL DEFAULT false,
    deleted_at              TIMESTAMPTZ,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_candidate_profiles_user_id
    ON candidate_profiles(user_id) WHERE is_deleted = false;

CREATE INDEX idx_candidate_profiles_id_card_hash
    ON candidate_profiles(candidate_id_card_hash) WHERE is_deleted = false;

CREATE INDEX idx_candidate_profiles_phone
    ON candidate_profiles(candidate_phone) WHERE is_deleted = false;

CREATE INDEX idx_candidate_profiles_province
    ON candidate_profiles(province_id);

CREATE INDEX idx_candidate_profiles_status
    ON candidate_profiles(status);
