package snapshot

import "errors"

// 推荐 snapshot 构造期的领域错误。Handler 层把它们翻译成具体 HTTP 状态。
var (
	// ErrProfileIncomplete: user_profiles 表里 region/subject/electives/total_score
	// 四要素之一缺失。HTTP 应返回 422 + 提示用户先把问卷填完。
	ErrProfileIncomplete = errors.New("profile is incomplete: region/subject/electives/total_score required")

	// ErrRankDataMissing: 当年 + 前一年的 score_rank_map 都没数据。
	// 包装 lookup.ErrRankNotAvailable，handler 应返回 422 + 提示「一分一段表尚未入库」。
	ErrRankDataMissing = errors.New("score-rank data not available for current year or previous year")
)
