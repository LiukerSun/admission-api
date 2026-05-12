package admission

import (
	"log/slog"
	"net/http"
	"strings"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type RecommendationHandler struct {
	web.BaseHandler
	service RecommendationService
}

func NewRecommendationHandler(service RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{service: service}
}

// Recommend godoc
// @Summary      生成志愿推荐表
// @Description  基于学生省份、选科、分数、位次、单科成绩、家庭资源、个人画像、地域偏好、专业偏好、职业规划，输出冲/稳/保三档志愿表，并对每个志愿给出综合评分与推荐理由。
// @Tags         admission
// @Accept       json
// @Produce      json
// @Param        body body RecommendationRequest true "学生画像与偏好"
// @Success      200 {object} web.Response{data=RecommendationResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Security     BearerAuth
// @Router       /api/v1/admission/recommendations [post]
func (h *RecommendationHandler) Recommend(c *gin.Context) {
	var req RecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.Recommend(c.Request.Context(), &req)
	if err != nil {
		// validateRequest 的错误属于用户输入问题 → 400；其余按服务侧 500 处理。
		// 两路都 log，方便定位 500 究竟挂在哪一步。
		status := http.StatusInternalServerError
		msg := err.Error()
		if strings.Contains(msg, "is required") ||
			strings.Contains(msg, "must be positive") ||
			strings.Contains(msg, "nil request") {
			status = http.StatusBadRequest
		}
		// "no admission data for region=X category=Y" 是用户填了一个 DB 里没数据
		// 的省份/科类组合，对用户友好地回 400 + 中文提示，而不是 500。
		if strings.Contains(msg, "no admission data") {
			status = http.StatusBadRequest
			msg = "暂不支持该省份/科类组合的志愿推荐（目前仅黑龙江物理类/历史类数据完备）"
		}
		slog.Error("recommendation request failed",
			"error", msg,
			"status", status,
			"region", req.RegionCode,
			"subject_category", req.SubjectCategoryCode,
			"rank", req.ProvincialRank,
			"plan_size", req.PlanSize,
			"enable_llm_tuning", req.EnableLLMTuning,
		)
		h.RespondError(c, status, web.ErrCodeInternal, msg)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
