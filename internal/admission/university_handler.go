package admission

import (
	"net/http"
	"strconv"
	"strings"

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

func parseTriBool(value string) *bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		v := true
		return &v
	case "0", "false", "no":
		v := false
		return &v
	default:
		return nil
	}
}

// ListUniversities godoc
// @Summary      List universities
// @Description  Returns university identities enriched with latest profile information. Filters can narrow the result by region, school category, ownership type, education level, and university-tier flags.
// @Tags         admission
// @Produce      json
// @Param        q query string false "Search university code or name"
// @Param        region_codes query string false "Comma-separated region codes"
// @Param        school_category_codes query string false "Comma-separated school category codes"
// @Param        ownership_type_codes query string false "Comma-separated school ownership type codes"
// @Param        education_level_code query string false "Education level code"
// @Param        is_985 query string false "Filter by 985 (1/0)"
// @Param        is_211 query string false "Filter by 211 (1/0)"
// @Param        is_double_first_class query string false "Filter by double-first-class (1/0)"
// @Param        is_national_key query string false "Filter by national-key (1/0)"
// @Param        is_provincial_key query string false "Filter by provincial-key (1/0)"
// @Param        has_postgraduate_recommendation query string false "Filter by postgraduate-recommendation eligibility (1/0)"
// @Success      200 {object} web.Response{data=[]UniversityResponse}
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/universities [get]
func (h *UniversityHandler) ListUniversities(c *gin.Context) {
	filter := UniversityFilter{
		Query:                         c.Query("q"),
		RegionCodes:                   splitCSV(c.Query("region_codes")),
		SchoolCategoryCodes:           splitCSV(c.Query("school_category_codes")),
		OwnershipTypeCodes:            splitCSV(c.Query("ownership_type_codes")),
		EducationLevelCode:            strings.TrimSpace(c.Query("education_level_code")),
		Is985:                         parseTriBool(c.Query("is_985")),
		Is211:                         parseTriBool(c.Query("is_211")),
		IsDoubleFirstClass:            parseTriBool(c.Query("is_double_first_class")),
		IsNationalKey:                 parseTriBool(c.Query("is_national_key")),
		IsProvincialKey:               parseTriBool(c.Query("is_provincial_key")),
		HasPostgraduateRecommendation: parseTriBool(c.Query("has_postgraduate_recommendation")),
	}

	resp, err := h.service.ListUniversities(c.Request.Context(), &filter)
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
