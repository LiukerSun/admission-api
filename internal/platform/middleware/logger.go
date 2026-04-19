package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger(c *gin.Context) {
	start := time.Now()
	traceID, _ := generateRandomToken()

	c.Header("X-Trace-ID", traceID)
	c.Set("trace_id", traceID)

	c.Next()

	slog.Info("request",
		"trace_id", traceID,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"status", c.Writer.Status(),
		"duration", time.Since(start).String(),
		"ip", c.ClientIP(),
	)
}
