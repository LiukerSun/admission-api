CREATE SCHEMA IF NOT EXISTS gaokao;

CREATE TABLE IF NOT EXISTS gaokao.data_source (
    source_code             text PRIMARY KEY,
    source_name             text NOT NULL,
    description             text,
    created_at              timestamptz NOT NULL DEFAULT now()
);

INSERT INTO gaokao.data_source (source_code, source_name, description)
VALUES
    ('xyl', 'xyl_public_data_xyl', '结构化较规整，适合做主数据'),
    ('rzy', 'rzy_365_zr', '字段更丰富，适合补扩展信息')
ON CONFLICT (source_code) DO NOTHING;

CREATE TABLE IF NOT EXISTS gaokao.province (
    province_id             integer PRIMARY KEY,
    province_name           varchar(32) NOT NULL,
    initial                 varchar(8),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gaokao_province_name
    ON gaokao.province (province_name);

CREATE TABLE IF NOT EXISTS gaokao.city (
    city_code               integer PRIMARY KEY,
    city_name               varchar(64) NOT NULL,
    province_id             integer REFERENCES gaokao.province (province_id),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_city_province
    ON gaokao.city (province_id);

CREATE TABLE IF NOT EXISTS gaokao.school (
    school_id               bigint PRIMARY KEY,
    school_name             varchar(255) NOT NULL,
    school_code             varchar(32),
    province_id             integer REFERENCES gaokao.province (province_id),
    city_code               integer REFERENCES gaokao.city (city_code),
    logo_url                text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_province
    ON gaokao.school (province_id);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_city
    ON gaokao.school (city_code);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_name
    ON gaokao.school (school_name);

CREATE TABLE IF NOT EXISTS gaokao.major (
    major_id                bigserial PRIMARY KEY,
    major_code              varchar(32),
    major_name              varchar(255) NOT NULL,
    major_subject           varchar(64),
    major_category          varchar(128),
    degree_name             varchar(64),
    study_years_text        varchar(32),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_major_code
    ON gaokao.major (major_code)
    WHERE major_code IS NOT NULL AND major_code <> '';

CREATE INDEX IF NOT EXISTS idx_gaokao_major_name
    ON gaokao.major (major_name);

CREATE TABLE IF NOT EXISTS gaokao.school_policy_tag (
    school_policy_tag_id    bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    tag_type                varchar(64) NOT NULL,
    tag_value               varchar(255) NOT NULL,
    effective_year          smallint NOT NULL,
    expire_year             smallint,
    source_system           text REFERENCES gaokao.data_source (source_code),
    source_url              text,
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, tag_type, effective_year, tag_value)
);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_policy_tag_lookup
    ON gaokao.school_policy_tag (school_id, tag_type, effective_year, expire_year);

CREATE TABLE IF NOT EXISTS gaokao.major_policy_tag (
    major_policy_tag_id     bigserial PRIMARY KEY,
    major_id                bigint NOT NULL REFERENCES gaokao.major (major_id) ON DELETE CASCADE,
    tag_type                varchar(64) NOT NULL,
    tag_value               varchar(255) NOT NULL,
    effective_year          smallint NOT NULL,
    expire_year             smallint,
    source_system           text REFERENCES gaokao.data_source (source_code),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (major_id, tag_type, effective_year, tag_value)
);

CREATE INDEX IF NOT EXISTS idx_gaokao_major_policy_tag_lookup
    ON gaokao.major_policy_tag (major_id, tag_type, effective_year, expire_year);

CREATE TABLE IF NOT EXISTS gaokao.school_profile (
    school_id                bigint PRIMARY KEY REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    alias_name               varchar(255),
    former_name              varchar(255),
    founded_year             smallint,
    address                  text,
    postcode                 varchar(16),
    website_url              text,
    admission_site_url       text,
    phone                    text,
    email                    text,
    area_square_meter        numeric(14, 2),
    description              text,
    labels                   text,
    campus_scenery           jsonb,
    learning_index           numeric(5, 2),
    life_index               numeric(5, 2),
    employment_index         numeric(5, 2),
    composite_score          numeric(5, 2),
    employment_rate          numeric(5, 2),
    male_ratio               numeric(5, 2),
    female_ratio             numeric(5, 2),
    china_rate               numeric(5, 2),
    abroad_rate              numeric(5, 2),
    hostel_text              text,
    canteen_text             text,
    extra_payload            jsonb,
    source_system            text REFERENCES gaokao.data_source (source_code),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gaokao.school_ranking (
    school_ranking_id       bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    ranking_source          varchar(64) NOT NULL,
    ranking_year            smallint NOT NULL DEFAULT 0,
    rank_value              integer NOT NULL,
    source_system           text REFERENCES gaokao.data_source (source_code),
    created_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, ranking_source, ranking_year)
);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_ranking_school
    ON gaokao.school_ranking (school_id, ranking_source);

CREATE TABLE IF NOT EXISTS gaokao.major_profile (
    major_id                bigint PRIMARY KEY REFERENCES gaokao.major (major_id) ON DELETE CASCADE,
    intro_text              text,
    course_text             text,
    job_text                text,
    select_suggests         text,
    average_salary          numeric(12, 2),
    fresh_average_salary    numeric(12, 2),
    salary_infos            jsonb,
    work_areas              jsonb,
    work_industries         jsonb,
    work_jobs               jsonb,
    extra_payload           jsonb,
    source_system           text REFERENCES gaokao.data_source (source_code),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gaokao.school_major_catalog (
    school_major_id         bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    major_id                bigint REFERENCES gaokao.major (major_id),
    major_code              varchar(32),
    major_name              varchar(255),
    school_major_name       varchar(255) NOT NULL,
    study_years_text        varchar(32),
    observed_year           smallint,
    source_system           text REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128),
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_major_catalog_school
    ON gaokao.school_major_catalog (school_id);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_major_catalog_major
    ON gaokao.school_major_catalog (major_id);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_major_catalog_code
    ON gaokao.school_major_catalog (major_code);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_school_major_catalog_business
    ON gaokao.school_major_catalog (
        school_id,
        COALESCE(major_code, ''),
        school_major_name,
        COALESCE(observed_year, 0),
        COALESCE(source_system, ''),
        COALESCE(source_table, '')
    );

CREATE TABLE IF NOT EXISTS gaokao.admission_policy (
    policy_id               bigserial PRIMARY KEY,
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    policy_year             smallint NOT NULL,
    exam_model              varchar(16) NOT NULL,
    volunteer_mode          varchar(32),
    batch_settings          jsonb,
    has_major_group         boolean DEFAULT false,
    has_parallel_vol        boolean DEFAULT true,
    scoreline_type          varchar(32),
    policy_note             text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (province_id, policy_year)
);

CREATE INDEX IF NOT EXISTS idx_gaokao_admission_policy_lookup
    ON gaokao.admission_policy (province_id, policy_year);

CREATE TABLE IF NOT EXISTS gaokao.admission_type_dim (
    admission_type_id       bigserial PRIMARY KEY,
    admission_type_name     varchar(128) NOT NULL UNIQUE,
    description             text,
    created_at              timestamptz NOT NULL DEFAULT now()
);

INSERT INTO gaokao.admission_type_dim (admission_type_name, description)
VALUES
    ('普通计划', '常规招生计划'),
    ('国家专项计划', '国家专项计划'),
    ('地方专项计划', '地方专项计划'),
    ('高校专项计划', '高校专项计划'),
    ('强基计划', '强基计划'),
    ('综合评价', '综合评价招生'),
    ('保送生', '保送生招生'),
    ('高水平运动队', '高水平运动队'),
    ('高水平艺术团', '高水平艺术团'),
    ('少数民族预科', '少数民族预科'),
    ('定向就业', '定向就业'),
    ('公费师范生', '公费师范生'),
    ('优师专项', '优师专项')
ON CONFLICT (admission_type_name) DO NOTHING;

CREATE TABLE IF NOT EXISTS gaokao.subject_requirement_dim (
    subject_req_id          bigserial PRIMARY KEY,
    raw_requirement         varchar(255) NOT NULL UNIQUE,
    first_subject           varchar(32),
    second_subjects         varchar(128),
    is_new_gaokao           boolean,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gaokao.school_major_group (
    school_major_group_id   bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    group_year              smallint NOT NULL,
    group_name              varchar(128) NOT NULL,
    group_code              varchar(64),
    subject_req_id          bigint REFERENCES gaokao.subject_requirement_dim (subject_req_id),
    group_plan_count        integer,
    lowest_score            numeric(6, 2),
    lowest_rank             integer,
    source_system           text REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128),
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_major_group_lookup
    ON gaokao.school_major_group (school_id, province_id, group_year);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_school_major_group_business
    ON gaokao.school_major_group (
        school_id,
        province_id,
        group_year,
        COALESCE(group_code, ''),
        group_name,
        COALESCE(source_system, ''),
        COALESCE(source_table, '')
    );

CREATE TABLE IF NOT EXISTS gaokao.school_admission_score_fact (
    school_admission_score_id bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    policy_id               bigint NOT NULL REFERENCES gaokao.admission_policy (policy_id),
    school_major_group_id   bigint REFERENCES gaokao.school_major_group (school_major_group_id),
    admission_year          smallint NOT NULL,
    raw_batch_name          varchar(128),
    raw_section_name        varchar(128),
    raw_admission_type      varchar(128),
    raw_major_group_name    varchar(255),
    raw_elective_req        varchar(255),
    highest_score           numeric(6, 2),
    average_score           numeric(6, 2),
    lowest_score            numeric(6, 2),
    lowest_rank             integer,
    province_control_score  numeric(6, 2),
    line_deviation          numeric(6, 2),
    source_system           text NOT NULL REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128) NOT NULL,
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_school_admission_query
    ON gaokao.school_admission_score_fact (school_id, province_id, admission_year);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_school_admission_business
    ON gaokao.school_admission_score_fact (
        school_id,
        province_id,
        admission_year,
        COALESCE(raw_batch_name, ''),
        COALESCE(raw_section_name, ''),
        COALESCE(raw_admission_type, ''),
        COALESCE(raw_major_group_name, ''),
        source_system,
        source_table
    );

CREATE TABLE IF NOT EXISTS gaokao.major_admission_score_fact (
    major_admission_score_id bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    major_id                bigint REFERENCES gaokao.major (major_id),
    school_major_id         bigint REFERENCES gaokao.school_major_catalog (school_major_id),
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    policy_id               bigint NOT NULL REFERENCES gaokao.admission_policy (policy_id),
    school_major_group_id   bigint REFERENCES gaokao.school_major_group (school_major_group_id),
    admission_year          smallint NOT NULL,
    raw_batch_name          varchar(128),
    raw_section_name        varchar(128),
    raw_admission_type      varchar(128),
    raw_major_group_name    varchar(255),
    raw_elective_req        varchar(255),
    school_major_name       varchar(255),
    major_code              varchar(32),
    highest_score           numeric(6, 2),
    average_score           numeric(6, 2),
    lowest_score            numeric(6, 2),
    lowest_rank             integer,
    line_deviation          numeric(6, 2),
    source_system           text NOT NULL REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128) NOT NULL,
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_major_admission_query
    ON gaokao.major_admission_score_fact (school_id, province_id, admission_year);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_major_admission_business
    ON gaokao.major_admission_score_fact (
        school_id,
        province_id,
        admission_year,
        COALESCE(raw_batch_name, ''),
        COALESCE(raw_section_name, ''),
        COALESCE(raw_admission_type, ''),
        COALESCE(raw_major_group_name, ''),
        COALESCE(school_major_id, 0),
        source_system,
        source_table
    );

CREATE TABLE IF NOT EXISTS gaokao.enrollment_plan_fact (
    enrollment_plan_id      bigserial PRIMARY KEY,
    school_id               bigint NOT NULL REFERENCES gaokao.school (school_id) ON DELETE CASCADE,
    major_id                bigint REFERENCES gaokao.major (major_id),
    school_major_id         bigint REFERENCES gaokao.school_major_catalog (school_major_id),
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    policy_id               bigint NOT NULL REFERENCES gaokao.admission_policy (policy_id),
    school_major_group_id   bigint REFERENCES gaokao.school_major_group (school_major_group_id),
    plan_year               smallint NOT NULL,
    raw_batch_name          varchar(128),
    raw_section_name        varchar(128),
    raw_admission_type      varchar(128),
    raw_major_group_name    varchar(255),
    raw_elective_req        varchar(255),
    school_major_name       varchar(255),
    major_code              varchar(32),
    plan_count              integer,
    tuition_fee             numeric(12, 2),
    study_years_text        varchar(32),
    school_code             varchar(32),
    major_plan_code         varchar(32),
    source_system           text NOT NULL REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128) NOT NULL,
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_enrollment_plan_query
    ON gaokao.enrollment_plan_fact (school_id, province_id, plan_year);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_enrollment_plan_business
    ON gaokao.enrollment_plan_fact (
        school_id,
        province_id,
        plan_year,
        COALESCE(raw_batch_name, ''),
        COALESCE(raw_section_name, ''),
        COALESCE(raw_admission_type, ''),
        COALESCE(raw_major_group_name, ''),
        COALESCE(school_major_id, 0),
        source_system,
        source_table
    );

CREATE TABLE IF NOT EXISTS gaokao.province_batch_line_fact (
    province_batch_line_id  bigserial PRIMARY KEY,
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    policy_id               bigint NOT NULL REFERENCES gaokao.admission_policy (policy_id),
    score_year              smallint NOT NULL,
    raw_batch_name          varchar(128),
    raw_category_name       varchar(128),
    raw_section_name        varchar(128),
    score_value             numeric(6, 2) NOT NULL,
    rank_value              integer,
    source_system           text NOT NULL REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128) NOT NULL,
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gaokao_province_batch_line_query
    ON gaokao.province_batch_line_fact (province_id, score_year);

CREATE UNIQUE INDEX IF NOT EXISTS uq_gaokao_province_batch_line_business
    ON gaokao.province_batch_line_fact (
        province_id,
        score_year,
        COALESCE(raw_batch_name, ''),
        COALESCE(raw_category_name, ''),
        COALESCE(raw_section_name, ''),
        source_system,
        source_table
    );

CREATE TABLE IF NOT EXISTS gaokao.province_score_range_fact (
    province_score_range_id  bigserial PRIMARY KEY,
    province_id             integer NOT NULL REFERENCES gaokao.province (province_id),
    score_year              smallint NOT NULL,
    highest_score           numeric(6, 2),
    lowest_score            numeric(6, 2),
    exam_model              varchar(32),
    source_system           text NOT NULL REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128) NOT NULL,
    source_pk               varchar(64),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (province_id, score_year, source_system, source_table)
);

CREATE TABLE IF NOT EXISTS gaokao.import_file_log (
    import_file_id          bigserial PRIMARY KEY,
    source_system           text NOT NULL REFERENCES gaokao.data_source (source_code),
    source_table            varchar(128) NOT NULL,
    file_name               varchar(255) NOT NULL,
    row_count               bigint,
    imported_at             timestamptz NOT NULL DEFAULT now(),
    remark                  text
);

CREATE OR REPLACE VIEW gaokao.v_school_current_tags AS
SELECT
    s.school_id,
    s.school_name,
    t.tag_type,
    t.tag_value,
    t.effective_year,
    t.expire_year
FROM gaokao.school s
JOIN gaokao.school_policy_tag t ON t.school_id = s.school_id
WHERE t.expire_year IS NULL;

CREATE OR REPLACE VIEW gaokao.v_major_current_tags AS
SELECT
    m.major_id,
    m.major_name,
    t.tag_type,
    t.tag_value,
    t.effective_year,
    t.expire_year
FROM gaokao.major m
JOIN gaokao.major_policy_tag t ON t.major_id = m.major_id
WHERE t.expire_year IS NULL;
