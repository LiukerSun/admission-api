package middleware

import "github.com/gin-gonic/gin"

func Platform(c *gin.Context) {
	platform := c.GetHeader("X-Platform")
	if platform == "" {
		platform = "web"
	}
	c.Set(ContextPlatformKey, platform)
	c.Next()
}
