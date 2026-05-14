package analysis

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// GetTrend godoc
// @Summary      University admission trend
// @Description  Returns multi-year admission data for a university, optionally filtered by group and major.
// @Tags         analysis
// @Produce      json
// @Param        id path int true "University ID"
// @Param        group_code query string false "Admission group code"
// @Param        local_major_code query string false "Local major code"
// @Param        years query int false "Number of years to return (default 5)"
// @Success      200 {object} web.Response{data=TrendResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/universities/{id}/trend [get]
func (h *Handler) GetTrend(c *gin.Context) {
	universityID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || universityID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid university id")
		return
	}

	years := 5
	if raw := c.Query("years"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "years must be a positive number")
			return
		}
		years = v
	}

	filter := &TrendFilter{
		UniversityID:   universityID,
		GroupCode:      c.Query("group_code"),
		LocalMajorCode: c.Query("local_major_code"),
		Years:          years,
	}

	resp, err := h.service.GetTrend(c.Request.Context(), filter)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get trend")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GetGroupComparison godoc
// @Summary      University group comparison
// @Description  Returns all admission groups for a university in a given year with aggregated metrics.
// @Tags         analysis
// @Produce      json
// @Param        id path int true "University ID"
// @Param        admission_year query int false "Admission year (default latest)"
// @Param        region_code query string false "Region code"
// @Param        subject_category_code query string false "Subject category code"
// @Success      200 {object} web.Response{data=GroupComparisonResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/universities/{id}/groups [get]
func (h *Handler) GetGroupComparison(c *gin.Context) {
	universityID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || universityID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid university id")
		return
	}

	filter := &GroupComparisonFilter{
		UniversityID:        universityID,
		RegionCode:          c.Query("region_code"),
		SubjectCategoryCode: c.Query("subject_category_code"),
	}

	if raw := c.Query("admission_year"); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "admission_year must be a number")
			return
		}
		filter.AdmissionYear = &year
	}

	resp, err := h.service.GetGroupComparison(c.Request.Context(), filter)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get group comparison")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GetMajorDistribution godoc
// @Summary      Major distribution within a group
// @Description  Returns per-major metrics for a university admission group.
// @Tags         analysis
// @Produce      json
// @Param        id path int true "University ID"
// @Param        group_code query string true "Admission group code"
// @Param        admission_year query int false "Admission year (default latest)"
// @Param        region_code query string false "Region code"
// @Param        subject_category_code query string false "Subject category code"
// @Success      200 {object} web.Response{data=MajorDistributionResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/universities/{id}/majors/distribution [get]
func (h *Handler) GetMajorDistribution(c *gin.Context) {
	universityID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || universityID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid university id")
		return
	}

	groupCode := c.Query("group_code")
	if groupCode == "" {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "group_code is required")
		return
	}

	filter := &MajorDistributionFilter{
		UniversityID:        universityID,
		GroupCode:           groupCode,
		RegionCode:          c.Query("region_code"),
		SubjectCategoryCode: c.Query("subject_category_code"),
	}

	if raw := c.Query("admission_year"); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "admission_year must be a number")
			return
		}
		filter.AdmissionYear = &year
	}

	resp, err := h.service.GetMajorDistribution(c.Request.Context(), filter)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get major distribution")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GetMajorComparison godoc
// @Summary      Cross-university major comparison
// @Description  Returns the same major across multiple universities for comparison.
// @Tags         analysis
// @Produce      json
// @Param        local_major_name query string true "Local major name"
// @Param        admission_year query int false "Admission year (default latest)"
// @Param        region_code query string false "Region code"
// @Param        subject_category_code query string false "Subject category code"
// @Param        limit query int false "Max results (default 50)"
// @Success      200 {object} web.Response{data=MajorComparisonResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/majors/comparison [get]
func (h *Handler) GetMajorComparison(c *gin.Context) {
	localMajorName := c.Query("local_major_name")
	if localMajorName == "" {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "local_major_name is required")
		return
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "limit must be a positive number")
			return
		}
		limit = v
	}

	filter := &MajorComparisonFilter{
		LocalMajorName:      localMajorName,
		RegionCode:          c.Query("region_code"),
		SubjectCategoryCode: c.Query("subject_category_code"),
		Limit:               limit,
	}

	if raw := c.Query("admission_year"); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "admission_year must be a number")
			return
		}
		filter.AdmissionYear = &year
	}

	resp, err := h.service.GetMajorComparison(c.Request.Context(), filter)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get major comparison")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
