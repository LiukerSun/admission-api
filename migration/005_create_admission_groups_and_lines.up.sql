CREATE TABLE IF NOT EXISTS admission_groups (
    id BIGSERIAL PRIMARY KEY,
    university_id BIGINT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    admission_year INTEGER NOT NULL,
    region_code VARCHAR(20) NOT NULL REFERENCES regions(code),
    subject_category_code VARCHAR(50) NOT NULL REFERENCES subject_categories(code),
    batch_code VARCHAR(50) NOT NULL REFERENCES batches(code),
    group_code VARCHAR(50) NOT NULL,
    subject_requirement_code VARCHAR(50) REFERENCES subject_requirements(code),
    education_level_code VARCHAR(50) REFERENCES education_levels(code),
    group_major_count INTEGER,
    group_major_names TEXT,
    group_type VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (
        university_id,
        admission_year,
        region_code,
        subject_category_code,
        batch_code,
        group_code
    )
);

CREATE INDEX IF NOT EXISTS idx_admission_groups_lookup
    ON admission_groups(admission_year, region_code, subject_category_code, university_id, group_code);

CREATE INDEX IF NOT EXISTS idx_admission_groups_university_year
    ON admission_groups(university_id, admission_year DESC);

CREATE TABLE IF NOT EXISTS university_major_admissions (
    id BIGSERIAL PRIMARY KEY,
    admission_group_id BIGINT NOT NULL REFERENCES admission_groups(id) ON DELETE CASCADE,
    local_major_code VARCHAR(50) NOT NULL,
    local_major_name TEXT NOT NULL,
    plan_count INTEGER,
    admitted_count INTEGER,
    min_score INTEGER,
    min_rank INTEGER,
    max_score INTEGER,
    max_rank INTEGER,
    equivalent_min_score INTEGER,
    tuition INTEGER,
    duration VARCHAR(50),
    admission_remark TEXT,
    major_intro TEXT,
    training_goal TEXT,
    subject_study_requirement TEXT,
    main_courses TEXT,
    postgraduate_direction TEXT,
    employment_direction TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (admission_group_id, local_major_code)
);

CREATE INDEX IF NOT EXISTS idx_major_admissions_group
    ON university_major_admissions(admission_group_id, local_major_code);

CREATE INDEX IF NOT EXISTS idx_major_admissions_local_name
    ON university_major_admissions(local_major_name);

CREATE INDEX IF NOT EXISTS idx_major_admissions_rank
    ON university_major_admissions(min_rank, min_score);

CREATE TABLE IF NOT EXISTS admission_major_tags (
    id BIGSERIAL PRIMARY KEY,
    university_major_admission_id BIGINT NOT NULL REFERENCES university_major_admissions(id) ON DELETE CASCADE,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    category_name VARCHAR(100) NOT NULL,
    class_code VARCHAR(20),
    class_name VARCHAR(100),
    major_code VARCHAR(20),
    major_name VARCHAR(200),
    standard_major_id BIGINT REFERENCES standard_majors(id),
    tag_level VARCHAR(20) NOT NULL
        CHECK (tag_level IN ('category', 'class', 'major')),
    note TEXT,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (
        university_major_admission_id,
        catalog_year,
        category_code,
        class_code,
        major_code
    )
);

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_category
    ON admission_major_tags(catalog_year, category_code);

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_class
    ON admission_major_tags(catalog_year, class_code)
    WHERE class_code IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_major
    ON admission_major_tags(catalog_year, major_code)
    WHERE major_code IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_standard_major
    ON admission_major_tags(standard_major_id)
    WHERE standard_major_id IS NOT NULL;
