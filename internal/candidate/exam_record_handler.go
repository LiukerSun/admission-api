package candidate

import (
	"net/http"
	"strconv"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// ExamRecordHandler handles exam record HTTP requests.
type ExamRecordHandler struct {
	web.BaseHandler
	service  ExamRecordService
	validate *validator.Validate
}

// NewExamRecordHandler creates a new exam record handler.
func NewExamRecordHandler(service ExamRecordService) *ExamRecordHandler {
	return &ExamRecordHandler{
		service:  service,
		validate: validator.New(),
	}
}

// ListByProfile godoc
// @Summary      列出考试记录
// @Description  按档案ID查询全部考试记录，按当前有效优先、年份倒序返回
// @Tags         candidate-exam-record
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        profile_id  path      int  true  "考生档案ID"
// @Success      200         {object}  web.Response{data=[]ExamRecordResponse}
// @Failure      401         {object}  web.Response
// @Failure      403         {object}  web.Response
// @Router       /api/v1/candidate/exam-records/by_profile_id/{profile_id} [get]
func (h *ExamRecordHandler) ListByProfile(c *gin.Context) {
	profileID, err := strconv.ParseInt(c.Param("profile_id"), 10, 64)
	if err != nil || profileID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid profile_id")
		return
	}

	userID := h.mustUserID(c)
	if userID == 0 {
		return
	}

	result, err := h.service.ListByProfile(c.Request.Context(), userID, profileID)
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

// Create godoc
// @Summary      创建考试记录
// @Description  录入一次新的考试成绩，自动将该档案下其他记录设为非当前
// @Tags         candidate-exam-record
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        profile_id  path      int                      true  "考生档案ID"
// @Param        body        body      CreateExamRecordRequest  true  "考试记录"
// @Success      200         {object}  web.Response{data=ExamRecordResponse}
// @Failure      400         {object}  web.Response
// @Failure      401         {object}  web.Response
// @Failure      403         {object}  web.Response
// @Router       /api/v1/candidate/exam-records/by_profile_id/{profile_id} [post]
func (h *ExamRecordHandler) Create(c *gin.Context) {
	profileID, err := strconv.ParseInt(c.Param("profile_id"), 10, 64)
	if err != nil || profileID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid profile_id")
		return
	}

	userID := h.mustUserID(c)
	if userID == 0 {
		return
	}

	var req CreateExamRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	result, err := h.service.Create(c.Request.Context(), userID, profileID, req)
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

// GetByID godoc
// @Summary      获取考试记录详情
// @Description  按记录ID查询单条考试记录
// @Tags         candidate-exam-record
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      int  true  "考试记录ID"
// @Success      200 {object}  web.Response{data=ExamRecordResponse}
// @Failure      401 {object}  web.Response
// @Failure      403 {object}  web.Response
// @Failure      404 {object}  web.Response
// @Router       /api/v1/candidate/exam-records/{id} [get]
func (h *ExamRecordHandler) GetByID(c *gin.Context) {
	recordID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || recordID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid id")
		return
	}

	userID := h.mustUserID(c)
	if userID == 0 {
		return
	}

	result, err := h.service.GetByID(c.Request.Context(), userID, recordID)
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

// Update godoc
// @Summary      更新考试记录
// @Description  修改考试记录的基础信息或成绩数据；若成绩字段发生变化，自动记录变更历史
// @Tags         candidate-exam-record
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int                      true  "考试记录ID"
// @Param        body body      UpdateExamRecordRequest  true  "更新内容"
// @Success      200  {object}  web.Response{data=ExamRecordResponse}
// @Failure      400  {object}  web.Response
// @Failure      401  {object}  web.Response
// @Failure      403  {object}  web.Response
// @Failure      404  {object}  web.Response
// @Router       /api/v1/candidate/exam-records/{id} [put]
func (h *ExamRecordHandler) Update(c *gin.Context) {
	recordID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || recordID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid id")
		return
	}

	userID := h.mustUserID(c)
	if userID == 0 {
		return
	}

	var req UpdateExamRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	result, err := h.service.Update(c.Request.Context(), userID, recordID, req)
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

// Void godoc
// @Summary      作废考试记录
// @Description  将指定考试记录标记为作废（void），非物理删除
// @Tags         candidate-exam-record
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      int  true  "考试记录ID"
// @Success      200 {object}  web.Response
// @Failure      401 {object}  web.Response
// @Failure      403 {object}  web.Response
// @Failure      404 {object}  web.Response
// @Router       /api/v1/candidate/exam-records/{id} [delete]
func (h *ExamRecordHandler) Void(c *gin.Context) {
	recordID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || recordID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid id")
		return
	}

	userID := h.mustUserID(c)
	if userID == 0 {
		return
	}

	if err := h.service.Void(c.Request.Context(), userID, recordID); err != nil {
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

// ListScoreHistories godoc
// @Summary      列出成绩修改历史
// @Description  查询某考试记录的所有成绩变更快照
// @Tags         candidate-exam-record
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      int  true  "考试记录ID"
// @Success      200 {object}  web.Response{data=[]ScoreHistoryResponse}
// @Failure      401 {object}  web.Response
// @Failure      403 {object}  web.Response
// @Failure      404 {object}  web.Response
// @Router       /api/v1/candidate/exam-records/{id}/score-histories [get]
func (h *ExamRecordHandler) ListScoreHistories(c *gin.Context) {
	recordID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || recordID <= 0 {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid id")
		return
	}

	userID := h.mustUserID(c)
	if userID == 0 {
		return
	}

	result, err := h.service.ListScoreHistories(c.Request.Context(), userID, recordID)
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

func (h *ExamRecordHandler) mustUserID(c *gin.Context) int64 {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return 0
	}
	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return 0
	}
	return userID
}
