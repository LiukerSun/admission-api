package analysis

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/web"
)

// Handler 数据分析处理器
type Handler struct {
	web.BaseHandler
	service Service
}

// NewHandler 创建新的处理器实例
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

// GetEnrollmentPlans godoc
// @Summary      获取招生计划数据
// @Description  获取模拟的招生计划数据，支持分页和筛选
// @Tags         analysis
// @Accept       json
// @Produce      json
// @Param        school_name  query     string  false  "学校名称"
// @Param        major_name   query     string  false  "专业名称"
// @Param        province     query     string  false  "省份"
// @Param        year         query     int     false  "年份"
// @Param        batch        query     string  false  "批次"
// @Param        page         query     int     false  "页码，默认1"
// @Param        per_page     query     int     false  "每页数量，默认10"
// @Success      200          {object}  web.Response{data=EnrollmentPlanResponse}
// @Failure      400          {object}  web.Response
// @Failure      500          {object}  web.Response
// @Router       /api/v1/analysis/enrollment-plans [get]
func (h *Handler) GetEnrollmentPlans(c *gin.Context) {
	var query EnrollmentPlanQuery

	// 绑定查询参数
	if err := c.ShouldBindQuery(&query); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "无效的查询参数")
		return
	}

	// 调用服务获取数据
	response, err := h.service.GetEnrollmentPlans(c.Request.Context(), &query)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "获取数据失败")
		return
	}

	// 返回成功响应
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(response))
}

// GetEmploymentData godoc
// @Summary      获取就业情况数据
// @Description  获取模拟的专业就业情况数据，支持分页和筛选
// @Tags         analysis
// @Accept       json
// @Produce      json
// @Param        major_name   query     string  false  "专业名称"
// @Param        province     query     string  false  "省份"
// @Param        year         query     int     false  "年份"
// @Param        industry     query     string  false  "行业"
// @Param        page         query     int     false  "页码，默认1"
// @Param        per_page     query     int     false  "每页数量，默认10"
// @Success      200          {object}  web.Response{data=EmploymentDataResponse}
// @Failure      400          {object}  web.Response
// @Failure      500          {object}  web.Response
// @Router       /api/v1/analysis/employment-data [get]
func (h *Handler) GetEmploymentData(c *gin.Context) {
	var query EmploymentDataQuery

	// 绑定查询参数
	if err := c.ShouldBindQuery(&query); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "无效的查询参数")
		return
	}

	// 调用服务获取数据
	response, err := h.service.GetEmploymentData(c.Request.Context(), &query)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "获取数据失败")
		return
	}

	// 返回成功响应
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(response))
}
