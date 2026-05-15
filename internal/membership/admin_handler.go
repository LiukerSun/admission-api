package membership

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/web"
)

// AdminHandler exposes plan CRUD for the admin dashboard. It reuses the
// existing Service so all plan state changes flow through the same business
// validations (gt=0 duration, currency length, etc).
type AdminHandler struct {
	web.BaseHandler
	service Service
}

func NewAdminHandler(service Service) *AdminHandler {
	return &AdminHandler{service: service}
}

// AdminListPlans godoc
// @Summary      管理员列出所有套餐
// @Description  返回全部 membership 套餐（含 inactive 与已禁用），按 sort_order 升序、duration_days 升序排序。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} web.Response{data=[]PlanResponse}
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admin/membership/plans [get]
func (h *AdminHandler) AdminListPlans(c *gin.Context) {
	plans, err := h.service.AdminListPlans(c.Request.Context())
	if err != nil {
		slog.Error("admin list plans", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list plans")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plans))
}

// AdminGetPlan godoc
// @Summary      管理员获取套餐详情
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Plan ID"
// @Success      200 {object} web.Response{data=PlanResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      404 {object} web.Response
// @Router       /api/v1/admin/membership/plans/{id} [get]
func (h *AdminHandler) AdminGetPlan(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	plan, err := h.service.AdminGetPlan(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "plan not found")
			return
		}
		slog.Error("admin get plan", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get plan")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plan))
}

// AdminCreatePlan godoc
// @Summary      管理员创建套餐
// @Description  plan_code 是稳定业务键，创建后无法修改；建议使用如 monthly/quarterly/yearly/lifetime 这类语义化字符串。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  PlanCreateRequest  true  "套餐"
// @Success      200 {object} web.Response{data=PlanResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      409 {object} web.Response  "plan_code 已存在"
// @Failure      500 {object} web.Response
// @Router       /api/v1/admin/membership/plans [post]
func (h *AdminHandler) AdminCreatePlan(c *gin.Context) {
	var req PlanCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	plan, err := h.service.AdminCreatePlan(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrPlanCodeExists) {
			h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "plan_code already exists")
			return
		}
		slog.Error("admin create plan", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to create plan")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plan))
}

// AdminUpdatePlan godoc
// @Summary      管理员更新套餐
// @Description  仅传入需要修改的字段（partial update）。plan_code 不可改。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int                true   "Plan ID"
// @Param        body  body  PlanUpdateRequest  true   "套餐字段，可部分提交"
// @Success      200 {object} web.Response{data=PlanResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admin/membership/plans/{id} [put]
func (h *AdminHandler) AdminUpdatePlan(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req PlanUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	plan, err := h.service.AdminUpdatePlan(c.Request.Context(), id, &req)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "plan not found")
			return
		}
		slog.Error("admin update plan", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to update plan")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(plan))
}

// AdminDeletePlan godoc
// @Summary      管理员删除套餐
// @Description  如果有 payment_orders 引用此套餐，则不会物理删除，而是把 status 置为 inactive
// @Description  并在响应里返回 soft_deleted=true、reference_rows=被引用次数。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Plan ID"
// @Success      200 {object} web.Response{data=PlanDeleteResult}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admin/membership/plans/{id} [delete]
func (h *AdminHandler) AdminDeletePlan(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	result, err := h.service.AdminDeletePlan(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "plan not found")
			return
		}
		slog.Error("admin delete plan", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to delete plan")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

func (h *AdminHandler) parseID(c *gin.Context) (int64, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid plan id")
		return 0, false
	}
	return id, true
}
