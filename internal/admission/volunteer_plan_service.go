package admission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type VolunteerPlanService struct {
	pool *pgxpool.Pool
}

func NewVolunteerPlanService(pool *pgxpool.Pool) *VolunteerPlanService {
	return &VolunteerPlanService{
		pool: pool,
	}
}

func (s *VolunteerPlanService) getUserDetails(ctx context.Context, userID int64) (UserDetails, error) {
	var userDetails UserDetails
	var username sql.NullString
	var email string

	err := s.pool.QueryRow(ctx, `
		SELECT username, email
		FROM users
		WHERE id = $1
	`, userID).Scan(&username, &email)

	if err != nil {
		slog.Error("query user details failed", "userID", userID, "error", err)
		return UserDetails{}, fmt.Errorf("query user details: %w", err)
	}

	userDetails.Username = username.String
	userDetails.Email = email

	return userDetails, nil
}

// GetRichPlan fetches a detailed volunteer plan with all associated rich data
func (s *VolunteerPlanService) GetRichPlan(ctx context.Context, userID, planID int64) (*RichVolunteerPlan, error) {
	var richPlan RichVolunteerPlan

	// 1. Fetch base volunteer plan
	var plan VolunteerPlan
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, COALESCE(description, ''), created_at
		FROM user_volunteer_plans
		WHERE id = $1 AND user_id = $2
	`, planID, userID).Scan(&plan.ID, &plan.UserID, &plan.Name, &plan.Description, &plan.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Error("volunteer plan not found or unauthorized", "planID", planID, "userID", userID, "error", err)
			return nil, fmt.Errorf("volunteer plan not found or unauthorized")
		}
		slog.Error("query base volunteer plan failed", "planID", planID, "userID", userID, "error", err)
		return nil, fmt.Errorf("query base volunteer plan: %w", err)
	}

	richPlan.ID = plan.ID
	richPlan.UserID = plan.UserID
	richPlan.Name = plan.Name
	richPlan.Description = plan.Description
	richPlan.CreatedAt = plan.CreatedAt

	// 2. Fetch User Details
	userDetails, err := s.getUserDetails(ctx, plan.UserID)
	if err != nil {
		slog.Error("get user details for rich plan failed", "planID", planID, "userID", plan.UserID, "error", err)
		return nil, fmt.Errorf("get user details for rich plan: %w", err)
	}
	richPlan.UserDetails = userDetails

	// 3. Fetch Detailed Groups and Majors
	groupRows, err := s.pool.Query(ctx, `
		SELECT
			ug.id, ug.plan_id, ug.order_no, ug.university_id, ug.university_code, ug.university_name,
			ug.group_id, ug.group_code, COALESCE(ug.group_name, ''), ug.is_obey_adjustment, COALESCE(ug.remark, ''),
			COALESCE(up.is_985, FALSE), COALESCE(up.is_211, FALSE), COALESCE(sc.name, '') as school_category_name, COALESCE(r.name, '') as region_name,
			COALESCE(ae.equivalent_min_score_2024, 0), COALESCE(ae.equivalent_min_score_2023, 0), COALESCE(ae.equivalent_min_score_2022, 0)
		FROM user_volunteer_groups ug
		LEFT JOIN university_profiles up ON ug.university_id = up.university_id AND up.profile_year = EXTRACT(YEAR FROM NOW())
		LEFT JOIN universities u ON ug.university_id = u.id
		LEFT JOIN school_categories sc ON up.school_category_code = sc.code
		LEFT JOIN regions r ON up.region_code = r.code
		LEFT JOIN admission_group_extensions ae ON ug.group_id = ae.admission_group_id
		WHERE ug.plan_id = $1
		ORDER BY ug.order_no ASC
	`, planID)
	if err != nil {
		slog.Error("query detailed groups failed", "planID", planID, "error", err)
		return nil, fmt.Errorf("query detailed groups: %w", err)
	}
	defer groupRows.Close()

	var detailedGroups []DetailedVolunteerPlanGroup
	uniqueSchools := make(map[string]bool)
	var totalMajors int
	majorDistribution := make(map[string]int)

	for groupRows.Next() {
		var dg DetailedVolunteerPlanGroup
		var universityID sql.NullInt64
		var groupID sql.NullInt64
		var is985, is211 bool                            // These are now directly boolean due to COALESCE
		var schoolCategoryName, regionName string        // These are now directly string due to COALESCE
		var minScore2024, minScore2023, minScore2022 int // These are now directly int due to COALESCE

		err := groupRows.Scan(
			&dg.ID, &dg.PlanID, &dg.OrderNo, &universityID, &dg.UniversityCode, &dg.UniversityName,
			&groupID, &dg.GroupCode, &dg.GroupName, &dg.IsObeyAdjustment, &dg.Remark,
			&is985, &is211, &schoolCategoryName, &regionName,
			&minScore2024, &minScore2023, &minScore2022,
		)
		if err != nil {
			slog.Error("scan detailed group failed", "planID", planID, "error", err)
			return nil, fmt.Errorf("scan detailed group: %w", err)
		}

		if universityID.Valid {
			dg.UniversityID = &universityID.Int64
		} else {
			dg.UniversityID = nil // Explicitly set to nil if NULL
		}

		if groupID.Valid {
			dg.GroupID = &groupID.Int64
		} else {
			dg.GroupID = nil // Explicitly set to nil if NULL
		}

		dg.UniversityDetails = UniversityDetails{
			Is985:          is985,
			Is211:          is211,
			SchoolCategory: schoolCategoryName,
			Region:         regionName,
		}

		dg.GroupAdmissionDetails = GroupAdmissionDetails{
			MinScore2024: minScore2024,
			MinScore2023: minScore2023,
			MinScore2022: minScore2022,
		}

		uniqueSchools[dg.UniversityCode] = true

		// Query Detailed Majors
		majorRows, err := s.pool.Query(ctx, `
			SELECT
				um.major_order, um.major_code, um.major_name,
				COALESCE(umad.min_score, 0), COALESCE(umad.min_rank, 0), COALESCE(umad.tuition, 0),
				COALESCE(umad.major_intro, ''), COALESCE(umad.training_goal, ''), COALESCE(umad.employment_direction, '')
			FROM user_volunteer_majors um
			LEFT JOIN university_major_admissions umad ON um.major_admission_id = umad.id
			WHERE um.group_id = $1
			ORDER BY um.major_order ASC
		`, dg.ID)
		if err != nil {
			slog.Error("query detailed majors failed", "planID", planID, "groupID", dg.ID, "error", err)
			return nil, fmt.Errorf("query detailed majors: %w", err)
		}
		var detailedMajors []DetailedVolunteerPlanMajor
		for majorRows.Next() {
			var dm DetailedVolunteerPlanMajor
			var minScore, minRank, tuition int                       // Directly int due to COALESCE
			var majorIntro, trainingGoal, employmentDirection string // Directly string due to COALESCE

			err := majorRows.Scan(
				&dm.MajorOrder, &dm.MajorCode, &dm.MajorName,
				&minScore, &minRank, &tuition,
				&majorIntro, &trainingGoal, &employmentDirection,
			)
			if err != nil {
				majorRows.Close()
				slog.Error("scan detailed major failed", "planID", planID, "groupID", dg.ID, "error", err)
				return nil, fmt.Errorf("scan detailed major: %w", err)
			}

			dm.MinScore = minScore
			dm.MinRank = minRank
			dm.Tuition = tuition
			dm.MajorIntro = majorIntro
			dm.TrainingGoal = trainingGoal
			dm.EmploymentDirection = employmentDirection

			detailedMajors = append(detailedMajors, dm)
			if dm.MajorName != "" {
				totalMajors++
				majorDistribution[dm.MajorName]++
			}
		}
		majorRows.Close()
		dg.DetailedMajors = detailedMajors
		detailedGroups = append(detailedGroups, dg)
	}
	richPlan.DetailedGroups = detailedGroups

	// 4. Populate Plan Statistics
	richPlan.PlanStatistics = PlanStatistics{
		TotalUniversities: len(uniqueSchools),
		TotalGroups:       len(detailedGroups),
		TotalMajors:       totalMajors,
		MajorDistribution: majorDistribution,
	}

	return &richPlan, nil
}

func (s *VolunteerPlanService) GetPlans(ctx context.Context, userID int64) (*VolunteerPlansResponse, error) {
	// 1. 获取该用户的所有方案
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, COALESCE(description, ''), created_at
		FROM user_volunteer_plans
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query plans: %w", err)
	}
	defer rows.Close()

	var plans []VolunteerPlan
	for rows.Next() {
		var p VolunteerPlan
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		plans = append(plans, p)
	}

	// 2. 为每个方案填充数据
	for i := range plans {
		groupRows, err := s.pool.Query(ctx, `
			SELECT id, order_no, university_id, university_code, university_name, group_id, group_code, COALESCE(group_name, ''), is_obey_adjustment, COALESCE(remark, '')
			FROM user_volunteer_groups
			WHERE plan_id = $1
			ORDER BY order_no ASC
		`, plans[i].ID)
		if err != nil {
			return nil, fmt.Errorf("query groups: %w", err)
		}

		var groups []VolunteerPlanGroup
		uniqueSchools := make(map[string]bool)

		for groupRows.Next() {
			var g VolunteerPlanGroup
			if err := groupRows.Scan(&g.ID, &g.OrderNo, &g.UniversityID, &g.UniversityCode, &g.UniversityName, &g.GroupID, &g.GroupCode, &g.GroupName, &g.IsObeyAdjustment, &g.Remark); err != nil {
				groupRows.Close()
				return nil, fmt.Errorf("scan group: %w", err)
			}
			uniqueSchools[g.UniversityCode] = true

			// 查询专业
			majorRows, err := s.pool.Query(ctx, `
				SELECT id, major_order, major_admission_id, COALESCE(major_code, ''), COALESCE(major_name, '')
				FROM user_volunteer_majors
				WHERE group_id = $1
				ORDER BY major_order ASC
			`, g.ID)
			if err != nil {
				groupRows.Close()
				return nil, fmt.Errorf("query majors: %w", err)
			}

			var majors []VolunteerPlanMajor
			for majorRows.Next() {
				var m VolunteerPlanMajor
				if err := majorRows.Scan(&m.ID, &m.MajorOrder, &m.MajorAdmissionID, &m.MajorCode, &m.MajorName); err != nil {
					majorRows.Close()
					groupRows.Close()
					return nil, fmt.Errorf("scan major: %w", err)
				}
				majors = append(majors, m)
			}
			majorRows.Close()
			g.Majors = majors
			groups = append(groups, g)
		}
		groupRows.Close()

		plans[i].Groups = groups
		plans[i].Stats = VolunteerPlanStats{
			SchoolCount: len(uniqueSchools),
			GroupCount:  len(groups),
			RecordCount: len(groups), // 这里以专业组作为记录数
		}
	}

	return &VolunteerPlansResponse{Plans: plans}, nil
}

func (s *VolunteerPlanService) UpdatePlan(ctx context.Context, userID, planID int64, name string, description string) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE user_volunteer_plans
		SET name = $1, description = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4
	`, name, description, planID, userID)
	if err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("plan not found or unauthorized")
	}
	return nil
}

func (s *VolunteerPlanService) UpdateGroupRemark(ctx context.Context, userID, groupID int64, remark string) error {
	// 验证该 group 是否属于该用户的方案
	result, err := s.pool.Exec(ctx, `
		UPDATE user_volunteer_groups
		SET remark = $1, updated_at = NOW()
		WHERE id = $2 AND plan_id IN (SELECT id FROM user_volunteer_plans WHERE user_id = $3)
	`, remark, groupID, userID)
	if err != nil {
		return fmt.Errorf("update group remark: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("group not found or unauthorized")
	}
	return nil
}
