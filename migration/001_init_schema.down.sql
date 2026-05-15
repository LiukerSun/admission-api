-- Drop tables in reverse dependency order.

-- 7. Recommendation algorithm metadata
DROP TABLE IF EXISTS recommendation_strategy_keywords;
DROP TABLE IF EXISTS recommendation_precomputed_scores;
DROP TABLE IF EXISTS recommendation_major_ability_rules;
DROP TABLE IF EXISTS recommendation_holland_keywords;
DROP TABLE IF EXISTS recommendation_family_resource_keywords;
DROP TABLE IF EXISTS city_group_members;
DROP TABLE IF EXISTS city_groups;

-- 6. Conversations, plan drafts, saved volunteer plans
DROP TABLE IF EXISTS user_volunteer_plans;
DROP TABLE IF EXISTS conversation_plan_drafts;
DROP TABLE IF EXISTS conversation_filters;
DROP TABLE IF EXISTS conversation_messages;
DROP TABLE IF EXISTS conversations;

-- 5. Admission groups and major-level admission rows
DROP TABLE IF EXISTS admission_major_tags;
DROP TABLE IF EXISTS university_postgraduate_profiles;
DROP TABLE IF EXISTS university_major_profiles;
DROP TABLE IF EXISTS university_major_admissions;
DROP TABLE IF EXISTS admission_group_extensions;
DROP TABLE IF EXISTS admission_groups;

-- 4. Universities + profiles
DROP TABLE IF EXISTS university_profiles;
DROP TABLE IF EXISTS universities;

-- 3. National major catalog
DROP TABLE IF EXISTS standard_majors;
DROP TABLE IF EXISTS major_classes;
DROP TABLE IF EXISTS major_categories;

-- 2. Dictionaries
DROP TABLE IF EXISTS school_categories;
DROP TABLE IF EXISTS school_ownership_types;
DROP TABLE IF EXISTS education_levels;
DROP TABLE IF EXISTS batches;
DROP TABLE IF EXISTS subject_requirements;
DROP TABLE IF EXISTS subject_categories;
DROP TABLE IF EXISTS regions;

-- 1. Accounts / membership / payments / refunds
DROP TABLE IF EXISTS payment_refunds;
DROP TABLE IF EXISTS membership_grants;
DROP TABLE IF EXISTS user_memberships;
DROP TABLE IF EXISTS payment_callbacks;
DROP TABLE IF EXISTS payment_attempts;
DROP TABLE IF EXISTS payment_orders;
DROP TABLE IF EXISTS membership_plans;
DROP TABLE IF EXISTS users;
