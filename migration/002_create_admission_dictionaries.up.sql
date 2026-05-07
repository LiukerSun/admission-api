CREATE TABLE IF NOT EXISTS regions (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subject_categories (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subject_requirements (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    normalized_subjects JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS batches (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS education_levels (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS school_ownership_types (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS school_categories (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO regions (code, name)
VALUES
    ('230000', '黑龙江省'),
    ('110000', '北京市')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO subject_categories (code, name)
VALUES
    ('physics', '物理'),
    ('history', '历史'),
    ('science', '理科'),
    ('arts', '文科')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO subject_requirements (code, name, normalized_subjects)
VALUES
    ('none', '不限', '[]'::jsonb),
    ('chemistry', '化学', '["化学"]'::jsonb),
    ('physics_chemistry', '物理+化学', '["物理", "化学"]'::jsonb)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    normalized_subjects = EXCLUDED.normalized_subjects,
    updated_at = NOW();

INSERT INTO batches (code, name)
VALUES
    ('regular_undergraduate', '普通本科批'),
    ('early_undergraduate', '本科提前批'),
    ('specialist', '专科批')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO education_levels (code, name)
VALUES
    ('undergraduate', '本科'),
    ('specialist', '专科')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO school_ownership_types (code, name)
VALUES
    ('public', '公办'),
    ('private', '民办'),
    ('sino_foreign', '中外合作办学')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO school_categories (code, name)
VALUES
    ('comprehensive', '综合类'),
    ('science_engineering', '理工类'),
    ('medicine', '医药类'),
    ('normal', '师范类'),
    ('finance_economics', '财经类'),
    ('agriculture_forestry', '农林类'),
    ('language', '语言类'),
    ('politics_law', '政法类'),
    ('art', '艺术类'),
    ('sports', '体育类')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();
