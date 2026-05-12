package admission

import (
	"net/http"

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
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, err.Error())
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
