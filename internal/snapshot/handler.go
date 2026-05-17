package snapshot

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

// Handler 暴露 GET /api/v1/me/profile/snapshot。
// 前端 / AI agent 用这一个 endpoint 拿到完整 recommendation 输入，不再自己拼。
type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// GetMySnapshot godoc
// @Summary      获取当前用户的推荐 snapshot
// @Description  把 user_profiles 表的问卷答案 + lookup 服务现查的位次/志愿数，合并为一份推荐算法可直接消费的 snapshot。
// @Tags         user-profile
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      422 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/me/profile/snapshot [get]
func (h *Handler) GetMySnapshot(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	snap, err := h.service.BuildRecommendationSnapshot(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrProfileIncomplete):
			h.RespondError(c, http.StatusUnprocessableEntity, web.ErrCodeBadRequest,
				"问卷尚未填完：省份/选科/再选科目/总分 四项必须齐全")
			return
		case errors.Is(err, ErrRankDataMissing):
			h.RespondError(c, http.StatusUnprocessableEntity, web.ErrCodeBadRequest,
				"今年和上一年的一分一段表均未入库，无法换算位次，请稍后")
			return
		default:
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal,
				"failed to build snapshot")
			return
		}
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(snap))
}
