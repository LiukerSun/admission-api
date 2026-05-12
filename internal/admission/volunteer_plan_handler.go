package admission

import (
	"fmt"
	"net/http"
	"strings"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type VolunteerPlanHandler struct {
	web.BaseHandler
	service *VolunteerPlanService
}

func NewVolunteerPlanHandler(service *VolunteerPlanService) *VolunteerPlanHandler {
	return &VolunteerPlanHandler{service: service}
}

// GetRichVolunteerPlan godoc
// @Summary      Get rich volunteer plan details
// @Description  Returns a detailed volunteer plan with user details, statistics, and rich group/major info.
// @Tags         admission
// @Produce      json
// @Param        id   path      int  true  "Volunteer Plan ID"
// @Success      200 {object} web.Response{data=RichVolunteerPlan}
// @Failure      401 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/volunteer-plans/{id}/rich-details [get]
func (h *VolunteerPlanHandler) GetRichVolunteerPlan(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	planIDStr := c.Param("id")
	var planID int64
	if _, err := fmt.Sscanf(planIDStr, "%d", &planID); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeInvalidParam, "invalid plan ID")
		return
	}

	richPlan, err := h.service.GetRichPlan(c.Request.Context(), userID, planID)
	if err != nil {
		if strings.Contains(err.Error(), "not found or unauthorized") {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, err.Error())
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get rich volunteer plan")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(richPlan))
}

// ListVolunteerPlans godoc
// @Summary      List volunteer plans
// @Description  Returns volunteer plans from plans.json.
// @Tags         admission
// @Produce      json
// @Success      200 {object} web.Response{data=VolunteerPlansResponse}
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/volunteer-plans [get]
func (h *VolunteerPlanHandler) ListVolunteerPlans(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	resp, err := h.service.GetPlans(c.Request.Context(), userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list volunteer plans")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

type UpdatePlanRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

func (h *VolunteerPlanHandler) UpdateVolunteerPlan(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	planIDStr := c.Param("id")
	var planID int64
	if _, err := fmt.Sscanf(planIDStr, "%d", &planID); err != nil {
    	h.RespondError(c, http.StatusBadRequest, web.ErrCodeInvalidParam, "invalid plan ID")
    	return
	}

	var req UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeInvalidParam, err.Error())
		return
	}

	if err := h.service.UpdatePlan(c.Request.Context(), userID, planID, req.Name, req.Description); err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to update volunteer plan")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}

type UpdateGroupRemarkRequest struct {
	Remark string `json:"remark"`
}

func (h *VolunteerPlanHandler) UpdateGroupRemark(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	groupIDStr := c.Param("groupId")
	var groupID int64
	if _, err := fmt.Sscanf(groupIDStr, "%d", &groupID); err != nil {
    	h.RespondError(c, http.StatusBadRequest, web.ErrCodeInvalidParam, "invalid group ID")
    	return
	}

	var req UpdateGroupRemarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeInvalidParam, err.Error())
		return
	}

	if err := h.service.UpdateGroupRemark(c.Request.Context(), userID, groupID, req.Remark); err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to update group remark")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}
