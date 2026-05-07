package admission

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type UniversityHandler struct {
	web.BaseHandler
	service UniversityService
}

func NewUniversityHandler(service UniversityService) *UniversityHandler {
	return &UniversityHandler{service: service}
}

// ListUniversities godoc
// @Summary      List universities
// @Description  Returns university identities.
// @Tags         admission
// @Produce      json
// @Param        q query string false "Search university code or name"
// @Success      200 {object} web.Response{data=[]UniversityResponse}
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/universities [get]
func (h *UniversityHandler) ListUniversities(c *gin.Context) {
	resp, err := h.service.ListUniversities(c.Request.Context(), UniversityFilter{Query: c.Query("q")})
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list universities")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GetUniversityProfile godoc
// @Summary      Get university profile
// @Description  Returns a university yearly profile. If profile_year is omitted, the latest profile is used.
// @Tags         admission
// @Produce      json
// @Param        id path int true "University ID"
// @Param        profile_year query int false "Profile year"
// @Success      200 {object} web.Response{data=UniversityProfileResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/universities/{id}/profile [get]
func (h *UniversityHandler) GetUniversityProfile(c *gin.Context) {
	universityID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || universityID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid university id")
		return
	}

	var profileYear *int
	if raw := c.Query("profile_year"); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "profile_year must be a number")
			return
		}
		profileYear = &year
	}

	resp, err := h.service.GetUniversityProfile(c.Request.Context(), universityID, profileYear)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get university profile")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
