package lookup

import "errors"

// ErrRankNotAvailable means we could not find any (year, region, subject)
// row in score_rank_map after walking back to year-1. Snapshot layer should
// turn this into a 422 — "今年一分一段表尚未入库，请稍后".
var ErrRankNotAvailable = errors.New("score-rank data not available for year or year-1")
