//go:build integration

package lookup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestStoreIntegration 验证 store 层 SQL 在真实 Postgres 上行为正确。
//
// 前置：测试库里已有 2025 黑龙江一分一段 seed（scripts/seed_2025_hlj_score_rank.sql），
// 否则跳过。这与开发环境的实际状态一致 —— 不重新建 fixture，避免与生产数据偏移。
//
// 运行方式：
//
//	ADMISSION_TEST_DATABASE_URL=postgres://app:app@localhost:5432/admission?sslmode=disable \
//	  go test -tags integration ./internal/lookup/...
func TestStoreIntegration(t *testing.T) {
	databaseURL := os.Getenv("ADMISSION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ADMISSION_TEST_DATABASE_URL to run lookup store integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	// 探测：seed 是否在库里。不在则跳过（避免污染失败信号）。
	var rowCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM score_rank_map
		  WHERE year=2025 AND region_code='230000' AND subject_category_code='physics'`,
	).Scan(&rowCount))
	if rowCount == 0 {
		t.Skip("score_rank_map seed for (2025, 230000, physics) missing; run scripts/seed_2025_hlj_score_rank.sql")
	}

	s := NewStore(pool)

	t.Run("RankFloor exact 600 → 5997", func(t *testing.T) {
		ms, r, err := s.RankFloor(ctx, 2025, "230000", "physics", 600)
		require.NoError(t, err)
		require.Equal(t, 600, ms)
		require.Equal(t, 5997, r)
	})

	t.Run("RankFloor over-max 750 → 694/34", func(t *testing.T) {
		ms, r, err := s.RankFloor(ctx, 2025, "230000", "physics", 750)
		require.NoError(t, err)
		require.Equal(t, 694, ms)
		require.Equal(t, 34, r)
	})

	t.Run("RankFloor below-min 100 → ErrNotFound", func(t *testing.T) {
		_, _, err := s.RankFloor(ctx, 2025, "230000", "physics", 100)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("RankFloor unknown year → ErrNotFound", func(t *testing.T) {
		_, _, err := s.RankFloor(ctx, 2099, "230000", "physics", 600)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("MinScoreRank physics → 130/117407", func(t *testing.T) {
		ms, r, err := s.MinScoreRank(ctx, 2025, "230000", "physics")
		require.NoError(t, err)
		require.Equal(t, 130, ms)
		require.Equal(t, 117407, r)
	})

	t.Run("MinScoreRank history → 130/54707", func(t *testing.T) {
		ms, r, err := s.MinScoreRank(ctx, 2025, "230000", "history")
		require.NoError(t, err)
		require.Equal(t, 130, ms)
		require.Equal(t, 54707, r)
	})

	t.Run("MinScoreRank empty slice → ErrNotFound", func(t *testing.T) {
		_, _, err := s.MinScoreRank(ctx, 2099, "230000", "physics")
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("PlanSize physics → 40", func(t *testing.T) {
		size, err := s.PlanSize(ctx, 2025, "230000", "physics")
		require.NoError(t, err)
		require.Equal(t, 40, size)
	})

	t.Run("PlanSize unknown → ErrNotFound", func(t *testing.T) {
		_, err := s.PlanSize(ctx, 2099, "230000", "physics")
		require.ErrorIs(t, err, ErrNotFound)
	})

	// Service 层 + 真 store 端到端：验证 year-walk 不会因为接口断层失效。
	t.Run("Service LookupRank prev_year walkback", func(t *testing.T) {
		svc := NewService(s)
		// 2026 数据未入库，回退到 2025。
		res, err := svc.LookupRank(ctx, 2026, "230000", "physics", 600)
		require.NoError(t, err)
		require.Equal(t, 2025, res.YearUsed)
		require.Equal(t, RankSourcePrevYear, res.Source)
		require.Equal(t, 5997, res.Rank)
	})

	t.Run("Service LookupRank below-min uses min score", func(t *testing.T) {
		svc := NewService(s)
		res, err := svc.LookupRank(ctx, 2025, "230000", "physics", 100)
		require.NoError(t, err)
		require.Equal(t, RankSourceBelowMin, res.Source)
		require.Equal(t, 130, res.MatchedScore)
		require.Equal(t, 117407, res.Rank)
	})

	t.Run("Service LookupRank too-far walkback returns ErrRankNotAvailable", func(t *testing.T) {
		svc := NewService(s)
		_, err := svc.LookupRank(ctx, 2099, "230000", "physics", 600)
		require.True(t, errors.Is(err, ErrRankNotAvailable))
	})

	t.Run("Service LookupPlanSize default fallback", func(t *testing.T) {
		svc := NewService(s)
		size, err := svc.LookupPlanSize(ctx, 2099, "230000", "physics")
		require.NoError(t, err)
		require.Equal(t, DefaultPlanSize, size)
	})
}
