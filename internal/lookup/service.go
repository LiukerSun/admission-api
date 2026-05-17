package lookup

import (
	"context"
	"errors"
	"fmt"
)

// DefaultPlanSize 是 region_plan_size_map 无命中时的兜底。
// 与 HLJ 普通本科批 40 个院校专业组一致，也跟 admission.defaultPlanSize 同值。
// 后者保留供 ai/tools.go 的 validator 兜底，不在 lookup 包内引用。
const DefaultPlanSize = 40

// rankWalkbackYears 是 year-walk 兜底向前看的最大年数。
// review 共识：最多回退 1 年；当年和 N-1 都缺则返回 ErrRankNotAvailable。
const rankWalkbackYears = 1

// RankSource 标识 LookupRank 实际命中的路径，供 snapshot 层决定是否在前端
// 提示「位次为估算值」。
type RankSource string

const (
	RankSourceExact    RankSource = "exact"     // 精确命中：表中存在该分
	RankSourceFloored  RankSource = "floored"   // 向下取整：用 score <= user_score 的最大分
	RankSourceBelowMin RankSource = "below_min" // 用户分数低于表内最低分，取最低分对应累计位次（保守上界）
	RankSourcePrevYear RankSource = "prev_year" // 当年数据未入库，回退用 (year-1) 的查询结果
)

// RankResult 是 LookupRank 的返回。
// Rank 是给推荐算法用的位次（cumulative_rank）；
// YearUsed / MatchedScore / Source 供上层日志、UI 提示、回归排查使用。
type RankResult struct {
	Rank         int
	YearUsed     int
	MatchedScore int
	Source       RankSource
}

// Service 是 lookup 包的业务入口。
type Service interface {
	// LookupRank 给定 (year, region, subject, score)，返回考生位次。
	// 优先精确命中 → 向下取整 → 表内最低分兜底；当年三条都没数据时回退
	// (year-1) 重复上面三步；仍失败则返回 ErrRankNotAvailable。
	LookupRank(ctx context.Context, year int, regionCode, subjectCode string, userScore int) (RankResult, error)

	// LookupPlanSize 给定 (year, region, subject)，返回志愿数。
	// 缺失时回退到 DefaultPlanSize。年份不做回退（plan_size 各年差异极小，
	// 缺数据时用默认值比用旧值更安全 —— 旧年的 plan_size 真分化时反而误导）。
	LookupPlanSize(ctx context.Context, year int, regionCode, subjectCode string) (int, error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) LookupRank(ctx context.Context, year int, regionCode, subjectCode string, userScore int) (RankResult, error) {
	// year-walk: 最多回退 rankWalkbackYears 年。
	for offset := 0; offset <= rankWalkbackYears; offset++ {
		y := year - offset
		matchedScore, rank, err := s.store.RankFloor(ctx, y, regionCode, subjectCode, userScore)
		if err == nil {
			source := RankSourceExact
			if matchedScore != userScore {
				source = RankSourceFloored
			}
			if offset > 0 {
				source = RankSourcePrevYear
			}
			return RankResult{
				Rank:         rank,
				YearUsed:     y,
				MatchedScore: matchedScore,
				Source:       source,
			}, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return RankResult{}, fmt.Errorf("rank floor (year=%d): %w", y, err)
		}

		// score 低于表内最低分 — 或者整个切片为空。再查最低分作为保守上界。
		minScore, maxRank, err := s.store.MinScoreRank(ctx, y, regionCode, subjectCode)
		if err == nil {
			source := RankSourceBelowMin
			if offset > 0 {
				// year-walk + below-min 两层兜底：优先标 prev_year，让前端提示更醒目
				source = RankSourcePrevYear
			}
			return RankResult{
				Rank:         maxRank,
				YearUsed:     y,
				MatchedScore: minScore,
				Source:       source,
			}, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return RankResult{}, fmt.Errorf("min score rank (year=%d): %w", y, err)
		}
		// 该年完全没数据，继续 year-walk。
	}
	return RankResult{}, ErrRankNotAvailable
}

func (s *service) LookupPlanSize(ctx context.Context, year int, regionCode, subjectCode string) (int, error) {
	size, err := s.store.PlanSize(ctx, year, regionCode, subjectCode)
	if err == nil {
		return size, nil
	}
	if errors.Is(err, ErrNotFound) {
		return DefaultPlanSize, nil
	}
	return 0, fmt.Errorf("plan size: %w", err)
}
