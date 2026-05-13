package payment

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"admission-api/internal/membership"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

type Handler struct {
	web.BaseHandler
	service                    Service
	validate                   *validator.Validate
	allowAnonymousMockCallback bool
	mockCallbackSecret         string
}

type HandlerOptions struct {
	AllowAnonymousMockCallback bool
	MockCallbackSecret         string
}

const MockCallbackSecretHeader = "X-Mock-Callback-Secret"

func NewHandler(service Service, opts HandlerOptions) *Handler {
	return &Handler{
		service:                    service,
		validate:                   validator.New(),
		allowAnonymousMockCallback: opts.AllowAnonymousMockCallback,
		mockCallbackSecret:         opts.MockCallbackSecret,
	}
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

// PayAlipay godoc
// @Summary      支付宝支付（电脑网站支付）
// @Tags         payment
// @Produce      json
// @Security     BearerAuth
// @Param        order_no path string true "订单号"
// @Success      200 {object} web.Response{data=AlipayPayResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      404 {object} web.Response
// @Router       /api/v1/payment/orders/{order_no}/pay/alipay [post]
func (h *Handler) PayAlipay(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.service.PayAlipay(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

// AlipayCallback godoc
// @Summary      支付宝异步通知回调
// @Tags         payment
// @Accept       x-www-form-urlencoded
// @Produce      text/plain
// @Param        body body string true "支付宝异步通知参数"
// @Success      200 {string} string "success"
// @Failure      400 {string} string "fail"
// @Router       /api/v1/payment/callbacks/alipay [post]
func (h *Handler) AlipayCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}

	params := make(map[string]string, len(values))
	for k := range values {
		params[k] = values.Get(k)
	}

	_, err = h.service.ProcessAlipayCallback(c.Request.Context(), params)
	if err != nil {
		slog.Error("alipay callback processing failed", "error", err)
		c.String(http.StatusOK, "fail")
		return
	}
	c.String(http.StatusOK, "success")
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
// @Param        X-Mock-Callback-Secret header string false "非开发环境下必填的内部 mock 回调密钥"
// @Param        body body MockCallbackRequest true "mock 回调"
// @Success      200 {object} web.Response{data=OrderResponse}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Router       /api/v1/payment/callbacks/mock [post]
func (h *Handler) MockCallback(c *gin.Context) {
	if !h.authorizeMockCallback(c) {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized mock callback")
		return
	}

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

func (h *Handler) authorizeMockCallback(c *gin.Context) bool {
	if h.allowAnonymousMockCallback {
		return true
	}
	if h.mockCallbackSecret == "" {
		return false
	}
	return c.GetHeader(MockCallbackSecretHeader) == h.mockCallbackSecret
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

func (h *Handler) RefundOrder(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	var req RefundOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.RefundOrder(c.Request.Context(), userID, c.Param("order_no"), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(resp))
}

func (h *Handler) ListRefunds(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}
	refunds, err := h.service.ListOrderRefunds(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(refunds))
}

func (h *Handler) AdminRefundOrder(c *gin.Context) {
	var req RefundOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}
	resp, err := h.service.AdminRefundOrder(c.Request.Context(), c.Param("order_no"), req)
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
	case errors.Is(err, ErrAlipayNotConfigured):
		h.RespondError(c, http.StatusServiceUnavailable, web.ErrCodeInternal, "alipay is not configured")
	case errors.Is(err, ErrAlipaySignature):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid alipay signature")
	case errors.Is(err, ErrChannelMismatch):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "payment channel mismatch")
	case errors.Is(err, ErrOrderNotRefundable):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "payment order is not refundable")
	case errors.Is(err, ErrRefundAmountExceeded):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "refund amount exceeds remaining order amount")
	case errors.Is(err, ErrRefundNotFound):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "refund not found")
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
