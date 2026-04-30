CREATE TABLE IF NOT EXISTS candidate_exam_records (
    id              BIGSERIAL PRIMARY KEY,
    profile_id      BIGINT NOT NULL REFERENCES candidate_profiles(id) ON DELETE CASCADE,
    exam_year       SMALLINT NOT NULL,
    exam_model      VARCHAR(16) NOT NULL,
    exam_type       VARCHAR(16) NOT NULL DEFAULT 'tongkao',
    total_score     NUMERIC(6, 2),
    rank_value      INTEGER,
    section_type    VARCHAR(16),
    select_subjects JSONB,
    subject_scores  JSONB,
    art_score       NUMERIC(6, 2),
    culture_score   NUMERIC(6, 2),
    art_type        VARCHAR(16),
    is_current      BOOLEAN NOT NULL DEFAULT true,
    verified        BOOLEAN NOT NULL DEFAULT false,
    status          VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_exam_records_current
    ON candidate_exam_records(profile_id) WHERE is_current = true;
CREATE INDEX idx_exam_records_profile
    ON candidate_exam_records(profile_id);
CREATE INDEX idx_exam_records_year
    ON candidate_exam_records(exam_year);
CREATE INDEX idx_exam_records_model_section
    ON candidate_exam_records(exam_model, section_type);

CREATE TABLE IF NOT EXISTS candidate_score_histories (
    id                   BIGSERIAL PRIMARY KEY,
    exam_record_id       BIGINT NOT NULL REFERENCES candidate_exam_records(id) ON DELETE CASCADE,
    prev_total_score     NUMERIC(6, 2),
    prev_rank_value      INTEGER,
    prev_subject_scores  JSONB,
    prev_select_subjects JSONB,
    new_total_score      NUMERIC(6, 2) NOT NULL,
    new_rank_value       INTEGER NOT NULL,
    new_subject_scores   JSONB,
    new_select_subjects  JSONB,
    change_reason        VARCHAR(255),
    source               VARCHAR(32) DEFAULT 'manual',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_score_histories_record
    ON candidate_score_histories(exam_record_id);
CREATE INDEX idx_score_histories_created
    ON candidate_score_histories(created_at);
