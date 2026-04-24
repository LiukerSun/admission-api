package membership

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// ListPlans godoc
// @Summary      获取会员套餐列表
// @Description  返回当前可购买的 premium 月卡、季卡、年卡套餐
// @Tags         membership
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} web.Response{data=[]PlanResponse}
// @Failure      401 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/membership/plans [get]
func (h *Handler) ListPlans(c *gin.Context) {
	plans, err := h.service.ListPlans(c.Request.Context())
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list membership plans")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plans))
}

// GetCurrent godoc
// @Summary      获取当前会员状态
// @Description  返回当前用户的 premium 会员状态和有效期
// @Tags         membership
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} web.Response{data=CurrentMembershipResponse}
// @Failure      401 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/membership [get]
func (h *Handler) GetCurrent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.service.GetCurrent(c.Request.Context(), userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get membership")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func currentUserID(c *gin.Context) (int64, bool) {
	raw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := raw.(int64)
	return userID, ok && userID > 0
}

func WritePlanError(h *web.BaseHandler, c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, ErrPlanNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "membership plan not found")
		return true
	case errors.Is(err, ErrPlanNotPurchasable):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "membership plan is not purchasable")
		return true
	default:
		return false
	}
}
