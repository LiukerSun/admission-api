package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockMerchantService struct {
	mock.Mock
}

func (m *mockMerchantService) CreateMerchant(ctx context.Context, req CreateMerchantRequest) (*PlannerMerchant, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerMerchant), args.Error(1)
}

func (m *mockMerchantService) GetMerchant(ctx context.Context, id int64) (*PlannerMerchant, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerMerchant), args.Error(1)
}

func (m *mockMerchantService) ListMerchants(ctx context.Context, status, merchantName, serviceRegion string, page, pageSize int) (*MerchantListResponse, error) {
	args := m.Called(ctx, status, merchantName, serviceRegion, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MerchantListResponse), args.Error(1)
}

func (m *mockMerchantService) UpdateMerchant(ctx context.Context, id int64, req UpdateMerchantRequest) (*PlannerMerchant, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PlannerMerchant), args.Error(1)
}

func setupMerchantTest() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestMerchantHandler_CreateMerchant(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	svc.On("CreateMerchant", mock.Anything, mock.Anything).Return(
		&PlannerMerchant{ID: 1, MerchantName: "Test Org", Status: "active"},
		nil,
	)

	c, w := setupMerchantTest()
	body := CreateMerchantRequest{MerchantName: "Test Org"}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/planner/merchants", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateMerchant(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestMerchantHandler_CreateMerchant_ValidationError(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	c, w := setupMerchantTest()
	body := CreateMerchantRequest{MerchantName: ""}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/planner/merchants", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateMerchant(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMerchantHandler_GetMerchant(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	svc.On("GetMerchant", mock.Anything, int64(7)).Return(
		&PlannerMerchant{ID: 7, MerchantName: "Org A", Status: "active"},
		nil,
	)

	c, w := setupMerchantTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/merchants/7", http.NoBody)

	h.GetMerchant(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestMerchantHandler_GetMerchant_InvalidID(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	c, w := setupMerchantTest()
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/merchants/invalid", http.NoBody)

	h.GetMerchant(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMerchantHandler_ListMerchants(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	svc.On("ListMerchants", mock.Anything, "active", "", "", 1, 20).Return(
		&MerchantListResponse{
			Merchants: []*PlannerMerchant{
				{ID: 1, MerchantName: "Org A", Status: "active"},
			},
			Total: 1,
		},
		nil,
	)

	c, w := setupMerchantTest()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/planner/merchants?status=active&page=1&page_size=20", http.NoBody)

	h.ListMerchants(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestMerchantHandler_UpdateMerchant(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	svc.On("UpdateMerchant", mock.Anything, int64(7), mock.Anything).Return(
		&PlannerMerchant{ID: 7, MerchantName: "Updated Org", Status: "active"},
		nil,
	)

	c, w := setupMerchantTest()
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	body := UpdateMerchantRequest{MerchantName: strPtr("Updated Org")}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/planner/merchants/7", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateMerchant(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp web.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Code)
}

func TestMerchantHandler_UpdateMerchant_InvalidID(t *testing.T) {
	svc := new(mockMerchantService)
	h := NewMerchantHandler(svc)

	c, w := setupMerchantTest()
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}
	body := UpdateMerchantRequest{MerchantName: strPtr("Updated")}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/planner/merchants/invalid", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateMerchant(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
