//go:build integration

package admission

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestAdmissionLineStoreFiltersSchoolsGroupsAndDefaultsToLatestYear(t *testing.T) {
	databaseURL := os.Getenv("ADMISSION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ADMISSION_TEST_DATABASE_URL to run admission line store integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	cleanupAdmissionLineIntegrationData(t, pool)
	seedAdmissionLineIntegrationData(t, pool)

	store := NewAdmissionLineStore(pool)
	lines, err := store.ListAdmissionLines(ctx, AdmissionLineFilter{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		UniversityCodes:     []string{"TDD-1003", "TDD-1001"},
		GroupCodes:          []string{"008", "009"},
	})

	require.NoError(t, err)
	require.Len(t, lines, 2)
	require.Equal(t, 2025, lines[0].AdmissionYear)
	require.Equal(t, "TDD-1001", lines[0].UniversityCode)
	require.Equal(t, "009", lines[0].GroupCode)
	require.Equal(t, "31", lines[0].LocalMajorCode)
	require.Equal(t, "理科试验班类", lines[0].LocalMajorName)
	require.Equal(t, 80, *lines[0].MinRank)
	require.Equal(t, 2025, lines[1].AdmissionYear)
	require.Equal(t, "TDD-1003", lines[1].UniversityCode)
	require.Equal(t, "008", lines[1].GroupCode)
	require.Equal(t, "25", lines[1].LocalMajorCode)
	require.Equal(t, "计算机类", lines[1].LocalMajorName)
	require.Equal(t, "physics_chemistry", lines[1].SubjectRequirementCode)
	require.Equal(t, 721, *lines[1].MinScore)
	require.Equal(t, 45, *lines[1].MinRank)
	require.Equal(t, 5000, *lines[1].Tuition)
	require.Equal(t, "四年", lines[1].Duration)
	require.Equal(t, "含人工智能方向", lines[1].AdmissionRemark)

	explicitYear := 2024
	historicalLines, err := store.ListAdmissionLines(ctx, AdmissionLineFilter{
		AdmissionYear:       &explicitYear,
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		UniversityCodes:     []string{"TDD-1003", "TDD-1001"},
		GroupCodes:          []string{"008", "009"},
	})

	require.NoError(t, err)
	require.Len(t, historicalLines, 1)
	require.Equal(t, 2024, historicalLines[0].AdmissionYear)
	require.Equal(t, "TDD-1003", historicalLines[0].UniversityCode)
	require.Equal(t, "008", historicalLines[0].GroupCode)
	require.Equal(t, "25", historicalLines[0].LocalMajorCode)
	require.Equal(t, 700, *historicalLines[0].MinScore)

	taggedLines, err := store.ListAdmissionLines(ctx, AdmissionLineFilter{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TagCatalogYear:      intPtr(2025),
		TagMajorCode:        "080901",
		MinRankFrom:         intPtr(1),
		MinRankTo:           intPtr(60),
		MinScoreFrom:        intPtr(700),
		MinScoreTo:          intPtr(730),
	})

	require.NoError(t, err)
	require.Len(t, taggedLines, 1)
	require.Equal(t, "TDD-1003", taggedLines[0].UniversityCode)
	require.Equal(t, "计算机类", taggedLines[0].LocalMajorName)

	tagKeywordLines, err := store.ListAdmissionLines(ctx, AdmissionLineFilter{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TagCatalogYear:      intPtr(2025),
		TagQuery:            "数学",
	})

	require.NoError(t, err)
	require.Len(t, tagKeywordLines, 1)
	require.Equal(t, "TDD-1001", tagKeywordLines[0].UniversityCode)
	require.Equal(t, "理科试验班类", tagKeywordLines[0].LocalMajorName)
}

func cleanupAdmissionLineIntegrationData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(context.Background(), `
		DELETE FROM universities
		WHERE university_code IN ('TDD-1001', 'TDD-1003')
	`)
	require.NoError(t, err)
}

