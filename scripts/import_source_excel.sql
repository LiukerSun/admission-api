BEGIN;

CREATE TEMP TABLE tmp_regions (
    code text,
    name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_subject_categories (
    code text,
    name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_subject_requirements (
    code text,
    name text,
    normalized_subjects jsonb
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_batches (
    code text,
    name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_education_levels (
    code text,
    name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_school_ownership_types (
    code text,
    name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_school_categories (
    code text,
    name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_universities (
    university_code text,
    name text,
    normalized_name text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_university_profiles (
    university_code text,
    university_name text,
    profile_year integer,
    region_code text,
    city text,
    ownership_type_code text,
    school_category_code text,
    education_level_code text,
    is_985 boolean,
    is_211 boolean,
    is_double_first_class boolean,
    is_national_key boolean,
    is_provincial_key boolean,
    has_postgraduate_recommendation boolean,
    postgraduate_recommendation_rate numeric,
    soft_rank text,
    alumni_rank text,
    difficulty_rank text,
    doctoral_program_count integer,
    master_program_count integer,
    national_key_subject_count integer,
    affiliation text,
    school_level_tags text,
    excellence_tags text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_admission_groups (
    university_code text,
    university_name text,
    admission_year integer,
    region_code text,
    subject_category_code text,
    batch_code text,
    group_code text,
    subject_requirement_code text,
    education_level_code text,
    group_major_count integer,
    group_major_names text,
    group_type text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_admission_group_extensions (
    university_code text,
    university_name text,
    admission_year integer,
    region_code text,
    subject_category_code text,
    batch_code text,
    group_code text,
    batch_remark text,
    group_min_score integer,
    group_min_rank integer,
    equivalent_min_score_2024 integer,
    equivalent_min_score_2023 integer,
    equivalent_min_score_2022 integer,
    subject_change_2024 text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_university_major_admissions (
    university_code text,
    university_name text,
    admission_year integer,
    region_code text,
    subject_category_code text,
    batch_code text,
    group_code text,
    local_major_code text,
    local_major_name text,
    admitted_count integer,
    min_score integer,
    min_rank integer,
    max_score integer,
    max_rank integer,
    equivalent_min_score integer,
    tuition integer,
    duration text,
    admission_remark text,
    major_intro text,
    training_goal text,
    subject_study_requirement text,
    main_courses text,
    postgraduate_direction text,
    employment_direction text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_university_major_profiles (
    university_code text,
    university_name text,
    admission_year integer,
    region_code text,
    subject_category_code text,
    batch_code text,
    group_code text,
    local_major_code text,
    discipline_category text,
    first_level_discipline text,
    fourth_round_subject_eval text,
    double_first_class_subject text,
    soft_major_grade text,
    major_evaluation_score numeric,
    major_rank text,
    is_national_feature boolean,
    corresponding_master_majors text,
    corresponding_doctoral_majors text
) ON COMMIT DROP;

CREATE TEMP TABLE tmp_university_postgraduate_profiles (
    university_code text,
    university_name text,
    profile_year integer,
    master_major_count integer,
    master_major_names text,
    doctoral_major_count integer,
    doctoral_major_names text
) ON COMMIT DROP;

\copy tmp_regions FROM '/tmp/source_import/regions.csv' CSV HEADER
\copy tmp_subject_categories FROM '/tmp/source_import/subject_categories.csv' CSV HEADER
\copy tmp_subject_requirements FROM '/tmp/source_import/subject_requirements.csv' CSV HEADER
\copy tmp_batches FROM '/tmp/source_import/batches.csv' CSV HEADER
\copy tmp_education_levels FROM '/tmp/source_import/education_levels.csv' CSV HEADER
\copy tmp_school_ownership_types FROM '/tmp/source_import/school_ownership_types.csv' CSV HEADER
\copy tmp_school_categories FROM '/tmp/source_import/school_categories.csv' CSV HEADER
\copy tmp_universities FROM '/tmp/source_import/universities.csv' CSV HEADER
\copy tmp_university_profiles FROM '/tmp/source_import/university_profiles.csv' CSV HEADER
\copy tmp_admission_groups FROM '/tmp/source_import/admission_groups.csv' CSV HEADER
\copy tmp_admission_group_extensions FROM '/tmp/source_import/admission_group_extensions.csv' CSV HEADER
\copy tmp_university_major_admissions FROM '/tmp/source_import/university_major_admissions.csv' CSV HEADER
\copy tmp_university_major_profiles FROM '/tmp/source_import/university_major_profiles.csv' CSV HEADER
\copy tmp_university_postgraduate_profiles FROM '/tmp/source_import/university_postgraduate_profiles.csv' CSV HEADER

INSERT INTO regions (code, name)
SELECT code, name FROM tmp_regions
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO subject_categories (code, name)
SELECT code, name FROM tmp_subject_categories
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO subject_requirements (code, name, normalized_subjects)
SELECT code, name, normalized_subjects FROM tmp_subject_requirements
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    normalized_subjects = EXCLUDED.normalized_subjects,
    updated_at = NOW();

INSERT INTO batches (code, name)
SELECT code, name FROM tmp_batches
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO education_levels (code, name)
SELECT code, name FROM tmp_education_levels
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO school_ownership_types (code, name)
SELECT code, name FROM tmp_school_ownership_types
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO school_categories (code, name)
SELECT code, name FROM tmp_school_categories
WHERE code <> '' AND name <> ''
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO universities (university_code, name, normalized_name)
SELECT university_code, name, normalized_name
FROM tmp_universities
WHERE university_code <> '' AND name <> ''
ON CONFLICT (university_code, name) DO UPDATE
SET normalized_name = EXCLUDED.normalized_name,
    updated_at = NOW();

INSERT INTO university_profiles (
    university_id,
    profile_year,
    region_code,
    city,
    ownership_type_code,
    school_category_code,
    education_level_code,
    is_985,
    is_211,
    is_double_first_class,
    is_national_key,
    is_provincial_key,
    has_postgraduate_recommendation,
    postgraduate_recommendation_rate,
    soft_rank,
    alumni_rank,
    difficulty_rank,
    doctoral_program_count,
    master_program_count,
    national_key_subject_count,
    affiliation,
    school_level_tags,
    excellence_tags
)
SELECT
    u.id,
    p.profile_year,
    NULLIF(p.region_code, ''),
    NULLIF(p.city, ''),
    NULLIF(p.ownership_type_code, ''),
    NULLIF(p.school_category_code, ''),
    NULLIF(p.education_level_code, ''),
    p.is_985,
    p.is_211,
    p.is_double_first_class,
    p.is_national_key,
    p.is_provincial_key,
    p.has_postgraduate_recommendation,
    p.postgraduate_recommendation_rate,
    NULLIF(p.soft_rank, ''),
    NULLIF(p.alumni_rank, ''),
    NULLIF(p.difficulty_rank, ''),
    p.doctoral_program_count,
    p.master_program_count,
    p.national_key_subject_count,
    NULLIF(p.affiliation, ''),
    NULLIF(p.school_level_tags, ''),
    NULLIF(p.excellence_tags, '')
FROM tmp_university_profiles p
JOIN universities u
  ON u.university_code = p.university_code
 AND u.name = p.university_name
ON CONFLICT (university_id, profile_year) DO UPDATE
SET region_code = EXCLUDED.region_code,
    city = EXCLUDED.city,
    ownership_type_code = EXCLUDED.ownership_type_code,
    school_category_code = EXCLUDED.school_category_code,
    education_level_code = EXCLUDED.education_level_code,
    is_985 = EXCLUDED.is_985,
    is_211 = EXCLUDED.is_211,
    is_double_first_class = EXCLUDED.is_double_first_class,
    is_national_key = EXCLUDED.is_national_key,
    is_provincial_key = EXCLUDED.is_provincial_key,
    has_postgraduate_recommendation = EXCLUDED.has_postgraduate_recommendation,
    postgraduate_recommendation_rate = EXCLUDED.postgraduate_recommendation_rate,
    soft_rank = EXCLUDED.soft_rank,
    alumni_rank = EXCLUDED.alumni_rank,
    difficulty_rank = EXCLUDED.difficulty_rank,
    doctoral_program_count = EXCLUDED.doctoral_program_count,
    master_program_count = EXCLUDED.master_program_count,
    national_key_subject_count = EXCLUDED.national_key_subject_count,
    affiliation = EXCLUDED.affiliation,
    school_level_tags = EXCLUDED.school_level_tags,
    excellence_tags = EXCLUDED.excellence_tags,
    updated_at = NOW();

INSERT INTO admission_groups (
    university_id,
    admission_year,
    region_code,
    subject_category_code,
    batch_code,
    group_code,
    subject_requirement_code,
    education_level_code,
    group_major_count,
    group_major_names,
    group_type
)
SELECT
    u.id,
    g.admission_year,
    g.region_code,
    g.subject_category_code,
    g.batch_code,
    g.group_code,
    NULLIF(g.subject_requirement_code, ''),
    NULLIF(g.education_level_code, ''),
    g.group_major_count,
    NULLIF(g.group_major_names, ''),
    NULLIF(g.group_type, '')
FROM tmp_admission_groups g
JOIN universities u
  ON u.university_code = g.university_code
 AND u.name = g.university_name
WHERE g.group_code <> ''
ON CONFLICT (
    university_id,
    admission_year,
    region_code,
    subject_category_code,
    batch_code,
    group_code
) DO UPDATE
SET subject_requirement_code = EXCLUDED.subject_requirement_code,
    education_level_code = EXCLUDED.education_level_code,
    group_major_count = EXCLUDED.group_major_count,
    group_major_names = EXCLUDED.group_major_names,
    group_type = EXCLUDED.group_type,
    updated_at = NOW();

INSERT INTO admission_group_extensions (
    admission_group_id,
    batch_remark,
    group_min_score,
    group_min_rank,
    equivalent_min_score_2024,
    equivalent_min_score_2023,
    equivalent_min_score_2022,
    subject_change_2024
)
SELECT
    ag.id,
    NULLIF(e.batch_remark, ''),
    e.group_min_score,
    e.group_min_rank,
    e.equivalent_min_score_2024,
    e.equivalent_min_score_2023,
    e.equivalent_min_score_2022,
    NULLIF(e.subject_change_2024, '')
FROM tmp_admission_group_extensions e
JOIN universities u
  ON u.university_code = e.university_code
 AND u.name = e.university_name
JOIN admission_groups ag
  ON ag.university_id = u.id
 AND ag.admission_year = e.admission_year
 AND ag.region_code = e.region_code
 AND ag.subject_category_code = e.subject_category_code
 AND ag.batch_code = e.batch_code
 AND ag.group_code = e.group_code
WHERE e.group_code <> ''
ON CONFLICT (admission_group_id) DO UPDATE
SET batch_remark = EXCLUDED.batch_remark,
    group_min_score = EXCLUDED.group_min_score,
    group_min_rank = EXCLUDED.group_min_rank,
    equivalent_min_score_2024 = EXCLUDED.equivalent_min_score_2024,
    equivalent_min_score_2023 = EXCLUDED.equivalent_min_score_2023,
    equivalent_min_score_2022 = EXCLUDED.equivalent_min_score_2022,
    subject_change_2024 = EXCLUDED.subject_change_2024,
    updated_at = NOW();

INSERT INTO university_major_admissions (
    admission_group_id,
    local_major_code,
    local_major_name,
    admitted_count,
    min_score,
    min_rank,
    max_score,
    max_rank,
    equivalent_min_score,
    tuition,
    duration,
    admission_remark,
    major_intro,
    training_goal,
    subject_study_requirement,
    main_courses,
    postgraduate_direction,
    employment_direction
)
SELECT
    ag.id,
    a.local_major_code,
    a.local_major_name,
    a.admitted_count,
    a.min_score,
    a.min_rank,
    a.max_score,
    a.max_rank,
    a.equivalent_min_score,
    a.tuition,
    NULLIF(a.duration, ''),
    NULLIF(a.admission_remark, ''),
    NULLIF(a.major_intro, ''),
    NULLIF(a.training_goal, ''),
    NULLIF(a.subject_study_requirement, ''),
    NULLIF(a.main_courses, ''),
    NULLIF(a.postgraduate_direction, ''),
    NULLIF(a.employment_direction, '')
FROM tmp_university_major_admissions a
JOIN universities u
  ON u.university_code = a.university_code
 AND u.name = a.university_name
JOIN admission_groups ag
  ON ag.university_id = u.id
 AND ag.admission_year = a.admission_year
 AND ag.region_code = a.region_code
 AND ag.subject_category_code = a.subject_category_code
 AND ag.batch_code = a.batch_code
 AND ag.group_code = a.group_code
WHERE a.local_major_code <> ''
  AND a.local_major_name <> ''
ON CONFLICT (admission_group_id, local_major_code) DO UPDATE
SET local_major_name = EXCLUDED.local_major_name,
    admitted_count = EXCLUDED.admitted_count,
    min_score = EXCLUDED.min_score,
    min_rank = EXCLUDED.min_rank,
    max_score = EXCLUDED.max_score,
    max_rank = EXCLUDED.max_rank,
    equivalent_min_score = EXCLUDED.equivalent_min_score,
    tuition = EXCLUDED.tuition,
    duration = EXCLUDED.duration,
    admission_remark = EXCLUDED.admission_remark,
    major_intro = EXCLUDED.major_intro,
    training_goal = EXCLUDED.training_goal,
    subject_study_requirement = EXCLUDED.subject_study_requirement,
    main_courses = EXCLUDED.main_courses,
    postgraduate_direction = EXCLUDED.postgraduate_direction,
    employment_direction = EXCLUDED.employment_direction,
    updated_at = NOW();

INSERT INTO university_major_profiles (
    university_major_admission_id,
    discipline_category,
    first_level_discipline,
    fourth_round_subject_eval,
    double_first_class_subject,
    soft_major_grade,
    major_evaluation_score,
    major_rank,
    is_national_feature,
    corresponding_master_majors,
    corresponding_doctoral_majors
)
SELECT
    uma.id,
    NULLIF(p.discipline_category, ''),
    NULLIF(p.first_level_discipline, ''),
    NULLIF(p.fourth_round_subject_eval, ''),
    NULLIF(p.double_first_class_subject, ''),
    NULLIF(p.soft_major_grade, ''),
    p.major_evaluation_score,
    NULLIF(p.major_rank, ''),
    p.is_national_feature,
    NULLIF(p.corresponding_master_majors, ''),
    NULLIF(p.corresponding_doctoral_majors, '')
FROM tmp_university_major_profiles p
JOIN universities u
  ON u.university_code = p.university_code
 AND u.name = p.university_name
JOIN admission_groups ag
  ON ag.university_id = u.id
 AND ag.admission_year = p.admission_year
 AND ag.region_code = p.region_code
 AND ag.subject_category_code = p.subject_category_code
 AND ag.batch_code = p.batch_code
 AND ag.group_code = p.group_code
JOIN university_major_admissions uma
  ON uma.admission_group_id = ag.id
 AND uma.local_major_code = p.local_major_code
WHERE p.local_major_code <> ''
ON CONFLICT (university_major_admission_id) DO UPDATE
SET discipline_category = EXCLUDED.discipline_category,
    first_level_discipline = EXCLUDED.first_level_discipline,
    fourth_round_subject_eval = EXCLUDED.fourth_round_subject_eval,
    double_first_class_subject = EXCLUDED.double_first_class_subject,
    soft_major_grade = EXCLUDED.soft_major_grade,
    major_evaluation_score = EXCLUDED.major_evaluation_score,
    major_rank = EXCLUDED.major_rank,
    is_national_feature = EXCLUDED.is_national_feature,
    corresponding_master_majors = EXCLUDED.corresponding_master_majors,
    corresponding_doctoral_majors = EXCLUDED.corresponding_doctoral_majors,
    updated_at = NOW();

INSERT INTO university_postgraduate_profiles (
    university_id,
    profile_year,
    master_major_count,
    master_major_names,
    doctoral_major_count,
    doctoral_major_names
)
SELECT
    u.id,
    p.profile_year,
    p.master_major_count,
    NULLIF(p.master_major_names, ''),
    p.doctoral_major_count,
    NULLIF(p.doctoral_major_names, '')
FROM tmp_university_postgraduate_profiles p
JOIN universities u
  ON u.university_code = p.university_code
 AND u.name = p.university_name
ON CONFLICT (university_id, profile_year) DO UPDATE
SET master_major_count = EXCLUDED.master_major_count,
    master_major_names = EXCLUDED.master_major_names,
    doctoral_major_count = EXCLUDED.doctoral_major_count,
    doctoral_major_names = EXCLUDED.doctoral_major_names,
    updated_at = NOW();

COMMIT;
