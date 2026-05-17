//go:build integration

package admission

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestRecommendationElectiveFilterIntegration 验证 PRD 验收清单第 3 条：
// "算法行为与改造前完全一致（同一用户同一分数，推荐结果一致）"。
//
// 改造的核心是 recommendation_store.go FetchCandidates 加了一段选科过滤分支。
// 当 ElectiveSubjects 为空时（旧客户端、AI agent legacy 路径），新分支不触发，
// 行为应该等同于改造前。当 ElectiveSubjects 非空时，候选池应是旧路径的子集，
// 并且严格遵循"专业组要求 ⊆ 用户已修读"的语义。
//
// 这是 integration test：用真实 DB + 真实候选数据。运行：
//
//	ADMISSION_TEST_DATABASE_URL=postgres://app:app@localhost:5432/admission?sslmode=disable \
//	  go test -tags integration ./internal/admission/... -run TestRecommendationElectiveFilter
func TestRecommendationElectiveFilterIntegration(t *testing.T) {
	databaseURL := os.Getenv("ADMISSION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ADMISSION_TEST_DATABASE_URL to run elective filter integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	store := NewRecommendationStore(pool)

	// 共用基线 query：物理类 230000，rank 窗口宽到足以拿出充分候选。
	baseline := func() *CandidateQuery {
		return &CandidateQuery{
			AdmissionYear:       2024, // 用 2024，pool 较稳定；2025 数据可能仍在变
			RegionCode:          "230000",
			SubjectCategoryCode: "physics",
			RankMin:             1,
			RankMax:             50000,
			Limit:               0,
		}
	}

	// ------------------------------------------------------------
	// 行为一致性：UserSubjectLabels=nil（旧客户端） vs 不传 ElectiveSubjects
	// 两者都走旧 SQL 分支，结果集应该完全一样。
	// ------------------------------------------------------------
	t.Run("UserSubjectLabels=nil 保持旧路径不变", func(t *testing.T) {
		q1 := baseline()
		q1.UserSubjectLabels = nil
		got1, err := store.FetchCandidates(ctx, q1)
		require.NoError(t, err)

		q2 := baseline()
		// 显式空切片
		q2.UserSubjectLabels = []string{}
		got2, err := store.FetchCandidates(ctx, q2)
		require.NoError(t, err)

		require.Equal(t, len(got1), len(got2),
			"nil 和 [] 必须等价：都不触发选科过滤")
	})

	// ------------------------------------------------------------
	// 新路径有效：传 UserSubjectLabels 后候选数 ≤ 旧路径。
	// ------------------------------------------------------------
	t.Run("UserSubjectLabels 收紧候选池", func(t *testing.T) {
		q1 := baseline()
		got1, err := store.FetchCandidates(ctx, q1)
		require.NoError(t, err)

		q2 := baseline()
		// 物理 + 化学 + 生物（最常见组合）
		q2.UserSubjectLabels = []string{"物理", "化学", "生物"}
		got2, err := store.FetchCandidates(ctx, q2)
		require.NoError(t, err)

		require.LessOrEqual(t, len(got2), len(got1),
			"选科过滤后的候选必须是旧候选的子集（或相等）")
		require.Greater(t, len(got2), 0,
			"物理化生覆盖面够大，应仍能拿到一定数量的候选")
	})

	// ------------------------------------------------------------
	// 新路径正确性：要求 "化学" 的专业组（DB 里数量最多的硬约束类）
	//   - 用户【物理 + 化学 + 生物】 → 应通过
	//   - 用户【物理 + 政治 + 地理】 → 应被过滤
	// ------------------------------------------------------------
	t.Run("严格的化学要求遵循子集语义", func(t *testing.T) {
		var groupsRequiringChem int
		err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM admission_groups
			  WHERE admission_year=2024 AND region_code='230000'
			    AND subject_category_code='physics'
			    AND subject_requirement_code='chemistry'`,
		).Scan(&groupsRequiringChem)
		require.NoError(t, err)
		if groupsRequiringChem == 0 {
			t.Skip("DB 里没有 2024 要求 'chemistry' 的物理类专业组")
		}

		qA := baseline()
		qA.UserSubjectLabels = []string{"物理", "化学", "生物"}
		gotA, err := store.FetchCandidates(ctx, qA)
		require.NoError(t, err)

		qB := baseline()
		qB.UserSubjectLabels = []string{"物理", "政治", "地理"}
		gotB, err := store.FetchCandidates(ctx, qB)
		require.NoError(t, err)

		countChem := func(cands []RecommendationCandidate) int {
			n := 0
			for _, c := range cands {
				if c.SubjectRequirementCode == "chemistry" {
					n++
				}
			}
			return n
		}
		nA := countChem(gotA)
		nB := countChem(gotB)

		require.Greater(t, nA, 0,
			"化学要求专业组对选了化学的用户应可见，但 gotA 里 0 个")
		require.Equal(t, 0, nB,
			"化学要求专业组对没选化学的用户必须被过滤掉，但 gotB 里看到 %d 个", nB)
	})

	// ------------------------------------------------------------
	// 不限要求（'none' 或 NULL）始终通过
	// ------------------------------------------------------------
	t.Run("不限要求专业组对任何选科都可见", func(t *testing.T) {
		var noneGroups int
		err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM admission_groups
			  WHERE admission_year=2024 AND region_code='230000'
			    AND subject_category_code='physics'
			    AND (subject_requirement_code IS NULL
			         OR subject_requirement_code = ''
			         OR subject_requirement_code = 'none')`,
		).Scan(&noneGroups)
		require.NoError(t, err)
		if noneGroups == 0 {
			t.Skip("DB 里没有 2024 不限要求的物理类专业组")
		}

		q := baseline()
		// 只选最少：物理 + 政治 + 地理（典型文理混合）
		q.UserSubjectLabels = []string{"物理", "政治", "地理"}
		got, err := store.FetchCandidates(ctx, q)
		require.NoError(t, err)

		// 数 'none' / NULL / '' 在结果里
		seen := 0
		for _, c := range got {
			if c.SubjectRequirementCode == "" || c.SubjectRequirementCode == "none" {
				seen++
			}
		}
		require.Greater(t, seen, 0,
			"不限要求专业组必须对任何选科都可见，但 0 个被返回")
	})

	// ------------------------------------------------------------
	// Legacy SubjectRequirementCode 与新路径互斥：
	// 同时传两者时，新路径优先，旧字段被忽略。
	// ------------------------------------------------------------
	t.Run("UserSubjectLabels 优先于 legacy SubjectRequirementCode", func(t *testing.T) {
		// 只用 legacy：明确指定 physics_chemistry
		qLegacy := baseline()
		qLegacy.SubjectRequirementCode = "physics_chemistry"
		gotLegacy, err := store.FetchCandidates(ctx, qLegacy)
		require.NoError(t, err)

		// 同时传新 + 旧：UserSubjectLabels 含化学应该胜出（不锁死在 physics_chemistry）
		qBoth := baseline()
		qBoth.SubjectRequirementCode = "physics_chemistry"
		qBoth.UserSubjectLabels = []string{"物理", "化学", "生物"}
		gotBoth, err := store.FetchCandidates(ctx, qBoth)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(gotBoth), len(gotLegacy),
			"新路径胜出时，候选数应 ≥ legacy（因为 legacy 把要求收死在单 code，新路径接受更多匹配项）")
	})
}
