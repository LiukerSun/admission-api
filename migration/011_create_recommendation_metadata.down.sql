DROP TABLE IF EXISTS recommendation_major_ability_rules;
DROP TABLE IF EXISTS recommendation_holland_keywords;
DROP TABLE IF EXISTS recommendation_family_resource_keywords;
DROP TABLE IF EXISTS city_group_members;
DROP TABLE IF EXISTS city_groups;

DROP INDEX IF EXISTS idx_university_profiles_tier;

ALTER TABLE university_profiles
    DROP COLUMN IF EXISTS university_tier;
