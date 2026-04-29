package candidate

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// ProfileHandler exposes candidate-profile HTTP endpoints.
type ProfileHandler struct {
	web.BaseHandler
	service  ProfileService
	validate *validator.Validate
}

// NewProfileHandler constructs a profile handler.
func NewProfileHandler(service ProfileService) *ProfileHandler {
	return &ProfileHandler{
		service:  service,
		validate: validator.New(),
	}
}

// --- helpers ---

func (h *ProfileHandler) authContext(c *gin.Context) (int64, string, bool) {
	userIDRaw, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return 0, "", false
	}
	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return 0, "", false
	}
	userTypeRaw, _ := c.Get(middleware.ContextUserTypeKey)
	userType, _ := userTypeRaw.(string)
	return userID, userType, true
}

func (h *ProfileHandler) parseIDParam(c *gin.Context, name string) (int64, bool) {
	idStr := c.Param(name)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func (h *ProfileHandler) respondAppError(c *gin.Context, err error) {
	if appErr, ok := err.(*web.AppError); ok {
		status := http.StatusInternalServerError
		switch appErr.Code {
		case web.ErrCodeBadRequest:
			status = http.StatusBadRequest
		case web.ErrCodeUnauthorized:
			status = http.StatusUnauthorized
		case web.ErrCodeForbidden:
			status = http.StatusForbidden
		case web.ErrCodeNotFound:
			status = http.StatusNotFound
		case web.ErrCodeConflict:
			status = http.StatusConflict
		}
		h.RespondError(c, status, appErr.Code, appErr.Message)
		return
	}
	h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
}

// --- CRUD endpoints ---

// GetMyProfiles godoc
// @Summary      获取我的考生档案列表
// @Description  返回当前用户拥有的档案以及通过 user_bindings 关联到的对端拥有的档案
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  web.Response{data=ProfileListResponse}
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Router       /api/v1/candidate/profiles [get]
func (h *ProfileHandler) GetMyProfiles(c *gin.Context) {
	userID, userType, ok := h.authContext(c)
	if !ok {
		return
	}
	profiles, err := h.service.GetMyProfiles(c.Request.Context(), userID, userType)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(ProfileListResponse{
		Profiles: profiles,
		Total:    len(profiles),
	}))
}

// CreateProfile godoc
// @Summary      创建考生档案
// @Description  创建当前登录用户拥有的考生档案，身份证号 AES-GCM 加密入库
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateProfileRequest  true  "档案信息"
// @Success      200   {object}  web.Response{data=ProfileResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Router       /api/v1/candidate/profiles [post]
func (h *ProfileHandler) CreateProfile(c *gin.Context) {
	userID, _, ok := h.authContext(c)
	if !ok {
		return
	}
	var req CreateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	resp, err := h.service.CreateProfile(c.Request.Context(), userID, req)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GetProfile godoc
// @Summary      获取考生档案详情
// @Description  获取指定档案；调用者需为档案所有者或通过 user_bindings 绑定的对端
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "档案ID"
// @Success      200  {object}  web.Response{data=ProfileResponse}
// @Failure      400  {object}  web.Response
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/candidate/profiles/{id} [get]
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	userID, userType, ok := h.authContext(c)
	if !ok {
		return
	}
	id, ok := h.parseIDParam(c, "id")
	if !ok {
		return
	}
	resp, err := h.service.GetProfile(c.Request.Context(), userID, id, userType)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// UpdateProfile godoc
// @Summary      更新考生档案
// @Description  仅档案所有者可调用；动态字段更新；如修改身份证号会重新加密并更新哈希
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                   true   "档案ID"
// @Param        body  body      UpdateProfileRequest  true   "更新字段"
// @Success      200   {object}  web.Response{data=ProfileResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/candidate/profiles/{id} [put]
func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	userID, _, ok := h.authContext(c)
	if !ok {
		return
	}
	id, ok := h.parseIDParam(c, "id")
	if !ok {
		return
	}
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.UpdateProfile(c.Request.Context(), userID, id, req)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// DeleteProfile godoc
// @Summary      删除考生档案
// @Description  仅档案所有者可调用；执行软删除（is_deleted = true），不物理删除
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "档案ID"
// @Success      200  {object}  web.Response{data=map[string]string}
// @Failure      400  {object}  web.Response
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/candidate/profiles/{id} [delete]
func (h *ProfileHandler) DeleteProfile(c *gin.Context) {
	userID, _, ok := h.authContext(c)
	if !ok {
		return
	}
	id, ok := h.parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteProfile(c.Request.Context(), userID, id); err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "deleted"}))
}

// --- Lookup endpoints ---

// LookupByIDCard godoc
// @Summary      通过身份证号查找考生档案
// @Description  匹配候选档案并返回最小信息（含 owner_email），引导前端调用 /api/v1/bindings 完成账号绑定。仅 student/parent 可调用。
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      LookupByIDCardRequest  true  "身份证号"
// @Success      200   {object}  web.Response{data=LookupResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/candidate/profiles/lookup/idcard [post]
func (h *ProfileHandler) LookupByIDCard(c *gin.Context) {
	_, userType, ok := h.authContext(c)
	if !ok {
		return
	}
	var req LookupByIDCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.LookupByIDCard(c.Request.Context(), userType, req.IDCard)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// LookupByPhone godoc
// @Summary      通过手机号查找考生档案
// @Description  仅 student/parent 可调用；命中只回脱敏信息 + owner_email。
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      LookupByPhoneRequest  true  "手机号"
// @Success      200   {object}  web.Response{data=LookupResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/candidate/profiles/lookup/phone [post]
func (h *ProfileHandler) LookupByPhone(c *gin.Context) {
	_, userType, ok := h.authContext(c)
	if !ok {
		return
	}
	var req LookupByPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.LookupByPhone(c.Request.Context(), userType, req.Phone)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// LookupByCode godoc
// @Summary      通过 6 位绑定码查找考生档案（一次性）
// @Description  绑定码命中后立即销毁；仅 student/parent 可调用。
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      LookupByCodeRequest  true  "6 位数字绑定码"
// @Success      200   {object}  web.Response{data=LookupResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      403   {object}  web.Response
// @Failure      404   {object}  web.Response
// @Router       /api/v1/candidate/profiles/lookup/code [post]
func (h *ProfileHandler) LookupByCode(c *gin.Context) {
	_, userType, ok := h.authContext(c)
	if !ok {
		return
	}
	var req LookupByCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.LookupByCode(c.Request.Context(), userType, req.Code)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GenerateInviteCode godoc
// @Summary      为考生档案生成 6 位绑定码
// @Description  仅档案所有者可调用；同档案重复生成会覆盖旧码。
// @Tags         candidate-profile
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "档案ID"
// @Success      200  {object}  web.Response{data=InviteResponse}
// @Failure      400  {object}  web.Response
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/candidate/profiles/{id}/invite-code [post]
func (h *ProfileHandler) GenerateInviteCode(c *gin.Context) {
	userID, _, ok := h.authContext(c)
	if !ok {
		return
	}
	id, ok := h.parseIDParam(c, "id")
	if !ok {
		return
	}
	resp, err := h.service.GenerateInviteCode(c.Request.Context(), userID, id)
	if err != nil {
		h.respondAppError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}
