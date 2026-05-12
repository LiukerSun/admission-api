package admission

import (
	"net/http"
	"time"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

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
	res, err := h.refresher.Refresh(c.Request.Context(), opts)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, err.Error())
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(res))
}
