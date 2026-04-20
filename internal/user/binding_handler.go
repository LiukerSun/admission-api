package user

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

type CreateBindingRequest struct {
	StudentEmail string `json:"student_email" validate:"required,email" example:"xiaoming@example.com"`
}

type BindingResponse struct {
	ID        int64  `json:"id" example:"1"`
	ParentID  int64  `json:"parent_id" example:"2"`
	StudentID int64  `json:"student_id" example:"5"`
	CreatedAt string `json:"created_at" example:"2026-04-20T10:00:00Z"`
}

type BindingListResponse struct {
	UserType string                   `json:"user_type" example:"parent"`
	Bindings []*BindingWithUserDetail `json:"bindings"`
}

type BindingWithUserDetail struct {
	ID        int64    `json:"id"`
	User      SafeUser `json:"user"`
	CreatedAt string   `json:"created_at"`
}

type BindingHandler struct {
	web.BaseHandler
	bindingService BindingService
	validate       *validator.Validate
}

func NewBindingHandler(bindingService BindingService) *BindingHandler {
	return &BindingHandler{
		bindingService: bindingService,
		validate:       validator.New(),
	}
}

// CreateBinding godoc
// @Summary      发起家长-学生绑定
// @Description  家长通过学生邮箱发起绑定，即时生效
// @Tags         binding
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateBindingRequest  true  "绑定信息"
// @Success      200   {object}  web.Response{data=BindingResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/bindings [post]
func (h *BindingHandler) CreateBinding(c *gin.Context) {
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

	userTypeRaw, exists := c.Get(middleware.ContextUserTypeKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	userType, ok := userTypeRaw.(string)
	if !ok || userType != "parent" {
		h.RespondError(c, http.StatusForbidden, web.ErrCodeForbidden, "only parents can create bindings")
		return
	}

	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	binding, err := h.bindingService.BindStudent(c.Request.Context(), userID, req.StudentEmail)
	if err != nil {
		switch err.Error() {
		case "student not found":
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "student not found")
		case "cannot bind yourself":
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "cannot bind yourself")
		case "student already bound to another parent":
			h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "student already bound to another parent")
		default:
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		}
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(BindingResponse{
		ID:        binding.ID,
		ParentID:  binding.ParentID,
		StudentID: binding.StudentID,
		CreatedAt: binding.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}))
}

// GetMyBindings godoc
// @Summary      查询我的绑定关系
// @Description  家长返回绑定的学生列表，学生返回绑定的家长信息
// @Tags         binding
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  web.Response{data=BindingListResponse}
// @Failure      401  {object}  web.Response
// @Router       /api/v1/bindings [get]
func (h *BindingHandler) GetMyBindings(c *gin.Context) {
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

	userTypeRaw, exists := c.Get(middleware.ContextUserTypeKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	userType, ok := userTypeRaw.(string)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	result, err := h.bindingService.GetMyBindings(c.Request.Context(), userID, userType)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	bindings := make([]*BindingWithUserDetail, 0, len(result.Bindings))
	for _, b := range result.Bindings {
		bindings = append(bindings, &BindingWithUserDetail{
			ID: b.ID,
			User: SafeUser{
				ID:    b.User.ID,
				Email: b.User.Email,
			},
			CreatedAt: b.CreatedAt,
		})
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(BindingListResponse{
		UserType: result.UserType,
		Bindings: bindings,
	}))
}

// DeleteBinding godoc
// @Summary      管理员解除绑定
// @Description  仅管理员可调用，解除家长-学生绑定关系
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Binding ID"
// @Success      200  {object}  web.Response{data=map[string]string}
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/admin/bindings/{id} [delete]
func (h *BindingHandler) DeleteBinding(c *gin.Context) {
	roleRaw, exists := c.Get(middleware.ContextRoleKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	role, ok := roleRaw.(string)
	if !ok || role != "admin" {
		h.RespondError(c, http.StatusForbidden, web.ErrCodeForbidden, "admin access required")
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid binding id")
		return
	}

	if err := h.bindingService.RemoveBinding(c.Request.Context(), id); err != nil {
		if err.Error() == "binding not found" {
			h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "binding not found")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "binding removed"}))
}
