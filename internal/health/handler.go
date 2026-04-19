package health

import (
	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/db"
	"admission-api/internal/platform/web"
)

// Handler provides the health check endpoint.
type Handler struct {
	web.BaseHandler
	db *db.DB
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{db: database}
}

// Check godoc
// @Summary      健康检查
// @Description  检查服务及数据库连接状态
// @Tags         health
// @Accept       json
// @Produce      json
// @Success      200  {object}  web.Response{data=map[string]string}
// @Failure      503  {object}  web.Response
// @Router       /health [get]
func (h *Handler) Check(c *gin.Context) {
	if err := h.db.HealthCheck(c.Request.Context()); err != nil {
		h.RespondError(c, 503, 5000, "database unavailable")
		return
	}

	h.RespondJSON(c, 200, web.SuccessResponse(map[string]string{
		"status": "healthy",
	}))
}
