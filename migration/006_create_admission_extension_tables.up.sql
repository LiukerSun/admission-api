CREATE TABLE IF NOT EXISTS admission_group_extensions (
    id BIGSERIAL PRIMARY KEY,
    admission_group_id BIGINT NOT NULL UNIQUE REFERENCES admission_groups(id) ON DELETE CASCADE,
    batch_remark TEXT,
    group_min_score INTEGER,
    group_min_rank INTEGER,
    equivalent_min_score_2024 INTEGER,
    equivalent_min_score_2023 INTEGER,
    equivalent_min_score_2022 INTEGER,
    subject_change_2024 TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admission_group_extensions_score
    ON admission_group_extensions(group_min_rank, group_min_score);

CREATE TABLE IF NOT EXISTS university_major_profiles (
    id BIGSERIAL PRIMARY KEY,
    university_major_admission_id BIGINT NOT NULL UNIQUE REFERENCES university_major_admissions(id) ON DELETE CASCADE,
    discipline_category VARCHAR(100),
    first_level_discipline VARCHAR(200),
    fourth_round_subject_eval VARCHAR(100),
    double_first_class_subject TEXT,
    soft_major_grade VARCHAR(100),
    major_evaluation_score NUMERIC(6,2),
    major_rank VARCHAR(100),
    is_national_feature BOOLEAN,
    corresponding_master_majors TEXT,
    corresponding_doctoral_majors TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_university_major_profiles_discipline
    ON university_major_profiles(discipline_category, first_level_discipline);

CREATE INDEX IF NOT EXISTS idx_university_major_profiles_rank
    ON university_major_profiles(major_rank);

CREATE TABLE IF NOT EXISTS university_postgraduate_profiles (
    id BIGSERIAL PRIMARY KEY,
    university_id BIGINT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    profile_year INTEGER NOT NULL,
    master_major_count INTEGER,
    master_major_names TEXT,
    doctoral_major_count INTEGER,
    doctoral_major_names TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_id, profile_year)
);

CREATE INDEX IF NOT EXISTS idx_university_postgraduate_profiles_year
    ON university_postgraduate_profiles(university_id, profile_year DESC);
