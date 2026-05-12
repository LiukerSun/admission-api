package admission

import (
	"context"
	"net/http"
	"time"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// refreshHandlerBudget is the overall ceiling we wrap each /admin/recommendation/scores/refresh
// request in. Gin's WriteTimeout is 15s and the LLM evaluator can take ~6s per row, so we leave
// a 2s margin so partial results still serialize cleanly even if the batch hits the cap.
const refreshHandlerBudget = 13 * time.Second

type RecommendationScoreHandler struct {
	web.BaseHandler
	refresher *RecommendationScoreRefresher
}

func NewRecommendationScoreHandler(refresher *RecommendationScoreRefresher) *RecommendationScoreHandler {
	return &RecommendationScoreHandler{refresher: refresher}
}

type refreshScoreRequest struct {
	MaxAgeDays int `json:"max_age_days,omitempty"` // default 90
	Limit      int `json:"limit,omitempty"`        // default 50
}

// Refresh godoc
// @Summary      刷新推荐打分缓存（管理员）
// @Description  扫描 recommendation_precomputed_scores 中缺失或过期（默认 90 天）的 (university × major) 行，逐个调用评估器（默认 LLM）填充五维基准分。可指定批量大小。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        body body refreshScoreRequest false "刷新参数"
// @Success      200 {object} web.Response{data=RefreshResult}
// @Failure      500 {object} web.Response
// @Security     BearerAuth
// @Router       /api/v1/admin/recommendation/scores/refresh [post]
func (h *RecommendationScoreHandler) Refresh(c *gin.Context) {
	var body refreshScoreRequest
	_ = c.ShouldBindJSON(&body) // body is optional

	opts := RefreshOptions{
		MaxAge: time.Duration(body.MaxAgeDays) * 24 * time.Hour,
		Limit:  body.Limit,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), refreshHandlerBudget)
	defer cancel()

	res, err := h.refresher.Refresh(ctx, opts)
	// ctx.Err() != nil means we hit the overall budget; res may still contain
	// partially-evaluated rows that were upserted before the deadline.
	if err != nil && ctx.Err() == nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, err.Error())
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(res))
}
