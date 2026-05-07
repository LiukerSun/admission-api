CREATE TABLE IF NOT EXISTS major_categories (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, category_code)
);

CREATE TABLE IF NOT EXISTS major_classes (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    class_code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, class_code),
    FOREIGN KEY (catalog_year, category_code)
        REFERENCES major_categories (catalog_year, category_code)
);

CREATE TABLE IF NOT EXISTS standard_majors (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    major_code VARCHAR(20) NOT NULL,
    name VARCHAR(200) NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    class_code VARCHAR(20) NOT NULL,
    duration VARCHAR(50),
    degree_category VARCHAR(100),
    source_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, major_code),
    FOREIGN KEY (catalog_year, category_code)
        REFERENCES major_categories (catalog_year, category_code),
    FOREIGN KEY (catalog_year, class_code)
        REFERENCES major_classes (catalog_year, class_code)
);

CREATE INDEX IF NOT EXISTS idx_standard_majors_catalog_name
    ON standard_majors(catalog_year, name);

CREATE INDEX IF NOT EXISTS idx_standard_majors_catalog_class
    ON standard_majors(catalog_year, class_code);
