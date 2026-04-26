package candidate

import (
	"net/http"
	"strconv"
	"time"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// ActivityLogHandler handles activity log HTTP requests.
type ActivityLogHandler struct {
	web.BaseHandler
	service ActivityLogService
}

// NewActivityLogHandler creates a new activity log handler.
func NewActivityLogHandler(service ActivityLogService) *ActivityLogHandler {
	return &ActivityLogHandler{service: service}
}

// ListActivities godoc
// @Summary      查询活动日志列表
// @Description  管理员分页查询活动日志，支持按用户、活动类型、目标类型、时间范围筛选
// @Tags         candidate-activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page          query     int     false  "页码"                  default(1)
// @Param        page_size     query     int     false  "每页数量"              default(20)
// @Param        user_id       query     int     false  "用户ID筛选"
// @Param        activity_type query     string  false  "活动类型筛选"
// @Param        target_type   query     string  false  "目标类型筛选"
// @Param        start_time    query     string  false  "开始时间 (RFC3339)"
// @Param        end_time      query     string  false  "结束时间 (RFC3339)"
// @Success      200           {object}  web.Response{data=ActivityLogListResponse}
// @Failure      401           {object}  web.Response
// @Router       /api/v1/admin/candidate/activities [get]
func (h *ActivityLogHandler) ListActivities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userIDStr := c.Query("user_id")
	activityType := c.Query("activity_type")
	targetType := c.Query("target_type")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	var userID int64
	if userIDStr != "" {
		userID, _ = strconv.ParseInt(userIDStr, 10, 64)
	}

	var startTime, endTime *time.Time
	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = &t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = &t
		}
	}

	filter := ActivityFilter{
		UserID:       userID,
		ActivityType: activityType,
		TargetType:   targetType,
		StartTime:    startTime,
		EndTime:      endTime,
	}

	result, err := h.service.ListActivities(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// GetMyActivities godoc
// @Summary      获取我的活动记录
// @Description  当前登录用户获取自己的活动日志列表
// @Tags         candidate-activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int  false  "页码"     default(1)
// @Param        page_size  query     int  false  "每页数量" default(20)
// @Success      200        {object}  web.Response{data=ActivityLogListResponse}
// @Failure      401        {object}  web.Response
// @Router       /api/v1/me/activities [get]
func (h *ActivityLogHandler) GetMyActivities(c *gin.Context) {
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

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	result, err := h.service.GetMyActivities(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// GetStats godoc
// @Summary      获取活动统计
// @Description  获取指定目标对象被访问的次数统计
// @Tags         candidate-activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        target_type  query     string  true  "目标类型"
// @Param        target_id    query     int     true  "目标ID"
// @Success      200          {object}  web.Response{data=ActivityStatsResponse}
// @Failure      400          {object}  web.Response
// @Failure      401          {object}  web.Response
// @Router       /api/v1/admin/candidate/activities/stats [get]
func (h *ActivityLogHandler) GetStats(c *gin.Context) {
	targetType := c.Query("target_type")
	targetIDStr := c.Query("target_id")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 || targetType == "" {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid target_type or target_id")
		return
	}

	result, err := h.service.GetStats(c.Request.Context(), targetType, targetID)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			h.RespondError(c, http.StatusBadRequest, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(result))
}

// DeleteByIDs godoc
// @Summary      批量删除活动日志
// @Description  管理员按ID列表批量删除活动日志
// @Tags         candidate-activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      object{ids=[]int64}  true  "ID列表"
// @Success      200   {object}  web.Response{data=map[string]int64}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Router       /api/v1/admin/candidate/activities [delete]
func (h *ActivityLogHandler) DeleteByIDs(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	deleted, err := h.service.DeleteByIDs(c.Request.Context(), req.IDs)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			h.RespondError(c, http.StatusBadRequest, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(map[string]int64{"deleted": deleted}))
}

// DeleteBefore godoc
// @Summary      按时间删除活动日志
// @Description  管理员删除指定时间之前的所有活动日志
// @Tags         candidate-activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      object{before=string}  true  "删除此时间之前的日志 (RFC3339)"
// @Success      200   {object}  web.Response{data=map[string]int64}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Router       /api/v1/admin/candidate/activities/before [delete]
func (h *ActivityLogHandler) DeleteBefore(c *gin.Context) {
	var req struct {
		Before string `json:"before" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	before, err := time.Parse(time.RFC3339, req.Before)
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid before time format, expected RFC3339")
		return
	}

	deleted, err := h.service.DeleteBefore(c.Request.Context(), before)
	if err != nil {
		if appErr, ok := err.(*web.AppError); ok {
			h.RespondError(c, http.StatusBadRequest, appErr.Code, appErr.Message)
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(map[string]int64{"deleted": deleted}))
}
