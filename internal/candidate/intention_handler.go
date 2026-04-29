package candidate

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// IntentionHandler handles intention HTTP requests.
type IntentionHandler struct {
	web.BaseHandler
	service  IntentionService
	validate *validator.Validate
}

// NewIntentionHandler creates a new intention handler.
func NewIntentionHandler(service IntentionService) *IntentionHandler {
	return &IntentionHandler{
		service:  service,
		validate: validator.New(),
	}
}

// GetIntentions godoc
// @Summary      获取考生意向列表
// @Description  按档案ID查询全部意向，按类型分组返回
// @Tags         candidate-intention
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        profile_id  path      int  true  "考生档案ID"
// @Success      200         {object}  web.Response{data=IntentionGroupResponse}
// @Failure      401         {object}  web.Response
// @Failure      403         {object}  web.Response
// @Router       /api/v1/candidate/intentions/{profile_id} [get]
func (h *IntentionHandler) GetIntentions(c *gin.Context) {
	profileID, err := strconv.ParseInt(c.Param("profile_id"), 10, 64)
	if err != nil || profileID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid profile_id")
		return
	}

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

	result, err := h.service.GetIntentions(c.Request.Context(), userID, profileID)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusBadRequest
			if appErr.Code == web.ErrCodeForbidden {
				status = http.StatusForbidden
			} else if appErr.Code == web.ErrCodeNotFound {
				status = http.StatusNotFound
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// SaveIntentions godoc
// @Summary      批量保存意向
// @Description  全量覆盖某类型下的全部意向（删除旧记录，批量插入新记录）
// @Tags         candidate-intention
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        profile_id   path      int                   true  "考生档案ID"
// @Param        type         path      string                true  "意向类型 (province/school/major/school_major)"
// @Param        body         body      SaveIntentionsRequest true  "意向列表"
// @Success      200          {object}  web.Response
// @Failure      400          {object}  web.Response
// @Failure      401          {object}  web.Response
// @Failure      403          {object}  web.Response
// @Router       /api/v1/candidate/intentions/{profile_id}/{type} [put]
func (h *IntentionHandler) SaveIntentions(c *gin.Context) {
	profileID, err := strconv.ParseInt(c.Param("profile_id"), 10, 64)
	if err != nil || profileID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid profile_id")
		return
	}
	intentionType := c.Param("type")

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

	var req SaveIntentionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	if err := h.service.SaveIntentions(c.Request.Context(), userID, profileID, intentionType, &req); err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusBadRequest
			if appErr.Code == web.ErrCodeForbidden {
				status = http.StatusForbidden
			} else if appErr.Code == web.ErrCodeNotFound {
				status = http.StatusNotFound
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}

// RemoveIntention godoc
// @Summary      删除单条意向
// @Description  删除指定ID的单条意向记录
// @Tags         candidate-intention
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      int  true  "意向ID"
// @Success      200 {object}  web.Response
// @Failure      401 {object}  web.Response
// @Failure      403 {object}  web.Response
// @Failure      404 {object}  web.Response
// @Router       /api/v1/candidate/intentions/{id} [delete]
func (h *IntentionHandler) RemoveIntention(c *gin.Context) {
	intentionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || intentionID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid intention id")
		return
	}

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

	if err := h.service.RemoveIntention(c.Request.Context(), userID, intentionID); err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusBadRequest
			if appErr.Code == web.ErrCodeForbidden {
				status = http.StatusForbidden
			} else if appErr.Code == web.ErrCodeNotFound {
				status = http.StatusNotFound
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}

// ClearIntentions godoc
// @Summary      清空某类型意向
// @Description  删除某档案下指定类型的全部意向
// @Tags         candidate-intention
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        profile_id  path      int     true  "考生档案ID"
// @Param        type        path      string  true  "意向类型 (province/school/major/school_major)"
// @Success      200         {object}  web.Response
// @Failure      400         {object}  web.Response
// @Failure      401         {object}  web.Response
// @Failure      403         {object}  web.Response
// @Router       /api/v1/candidate/intentions/by_profile_id/{profile_id}/{type} [delete]
func (h *IntentionHandler) ClearIntentions(c *gin.Context) {
	profileID, err := strconv.ParseInt(c.Param("profile_id"), 10, 64)
	if err != nil || profileID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid profile_id")
		return
	}
	intentionType := c.Param("type")

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

	if err := h.service.ClearIntentions(c.Request.Context(), userID, profileID, intentionType); err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			status := http.StatusBadRequest
			if appErr.Code == web.ErrCodeForbidden {
				status = http.StatusForbidden
			} else if appErr.Code == web.ErrCodeNotFound {
				status = http.StatusNotFound
			}
			h.RespondError(c, status, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(nil))
}
