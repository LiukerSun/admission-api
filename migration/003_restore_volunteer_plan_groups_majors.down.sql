DROP INDEX IF EXISTS idx_user_volunteer_majors_group_id;
DROP INDEX IF EXISTS idx_user_volunteer_groups_plan_id;
DROP TABLE IF EXISTS user_volunteer_majors;
DROP TABLE IF EXISTS user_volunteer_groups;

ALTER TABLE user_volunteer_plans
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS name;