func seedAdmissionLineIntegrationData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO universities (university_code, name, normalized_name)
		VALUES
			('TDD-1001', 'TDD北京大学', 'TDD北京大学'),
			('TDD-1003', 'TDD清华大学', 'TDD清华大学')
	`)
	require.NoError(t, err)

	var pkuID int64
	var tsinghuaID int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM universities WHERE university_code = 'TDD-1001'`).Scan(&pkuID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM universities WHERE university_code = 'TDD-1003'`).Scan(&tsinghuaID))

	_, err = pool.Exec(ctx, `
		INSERT INTO admission_groups (
			university_id,
			admission_year,
			region_code,
			subject_category_code,
			batch_code,
			group_code,
			subject_requirement_code,
			education_level_code
		)
		VALUES
			($1, 2025, '230000', 'physics', 'regular_undergraduate', '009', 'physics_chemistry', 'undergraduate'),
			($2, 2025, '230000', 'physics', 'regular_undergraduate', '008', 'physics_chemistry', 'undergraduate'),
			($2, 2024, '230000', 'physics', 'regular_undergraduate', '008', 'physics_chemistry', 'undergraduate'),
			($2, 2025, '110000', 'physics', 'regular_undergraduate', '008', 'physics_chemistry', 'undergraduate')
	`, pkuID, tsinghuaID)
	require.NoError(t, err)

	insertAdmissionLine(t, pool, pkuID, 2025, "230000", "009", "31", "理科试验班类", 715, 80, 5300, "四年", "")
	insertAdmissionLine(t, pool, tsinghuaID, 2025, "230000", "008", "25", "计算机类", 721, 45, 5000, "四年", "含人工智能方向")
	insertAdmissionLine(t, pool, tsinghuaID, 2024, "230000", "008", "25", "计算机类", 700, 120, 5000, "四年", "")
	insertAdmissionLine(t, pool, tsinghuaID, 2025, "110000", "008", "25", "计算机类", 690, 200, 5000, "四年", "")
	insertAdmissionMajorTag(t, pool, pkuID, 2025, "230000", "009", "31", "07", "理学", "0701", "数学类", "070101", "数学与应用数学")
	insertAdmissionMajorTag(t, pool, tsinghuaID, 2025, "230000", "008", "25", "08", "工学", "0809", "计算机类", "080901", "计算机科学与技术")
}

func insertAdmissionLine(t *testing.T, pool *pgxpool.Pool, universityID int64, year int, regionCode string, groupCode string, localMajorCode string, localMajorName string, minScore int, minRank int, tuition int, duration string, remark string) {
	t.Helper()

	var groupID int64
	err := pool.QueryRow(context.Background(), `
		SELECT id
		FROM admission_groups
		WHERE university_id = $1
		  AND admission_year = $2
		  AND region_code = $3
		  AND subject_category_code = 'physics'
		  AND batch_code = 'regular_undergraduate'
		  AND group_code = $4
	`, universityID, year, regionCode, groupCode).Scan(&groupID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO university_major_admissions (
			admission_group_id,
			local_major_code,
			local_major_name,
			plan_count,
			min_score,
			min_rank,
			tuition,
			duration,
			admission_remark
		)
		VALUES ($1, $2, $3, 2, $4, $5, $6, $7, $8)
	`, groupID, localMajorCode, localMajorName, minScore, minRank, tuition, duration, remark)
	require.NoError(t, err)
}

func insertAdmissionMajorTag(t *testing.T, pool *pgxpool.Pool, universityID int64, year int, regionCode string, groupCode string, localMajorCode string, categoryCode string, categoryName string, classCode string, className string, majorCode string, majorName string) {
	t.Helper()

	var lineID int64
	err := pool.QueryRow(context.Background(), `
		SELECT uma.id
		FROM university_major_admissions uma
		JOIN admission_groups ag ON ag.id = uma.admission_group_id
		WHERE ag.university_id = $1
		  AND ag.admission_year = $2
		  AND ag.region_code = $3
		  AND ag.subject_category_code = 'physics'
		  AND ag.batch_code = 'regular_undergraduate'
		  AND ag.group_code = $4
		  AND uma.local_major_code = $5
	`, universityID, year, regionCode, groupCode, localMajorCode).Scan(&lineID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO admission_major_tags (
			university_major_admission_id,
			catalog_year,
			category_code,
			category_name,
			class_code,
			class_name,
			major_code,
			major_name,
			tag_level
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'major')
	`, lineID, year, categoryCode, categoryName, classCode, className, majorCode, majorName)
	require.NoError(t, err)
}
