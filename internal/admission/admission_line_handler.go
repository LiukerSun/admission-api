package admission

import (
	"net/http"
	"strconv"
	"strings"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type AdmissionLineHandler struct { //nolint:revive // Matches route constructor naming.
	web.BaseHandler
	service AdmissionLineService
}

func NewAdmissionLineHandler(service AdmissionLineService) *AdmissionLineHandler {
	return &AdmissionLineHandler{service: service}
}

// ListAdmissionLines godoc
// @Summary      List admission lines
// @Description  Returns school-major admission lines with university, admission group, local school major, score, rank, and plan fields. If admission_year is omitted, the latest available admission year is used for the selected region and subject category.
// @Tags         admission
// @Produce      json
// @Param        admission_year query int false "Admission year"
// @Param        region_code query string false "Region code"
// @Param        subject_category_code query string false "Subject category code"
// @Param        university_ids query string false "Comma-separated internal university IDs"
// @Param        university_codes query string false "Comma-separated school-published university codes"
// @Param        group_codes query string false "Comma-separated admission group codes"
// @Param        tag_catalog_year query int false "CHSI tag catalog year"
// @Param        tag_query query string false "CHSI tag keyword, matching category/class/major code or name"
// @Param        tag_category_code query string false "CHSI major category code"
// @Param        tag_class_code query string false "CHSI major class code"
// @Param        tag_major_code query string false "CHSI standard major code"
// @Param        min_rank_from query int false "Minimum rank lower bound"
// @Param        min_rank_to query int false "Minimum rank upper bound"
// @Param        min_score_from query int false "Minimum score lower bound"
// @Param        min_score_to query int false "Minimum score upper bound"
// @Success      200 {object} web.Response
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/admission-lines [get]
func (h *AdmissionLineHandler) ListAdmissionLines(c *gin.Context) {
	filter, ok := h.parseAdmissionLineFilter(c)
	if !ok {
		return
	}
	resp, err := h.service.ListAdmissionLines(c.Request.Context(), &filter)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list admission lines")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *AdmissionLineHandler) parseAdmissionLineFilter(c *gin.Context) (AdmissionLineFilter, bool) {
	filter := AdmissionLineFilter{
		RegionCode:          c.Query("region_code"),
		SubjectCategoryCode: c.Query("subject_category_code"),
		UniversityCodes:     splitCSV(c.Query("university_codes")),
		GroupCodes:          splitCSV(c.Query("group_codes")),
		Cities:              splitCSV(c.Query("cities")),
		ExcludeCities:       splitCSV(c.Query("exclude_cities")),
		Provinces:           splitCSV(c.Query("provinces")),
		ExcludeProvinces:    splitCSV(c.Query("exclude_provinces")),
		SubjectCategories:   splitCSV(c.Query("subject_categories")),
		TagQuery:            c.Query("tag_query"),
		TagCategoryCode:     c.Query("tag_category_code"),
		TagClassCode:        c.Query("tag_class_code"),
		TagMajorCode:        c.Query("tag_major_code"),
	}
	if raw := c.Query("admission_year"); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "admission_year must be a number")
			return filter, false
		}
		filter.AdmissionYear = &year
	}
	if raw := c.Query("university_ids"); raw != "" {
		ids, err := parseInt64CSV(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "university_ids must be comma-separated numbers")
			return filter, false
		}
		filter.UniversityIDs = ids
	}
	intParams := []struct {
		name   string
		target **int
	}{
		{"tag_catalog_year", &filter.TagCatalogYear},
		{"min_rank_from", &filter.MinRankFrom},
		{"min_rank_to", &filter.MinRankTo},
		{"min_score_from", &filter.MinScoreFrom},
		{"min_score_to", &filter.MinScoreTo},
	}
	for _, param := range intParams {
		if raw := c.Query(param.name); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, param.name+" must be a number")
				return filter, false
			}
			*param.target = &value
		}
	}
	boolParams := []struct {
		name   string
		target **bool
	}{
		{"is_985", &filter.Is985},
		{"is_211", &filter.Is211},
		{"is_double_first_class", &filter.IsDoubleFirstClass},
	}
	for _, param := range boolParams {
		if raw := c.Query(param.name); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, param.name+" must be a boolean")
				return filter, false
			}
			*param.target = &value
		}
	}
	return filter, true
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parseInt64CSV(raw string) ([]int64, error) {
	parts := splitCSV(raw)
	values := make([]int64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
