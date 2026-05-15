//go:build integration

package admission

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestAggregateByProvinceReturnsCorrectCounts(t *testing.T) {
	databaseURL := os.Getenv("ADMISSION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ADMISSION_TEST_DATABASE_URL to run aggregate store integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	cleanupAggregateIntegrationData(t, pool)
	seedAggregateIntegrationData(t, pool)

	store := NewAggregateStore(pool)
	resp, err := store.Aggregate(ctx, &AggregateFilter{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		GroupBy:             "province",
		Metrics:             []string{"count"},
	})

	require.NoError(t, err)
	require.Equal(t, "province", resp.GroupBy)
	require.Equal(t, int64(3), resp.Total)
	require.Len(t, resp.Items, 2)

	// Province with code "110000" (Beijing) should have 2 lines
	beijing := findAggregateItem(resp.Items, "110000")
	require.NotNil(t, beijing)
	require.Equal(t, int64(2), beijing.Count)

	// Province with code "310000" (Shanghai) should have 1 line
	shanghai := findAggregateItem(resp.Items, "310000")
	require.NotNil(t, shanghai)
	require.Equal(t, int64(1), shanghai.Count)
}

func TestAggregateByProvinceWithAvgMinScore(t *testing.T) {
	databaseURL := os.Getenv("ADMISSION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ADMISSION_TEST_DATABASE_URL to run aggregate store integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	cleanupAggregateIntegrationData(t, pool)
	seedAggregateIntegrationData(t, pool)

	store := NewAggregateStore(pool)
	resp, err := store.Aggregate(ctx, &AggregateFilter{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		GroupBy:             "province",
		Metrics:             []string{"count", "avg_min_score"},
	})

	require.NoError(t, err)
	require.Equal(t, int64(3), resp.Total)

	beijing := findAggregateItem(resp.Items, "110000")
	require.NotNil(t, beijing)
	require.NotNil(t, beijing.AvgMinScore)
	require.InDelta(t, 718.0, *beijing.AvgMinScore, 0.5)
}

func findAggregateItem(items []AggregateItem, code string) *AggregateItem {
	for i := range items {
		if items[i].Code == code {
			return &items[i]
		}
	}
	return nil
}

func cleanupAggregateIntegrationData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		DELETE FROM universities
		WHERE university_code IN ('AGG-1001', 'AGG-1002', 'AGG-1003')
	`)
	require.NoError(t, err)
}

func seedAggregateIntegrationData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO universities (university_code, name, normalized_name)
		VALUES
			('AGG-1001', 'AGG北京大学', 'AGG北京大学'),
			('AGG-1002', 'AGG清华大学', 'AGG清华大学'),
			('AGG-1003', 'AGG复旦大学', 'AGG复旦大学')
	`)
	require.NoError(t, err)

	var pkuID, tsinghuaID, fudanID int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM universities WHERE university_code = 'AGG-1001'`).Scan(&pkuID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM universities WHERE university_code = 'AGG-1002'`).Scan(&tsinghuaID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM universities WHERE university_code = 'AGG-1003'`).Scan(&fudanID))

	_, err = pool.Exec(ctx, `
		INSERT INTO admission_groups (
			university_id, admission_year, region_code, subject_category_code,
			batch_code, group_code, subject_requirement_code, education_level_code
		)
		VALUES
			($1, 2025, '230000', 'physics', 'regular_undergraduate', '009', 'physics_chemistry', 'undergraduate'),
			($2, 2025, '230000', 'physics', 'regular_undergraduate', '008', 'physics_chemistry', 'undergraduate'),
			($3, 2025, '230000', 'physics', 'regular_undergraduate', '001', 'physics_chemistry', 'undergraduate')
	`, pkuID, tsinghuaID, fudanID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO university_profiles (university_id, profile_year, region_code, city, is_985, is_211, is_double_first_class)
		VALUES
			($1, 2025, '110000', '北京', true, true, true),
			($2, 2025, '110000', '北京', false, true, true),
			($3, 2025, '310000', '上海', true, true, true)
	`, pkuID, tsinghuaID, fudanID)
	require.NoError(t, err)

	insertAggregateAdmissionLine(t, pool, pkuID, 2025, "230000", "009", "31", "理科试验班类", 715)
	insertAggregateAdmissionLine(t, pool, tsinghuaID, 2025, "230000", "008", "25", "计算机类", 721)
	insertAggregateAdmissionLine(t, pool, fudanID, 2025, "230000", "001", "10", "数学类", 680)
}

func insertAggregateAdmissionLine(t *testing.T, pool *pgxpool.Pool, universityID int64, year int, regionCode string, groupCode string, localMajorCode string, localMajorName string, minScore int) {
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
			admitted_count,
			min_score,
			min_rank,
			tuition,
			duration,
			admission_remark
		)
		VALUES ($1, $2, $3, 2, $4, 100, 5000, '四年', '')
	`, groupID, localMajorCode, localMajorName, minScore)
	require.NoError(t, err)
}
