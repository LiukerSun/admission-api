package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"admission-api/internal/platform/web"
)

type mockMembershipChecker struct {
	active bool
	err    error
}

func (m mockMembershipChecker) HasActiveMembership(ctx context.Context, userID int64) (bool, error) {
	return m.active, m.err
}

func TestRequireActiveMembershipAllowsActiveMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, int64(1))
		c.Next()
	})
	r.GET("/premium", RequireActiveMembership(mockMembershipChecker{active: true}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/premium", http.NoBody)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestRequireActiveMembershipRejectsExpiredMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, int64(1))
		c.Next()
	})
	r.GET("/premium", RequireActiveMembership(mockMembershipChecker{active: false}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/premium", http.NoBody)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "active membership required")

	var body struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, web.ErrCodeMembershipRequired, body.Code)
	assert.Equal(t, web.PaywallReasonMembershipRequired, body.Data["reason"])
	assert.Equal(t, "premium", body.Data["required_level"])
	assert.Equal(t, "quarterly", body.Data["recommended_plan"])
}
