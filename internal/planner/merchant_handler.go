package planner

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// MerchantHandler handles merchant HTTP requests.
type MerchantHandler struct {
	web.BaseHandler
	service  MerchantService
	validate *validator.Validate
}

// NewMerchantHandler creates a new merchant handler.
func NewMerchantHandler(service MerchantService) *MerchantHandler {
	return &MerchantHandler{
		service:  service,
		validate: validator.New(),
	}
}

// CreateMerchant godoc
// @Summary      创建规划师机构
// @Description  管理员创建新的规划师机构
// @Tags         planner-merchant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateMerchantRequest  true  "机构信息"
// @Success      200   {object}  web.Response{data=PlannerMerchant}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/admin/planner/merchants [post]
func (h *MerchantHandler) CreateMerchant(c *gin.Context) {
	var req CreateMerchantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	m, err := h.service.CreateMerchant(c.Request.Context(), req)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusInternalServerError
			switch appErr.Code {
			case web.ErrCodeConflict:
				status = http.StatusConflict
			case web.ErrCodeBadRequest:
				status = http.StatusBadRequest
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(m))
}

// GetMerchant godoc
// @Summary      获取机构详情
// @Description  获取指定规划师机构的详情信息
// @Tags         planner-merchant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int  true  "机构ID"
// @Success      200   {object}  web.Response{data=PlannerMerchant}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/planner/merchants/{id} [get]
func (h *MerchantHandler) GetMerchant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid merchant id")
		return
	}

	m, err := h.service.GetMerchant(c.Request.Context(), id)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusInternalServerError
			if appErr.Code == web.ErrCodeNotFound {
				status = http.StatusNotFound
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(m))
}

// ListMerchants godoc
// @Summary      查询机构列表
// @Description  分页查询规划师机构列表，支持按状态筛选
// @Tags         planner-merchant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page            query     int     false  "页码"                  default(1)
// @Param        page_size       query     int     false  "每页数量"              default(20)
// @Param        status          query     string  false  "状态过滤 (active/inactive)"
// @Param        merchant_name   query     string  false  "机构名称模糊查询"
// @Param        service_region  query     string  false  "服务区域省份编码"
// @Success      200             {object}  web.Response{data=MerchantListResponse}
// @Failure      401             {object}  web.Response
// @Router       /api/v1/planner/merchants [get]
func (h *MerchantHandler) ListMerchants(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	merchantName := c.Query("merchant_name")
	serviceRegion := c.Query("service_region")

	result, err := h.service.ListMerchants(c.Request.Context(), status, merchantName, serviceRegion, page, pageSize)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// UpdateMerchant godoc
// @Summary      更新规划师机构
// @Description  管理员更新机构信息（动态字段更新）
// @Tags         planner-merchant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                   true  "机构ID"
// @Param        body  body      UpdateMerchantRequest true  "机构信息"
// @Success      200   {object}  web.Response{data=PlannerMerchant}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/admin/planner/merchants/{id} [put]
func (h *MerchantHandler) UpdateMerchant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid merchant id")
		return
	}

	var req UpdateMerchantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	m, err := h.service.UpdateMerchant(c.Request.Context(), id, req)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusInternalServerError
			switch appErr.Code {
			case web.ErrCodeNotFound:
				status = http.StatusNotFound
			case web.ErrCodeConflict:
				status = http.StatusConflict
			case web.ErrCodeBadRequest:
				status = http.StatusBadRequest
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(m))
}
