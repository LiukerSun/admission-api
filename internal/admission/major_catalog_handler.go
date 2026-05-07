package admission

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

type MajorCatalogHandler struct {
	web.BaseHandler
	service MajorCatalogService
}

func NewMajorCatalogHandler(service MajorCatalogService) *MajorCatalogHandler {
	return &MajorCatalogHandler{service: service}
}

// LatestCatalogYear godoc
// @Summary      Get latest major catalog year
// @Description  Returns the latest available CHSI standard major catalog year.
// @Tags         admission
// @Produce      json
// @Success      200 {object} web.Response{data=LatestCatalogYearResponse}
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/major-catalog/latest-year [get]
func (h *MajorCatalogHandler) LatestCatalogYear(c *gin.Context) {
	year, err := h.service.LatestCatalogYear(c.Request.Context())
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to get latest major catalog year")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(LatestCatalogYearResponse{CatalogYear: year}))
}

// ListStandardMajors godoc
// @Summary      List standard majors
// @Description  Returns CHSI standard majors. If catalog_year is omitted, the latest catalog year is used.
// @Tags         admission
// @Produce      json
// @Param        catalog_year query int false "Catalog year"
// @Param        q query string false "Search major code or name"
// @Success      200 {object} web.Response{data=[]StandardMajorResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admission/standard-majors [get]
func (h *MajorCatalogHandler) ListStandardMajors(c *gin.Context) {
	var catalogYear *int
	if raw := c.Query("catalog_year"); raw != "" {
		year, err := strconv.Atoi(raw)
		if err != nil {
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "catalog_year must be a number")
			return
		}
		catalogYear = &year
	}

	resp, err := h.service.ListStandardMajors(c.Request.Context(), StandardMajorFilter{
		CatalogYear: catalogYear,
		Query:       c.Query("q"),
	})
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to list standard majors")
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
