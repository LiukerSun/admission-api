package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"admission-api/internal/platform/web"
)

type ActiveMembershipChecker interface {
	HasActiveMembership(ctx context.Context, userID int64) (bool, error)
}

// RequireActiveMembership blocks the request with a paywall response when the
// authenticated user does not have an active membership. The response uses
// web.ErrCodeMembershipRequired so the frontend can detect the paywall by
// response code alone and surface the upgrade modal.
func RequireActiveMembership(checker ActiveMembershipChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDRaw, exists := c.Get(ContextUserIDKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, web.Response{
				Code:    web.ErrCodeUnauthorized,
				Message: "unauthorized",
			})
			c.Abort()
			return
		}

		userID, ok := userIDRaw.(int64)
		if !ok || userID <= 0 {
			c.JSON(http.StatusUnauthorized, web.Response{
				Code:    web.ErrCodeUnauthorized,
				Message: "unauthorized",
			})
			c.Abort()
			return
		}

		active, err := checker.HasActiveMembership(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, web.Response{
				Code:    web.ErrCodeInternal,
				Message: "membership check failed",
			})
			c.Abort()
			return
		}
		if !active {
			c.JSON(http.StatusForbidden, web.Response{
				Code:    web.ErrCodeMembershipRequired,
				Message: "active membership required",
				Data: gin.H{
					"reason":           web.PaywallReasonMembershipRequired,
					"required_level":   "premium",
					"recommended_plan": "quarterly",
					"checkout_url":     "/membership",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
