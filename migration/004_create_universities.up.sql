CREATE TABLE IF NOT EXISTS universities (
    id BIGSERIAL PRIMARY KEY,
    university_code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    normalized_name VARCHAR(200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_code, name)
);

CREATE INDEX IF NOT EXISTS idx_universities_name
    ON universities(name);

CREATE TABLE IF NOT EXISTS university_profiles (
    id BIGSERIAL PRIMARY KEY,
    university_id BIGINT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    profile_year INTEGER NOT NULL,
    region_code VARCHAR(20) REFERENCES regions(code),
    city VARCHAR(100),
    ownership_type_code VARCHAR(50) REFERENCES school_ownership_types(code),
    school_category_code VARCHAR(50) REFERENCES school_categories(code),
    education_level_code VARCHAR(50) REFERENCES education_levels(code),
    is_985 BOOLEAN,
    is_211 BOOLEAN,
    is_double_first_class BOOLEAN,
    is_national_key BOOLEAN,
    is_provincial_key BOOLEAN,
    has_postgraduate_recommendation BOOLEAN,
    postgraduate_recommendation_rate NUMERIC(5,2),
    soft_rank VARCHAR(100),
    alumni_rank VARCHAR(100),
    difficulty_rank VARCHAR(100),
    doctoral_program_count INTEGER,
    master_program_count INTEGER,
    national_key_subject_count INTEGER,
    affiliation VARCHAR(200),
    school_level_tags TEXT,
    excellence_tags TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_id, profile_year)
);

CREATE INDEX IF NOT EXISTS idx_university_profiles_year
    ON university_profiles(university_id, profile_year DESC);
