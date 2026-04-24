package payment

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"admission-api/internal/membership"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

type Handler struct {
	web.BaseHandler
	service  Service
	validate *validator.Validate
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service, validate: validator.New()}
}

// CreateOrder godoc
// @Summary      创建会员支付订单
// @Tags         payment
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateOrderRequest true "订单信息"
// @Success      200 {object} web.Response{data=OrderResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      409 {object} web.Response
// @Router       /api/v1/payment/orders [post]
func (h *Handler) CreateOrder(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.CreateOrder(c.Request.Context(), userID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// ListMyOrders godoc
// @Summary      查询我的支付订单
// @Tags         payment
// @Produce      json
// @Security     BearerAuth
// @Param        page      query int false "页码"
// @Param        page_size query int false "每页数量"
// @Success      200 {object} web.Response{data=OrderListResponse}
// @Failure      401 {object} web.Response
// @Router       /api/v1/payment/orders [get]
func (h *Handler) ListMyOrders(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	resp, err := h.service.ListMyOrders(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// GetMyOrder godoc
// @Summary      查询我的支付订单详情
// @Tags         payment
// @Produce      json
// @Security     BearerAuth
// @Param        order_no path string true "订单号"
// @Success      200 {object} web.Response{data=OrderResponse}
// @Failure      401 {object} web.Response
// @Failure      404 {object} web.Response
// @Router       /api/v1/payment/orders/{order_no} [get]
func (h *Handler) GetMyOrder(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.service.GetMyOrder(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// PayMock godoc
// @Summary      mock 支付会员订单
// @Tags         payment
// @Produce      json
// @Security     BearerAuth
// @Param        order_no path string true "订单号"
// @Success      200 {object} web.Response{data=OrderResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Router       /api/v1/payment/orders/{order_no}/pay [post]
func (h *Handler) PayMock(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.service.PayMock(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// Detect godoc
// @Summary      主动检测订单支付状态
// @Tags         payment
// @Produce      json
// @Security     BearerAuth
// @Param        order_no path string true "订单号"
// @Success      200 {object} web.Response{data=OrderResponse}
// @Failure      401 {object} web.Response
// @Failure      404 {object} web.Response
// @Router       /api/v1/payment/orders/{order_no}/detect [post]
func (h *Handler) Detect(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.service.Detect(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// MockCallback godoc
// @Summary      mock 支付回调
// @Tags         payment
// @Accept       json
// @Produce      json
// @Param        body body MockCallbackRequest true "mock 回调"
// @Success      200 {object} web.Response{data=OrderResponse}
// @Failure      400 {object} web.Response
// @Router       /api/v1/payment/callbacks/mock [post]
func (h *Handler) MockCallback(c *gin.Context) {
	var req MockCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.ProcessMockCallback(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) AdminListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseInt(c.Query("user_id"), 10, 64)
	resp, err := h.service.ListAdminOrders(c.Request.Context(), AdminOrderFilter{
		OrderNo:     c.Query("order_no"),
		UserID:      userID,
		PlanCode:    c.Query("plan_code"),
		Channel:     c.Query("channel"),
		OrderStatus: c.Query("order_status"),
	}, page, pageSize)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) AdminGetOrder(c *gin.Context) {
	resp, err := h.service.GetAdminOrder(c.Request.Context(), c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) AdminCloseOrder(c *gin.Context) {
	resp, err := h.service.CloseAdminOrder(c.Request.Context(), c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) AdminRedetect(c *gin.Context) {
	resp, err := h.service.RedetectAdmin(c.Request.Context(), c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) AdminRegrantMembership(c *gin.Context) {
	resp, err := h.service.RegrantMembership(c.Request.Context(), c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case membership.WritePlanError(&h.BaseHandler, c, err):
		return
	case errors.Is(err, ErrOrderNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "payment order not found")
	case errors.Is(err, ErrOrderAccessDenied):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "payment order not found")
	case errors.Is(err, ErrOrderNotPayable):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "payment order is not payable")
	case errors.Is(err, ErrOrderExpired):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "payment order expired")
	case errors.Is(err, ErrIdempotencyConflict):
		h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "idempotency key conflict")
	default:
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
	}
}

func userIDFromContext(c *gin.Context) (int64, bool) {
	raw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := raw.(int64)
	return userID, ok && userID > 0
}
