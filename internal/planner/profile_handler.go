package planner

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// ProfileHandler handles planner profile HTTP requests.
type ProfileHandler struct {
	web.BaseHandler
	service  ProfileService
	validate *validator.Validate
}

// NewProfileHandler creates a new profile handler.
func NewProfileHandler(service ProfileService) *ProfileHandler {
	return &ProfileHandler{
		service:  service,
		validate: validator.New(),
	}
}

// CreateProfile godoc
// @Summary      创建规划师档案
// @Description  管理员创建规划师档案并同时创建用户账号
// @Tags         planner-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateProfileRequest  true  "档案信息"
// @Success      200   {object}  web.Response{data=PlannerProfileResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/admin/planner/profiles [post]
func (h *ProfileHandler) CreateProfile(c *gin.Context) {
	var req CreateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	p, err := h.service.CreateProfile(c.Request.Context(), req)
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

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(p))
}

// GetMyProfile godoc
// @Summary      获取我的档案
// @Description  当前登录规划师获取自己的档案
// @Tags         planner-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  web.Response{data=PlannerProfileResponse}
// @Failure      401  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/planner/profiles/me [get]
func (h *ProfileHandler) GetMyProfile(c *gin.Context) {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	p, err := h.service.GetMyProfile(c.Request.Context(), userID)
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

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(p))
}

// UpdateMyProfile godoc
// @Summary      更新我的档案
// @Description  当前登录规划师更新自己的档案信息（动态字段更新）
// @Tags         planner-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      UpdateMyProfileRequest  true  "档案信息"
// @Success      200   {object}  web.Response{data=PlannerProfileResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/planner/profiles/me [put]
func (h *ProfileHandler) UpdateMyProfile(c *gin.Context) {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req UpdateMyProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	p, err := h.service.UpdateMyProfile(c.Request.Context(), userID, req)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusInternalServerError
			switch appErr.Code {
			case web.ErrCodeNotFound:
				status = http.StatusNotFound
			case web.ErrCodeBadRequest:
				status = http.StatusBadRequest
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(p))
}

// GetProfile godoc
// @Summary      获取档案详情
// @Description  获取指定规划师档案的详情信息
// @Tags         planner-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int  true  "档案ID"
// @Success      200   {object}  web.Response{data=PlannerProfileResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/planner/profiles/{id} [get]
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid profile id")
		return
	}

	p, err := h.service.GetProfile(c.Request.Context(), id)
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

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(p))
}

// ListProfiles godoc
// @Summary      查询规划师档案列表
// @Description  分页查询规划师档案列表，支持按等级、状态、机构筛选
// @Tags         planner-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page        query     int     false  "页码"                  default(1)
// @Param        page_size   query     int     false  "每页数量"              default(20)
// @Param        level       query     string  false  "等级筛选 (junior/intermediate/senior/expert)"
// @Param        status      query     string  false  "状态筛选 (active/inactive/retired/pending)"
// @Param        merchant_id query     int     false  "机构ID筛选"
// @Param        real_name   query     string  false  "真实姓名模糊查询"
// @Param        phone       query     string  false  "电话模糊查询"
// @Param        sort_field  query     string  false  "排序字段 (created_at/rating_avg/total_service_count)"
// @Param        sort_order  query     string  false  "排序方向 (asc/desc)"
// @Success      200         {object}  web.Response{data=ProfileListResponse}
// @Failure      401         {object}  web.Response
// @Router       /api/v1/planner/profiles [get]
func (h *ProfileHandler) ListProfiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	level := c.Query("level")
	status := c.Query("status")
	merchantIDStr := c.Query("merchant_id")
	realName := c.Query("real_name")
	phone := c.Query("phone")
	sortField := c.Query("sort_field")
	sortOrder := c.Query("sort_order")

	var merchantID *int64
	if merchantIDStr != "" {
		if id, err := strconv.ParseInt(merchantIDStr, 10, 64); err == nil && id > 0 {
			merchantID = &id
		}
	}

	filter := ProfileFilter{
		Level:      level,
		Status:     status,
		MerchantID: merchantID,
		RealName:   realName,
		Phone:      phone,
	}

	result, err := h.service.ListProfiles(c.Request.Context(), filter, page, pageSize, sortField, sortOrder)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}
