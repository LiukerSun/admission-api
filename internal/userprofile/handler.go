package userprofile

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

// Handler exposes /me/profile to the HTTP layer.
type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// GetMe godoc
// @Summary      获取当前用户的志愿调查问卷档案
// @Description  返回当前用户保存的 region/subject/score/rank 等基础信息与偏好；从未填写时返回空档案 + completed=false（不会 404）。
// @Tags         user-profile
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/me/profile [get]
func (h *Handler) GetMe(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.service.GetMyProfile(c.Request.Context(), userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to load profile")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// UpsertMe godoc
// @Summary      创建或更新当前用户的志愿调查问卷档案
// @Description  PUT 语义：传入的字段会整体覆盖既有档案；未传字段视为 NULL/缺省。4 项必填齐时自动写入 completed_at。
// @Tags         user-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body UpsertRequest true "Profile payload"
// @Success      200 {object} web.Response
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/me/profile [put]
func (h *Handler) UpsertMe(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	var req UpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	resp, err := h.service.UpsertMyProfile(c.Request.Context(), userID, &req)
	if err != nil {
		if status, code, msg, ok := mapValidationError(err); ok {
			h.RespondError(c, status, code, msg)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to save profile")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// mapValidationError translates domain errors into HTTP responses. Each rule
// has a specific message so the frontend can either surface it or map to its
// own localized copy.
func mapValidationError(err error) (status, code int, msg string, matched bool) {
	switch {
	case errors.Is(err, ErrInvalidRegion):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "省份代码格式不正确", true
	case errors.Is(err, ErrInvalidSubject):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "选科类别取值不合法", true
	case errors.Is(err, ErrInvalidStrategy):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "填报策略取值不合法", true
	case errors.Is(err, ErrScoreOutOfRange):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "总分超出 0-750 范围", true
	case errors.Is(err, ErrRankOutOfRange):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "省内位次超出允许范围", true
	case errors.Is(err, ErrPlanSizeOutOfRange):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "志愿数应在 1-96 之间", true
	case errors.Is(err, ErrSubjectScoreInvalid):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "单科成绩超出 0-150 范围", true
	case errors.Is(err, ErrPreferenceTooLong):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "偏好字段长度超出限制", true
	case errors.Is(err, ErrInvalidHollandCode):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "霍兰德兴趣代码格式不正确", true
	case errors.Is(err, ErrInvalidBudget):
		return http.StatusBadRequest, web.ErrCodeBadRequest, "学费预算超出允许范围", true
	}
	return 0, 0, "", false
}
