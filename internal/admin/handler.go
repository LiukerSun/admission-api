package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"admission-api/internal/platform/web"
)

// Handler handles admin HTTP requests.
type Handler struct {
	web.BaseHandler
	service  Service
	validate *validator.Validate
}

// NewHandler creates a new admin handler.
func NewHandler(service Service) *Handler {
	return &Handler{
		service:  service,
		validate: validator.New(),
	}
}

// GetUser godoc
// @Summary      管理员获取用户详情
// @Description  获取指定用户的完整信息，用于前端编辑表单回填
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int  true  "用户 ID"
// @Success      200   {object}  web.Response{data=UserResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/admin/users/{id} [get]
func (h *Handler) GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid user id")
		return
	}

	userResp, err := h.service.GetUser(c.Request.Context(), id)
	if err != nil {
		if err.Error() == "user not found" {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "user not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(userResp))
}

// ListUsers godoc
// @Summary      管理员获取用户列表
// @Description  分页获取所有用户信息，支持按 email、username、role、status 过滤
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page        query     int     false  "页码"       default(1)
// @Param        page_size   query     int     false  "每页数量"   default(20)
// @Param        email       query     string  false  "email 模糊搜索"
// @Param        username    query     string  false  "username 模糊搜索"
// @Param        role        query     string  false  "角色过滤 (user/premium/admin)"
// @Param        status      query     string  false  "状态过滤 (active/banned)"
// @Success      200         {object}  web.Response{data=UserListResponse}
// @Failure      401         {object}  web.Response
// @Failure      403         {object}  web.Response
// @Router       /api/v1/admin/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := ListUsersFilter{
		Email:    c.Query("email"),
		Username: c.Query("username"),
		Role:     c.Query("role"),
		Status:   c.Query("status"),
	}

	result, err := h.service.ListUsers(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// UpdateUser godoc
// @Summary      管理员修改用户信息
// @Description  修改指定用户的 email、username、role、user_type、status
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                true  "用户 ID"
// @Param        body  body      UpdateUserRequest  true  "用户信息"
// @Success      200   {object}  web.Response{data=UserResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/admin/users/{id} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid user id")
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	userResp, err := h.service.UpdateUser(c.Request.Context(), id, req)
	if err != nil {
		if err.Error() == "user not found" {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "user not found")
			return
		}
		if strings.Contains(err.Error(), "email or username already exists") {
			h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "email or username already exists")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(userResp))
}

// UpdateRole godoc
// @Summary      管理员修改用户角色
// @Description  修改指定用户的角色
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int               true  "用户 ID"
// @Param        body  body      UpdateRoleRequest  true  "角色信息"
// @Success      200   {object}  web.Response{data=map[string]string}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/admin/users/{id}/role [put]
func (h *Handler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid user id")
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	if err := h.service.UpdateRole(c.Request.Context(), id, req.Role); err != nil {
		if err.Error() == "user not found" {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "user not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "role updated"}))
}

// DisableUser godoc
// @Summary      管理员禁用用户
// @Description  禁用指定用户，清除其所有 refresh token
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "用户 ID"
// @Success      200  {object}  web.Response{data=map[string]string}
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/admin/users/{id}/disable [post]
func (h *Handler) DisableUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid user id")
		return
	}

	if err := h.service.DisableUser(c.Request.Context(), id); err != nil {
		if err.Error() == "user not found" {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "user not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "user disabled"}))
}

// EnableUser godoc
// @Summary      管理员启用用户
// @Description  解除对指定用户的禁用状态
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "用户 ID"
// @Success      200  {object}  web.Response{data=map[string]string}
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/admin/users/{id}/enable [post]
func (h *Handler) EnableUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid user id")
		return
	}

	if err := h.service.EnableUser(c.Request.Context(), id); err != nil {
		if err.Error() == "user not found" {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "user not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "user enabled"}))
}

// ListBindings godoc
// @Summary      管理员查看所有绑定关系
// @Description  分页获取所有家长-学生绑定关系
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int  false  "页码"      default(1)
// @Param        page_size  query     int  false  "每页数量"  default(20)
// @Success      200        {object}  web.Response{data=BindingListResponse}
// @Failure      401        {object}  web.Response
// @Failure      403        {object}  web.Response
// @Router       /api/v1/admin/bindings [get]
func (h *Handler) ListBindings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	result, err := h.service.ListBindings(c.Request.Context(), page, pageSize)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// GetStats godoc
// @Summary      管理员查看系统统计
// @Description  获取用户总数、绑定总数等运营数据
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  web.Response{data=StatsResponse}
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Router       /api/v1/admin/stats [get]
func (h *Handler) GetStats(c *gin.Context) {
	stats, err := h.service.GetStats(c.Request.Context())
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(stats))
}
