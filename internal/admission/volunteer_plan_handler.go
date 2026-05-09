package admission

import (
	"net/http"

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

// ListVolunteerPlans godoc
// @Summary      List volunteer plans
// @Description  Returns volunteer plans from plans.json.
// @Tags         admission
// @Produce      json
// @Success      200 {object} web.Response{data=VolunteerPlansResponse}
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/volunteer-plans [get]
func (h *VolunteerPlanHandler) ListVolunteerPlans(c *gin.Context) {
	resp, err := h.service.GetPlans()
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list volunteer plans")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
