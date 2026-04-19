package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recover(c *gin.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("panic recovered",
				"error", rec,
				"stack", string(debug.Stack()),
				"path", c.Request.URL.Path,
			)
			c.JSON(500, gin.H{"code": 5000, "message": "internal server error"})
			c.Abort()
		}
	}()
	c.Next()
}
