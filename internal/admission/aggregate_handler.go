package admission

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type AggregateHandler struct {
	web.BaseHandler
	service AggregateService
}

func NewAggregateHandler(service AggregateService) *AggregateHandler {
	return &AggregateHandler{service: service}
}

// Aggregate godoc
// @Summary      Aggregate admission data
// @Description  Aggregates admission lines by a dimension (province, city, subject_category, university, group). Supports metrics: count, avg_min_score, avg_min_rank, avg_tuition, is_985_count, is_211_count, is_double_first_class_count.
// @Tags         admission
// @Produce      json
// @Param        admission_year query int false "Admission year"
// @Param        region_code query string false "Region code"
// @Param        subject_category_code query string false "Subject category code"
// @Param        university_ids query string false "Comma-separated internal university IDs"
// @Param        university_codes query string false "Comma-separated school-published university codes"
// @Param        group_codes query string false "Comma-separated admission group codes"
// @Param        tag_catalog_year query int false "CHSI tag catalog year"
// @Param        tag_query query string false "CHSI tag keyword"
// @Param        tag_category_code query string false "CHSI major category code"
// @Param        tag_class_code query string false "CHSI major class code"
// @Param        tag_major_code query string false "CHSI standard major code"
// @Param        min_rank_from query int false "Minimum rank lower bound"
// @Param        min_rank_to query int false "Minimum rank upper bound"
// @Param        min_score_from query int false "Minimum score lower bound"
// @Param        min_score_to query int false "Minimum score upper bound"
// @Param        is_985 query bool false "Filter by 985 status"
// @Param        is_211 query bool false "Filter by 211 status"
// @Param        is_double_first_class query bool false "Filter by double-first-class status"
// @Param        group_by query string true "Dimension to group by: province, city, subject_category, university, group"
// @Param        metrics query string true "Comma-separated metrics: count, avg_min_score, avg_min_rank, avg_tuition, is_985_count, is_211_count, is_double_first_class_count"
// @Success      200 {object} web.Response{data=AggregateResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/aggregate [get]
func (h *AggregateHandler) Aggregate(c *gin.Context) {
	filter, ok := h.parseAggregateFilter(c)
	if !ok {
		return
	}
	resp, err := h.service.Aggregate(c.Request.Context(), &filter)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to aggregate admission data")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *AggregateHandler) parseAggregateFilter(c *gin.Context) (AggregateFilter, bool) {
	filter := AggregateFilter{
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
		GroupBy:             c.Query("group_by"),
		Metrics:             splitCSV(c.Query("metrics")),
	}
	if filter.GroupBy == "" {
		filter.GroupBy = "province"
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
