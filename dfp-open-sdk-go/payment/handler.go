package payment

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	service  Service
	validate *validator.Validate
}

func NewHandler(service Service) *Handler {
	return &Handler{
		service:  service,
		validate: validator.New(),
	}
}

// CreateOrder handles POST /openbank/orders
func (h *Handler) CreateOrder(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "invalid request body"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": err.Error()})
		return
	}
	resp, err := h.service.CreateOrder(c.Request.Context(), userID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// Pay handles POST /openbank/orders/:order_no/pay
func (h *Handler) Pay(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	orderNo := c.Param("order_no")
	clientIP := c.ClientIP()
	resp, err := h.service.Pay(c.Request.Context(), userID, orderNo, clientIP)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// GetOrder handles GET /openbank/orders/:order_no
func (h *Handler) GetOrder(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	resp, err := h.service.GetOrder(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// Detect handles POST /openbank/orders/:order_no/detect
func (h *Handler) Detect(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	resp, err := h.service.Detect(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// RefundOrder handles POST /openbank/orders/:order_no/refund
func (h *Handler) RefundOrder(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	var req RefundOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "invalid request body"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": err.Error()})
		return
	}
	resp, err := h.service.RefundOrder(c.Request.Context(), userID, c.Param("order_no"), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// ListRefunds handles GET /openbank/orders/:order_no/refunds
func (h *Handler) ListRefunds(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	refunds, err := h.service.ListOrderRefunds(c.Request.Context(), userID, c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": gin.H{"items": refunds}})
}

// Callback handles POST /openbank/callbacks (no auth)
func (h *Handler) Callback(c *gin.Context) {
	keyID := c.GetHeader("KeyId")
	if keyID == "" {
		keyID = c.GetHeader("keyid")
	}
	timestamp := c.GetHeader("Timestamp")
	nonce := c.GetHeader("Nonce")
	signature := c.GetHeader("Signature")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.Error("openbank callback: read body failed", "error", err)
		c.String(http.StatusOK, "fail")
		return
	}

	_, err = h.service.ProcessCallback(c.Request.Context(), keyID, timestamp, nonce, signature, string(body))
	if err != nil {
		slog.Error("openbank callback processing failed", "error", err)
		c.String(http.StatusOK, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

// Admin

// CloseOrder handles POST /admin/payment/openbank/orders/:order_no/close
func (h *Handler) CloseOrder(c *gin.Context) {
	resp, err := h.service.CloseOrder(c.Request.Context(), c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// Redetect handles POST /admin/payment/openbank/orders/:order_no/redetect
func (h *Handler) Redetect(c *gin.Context) {
	resp, err := h.service.Redetect(c.Request.Context(), c.Param("order_no"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// ListPendingRefunds handles GET /admin/payment/openbank/refunds/pending
func (h *Handler) ListPendingRefunds(c *gin.Context) {
	page := 1
	pageSize := 20
	items, total, err := h.service.ListPendingRefunds(c.Request.Context(), page, pageSize)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": "success",
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// ApproveRefund handles POST /admin/payment/openbank/refunds/:refund_no/approve
func (h *Handler) ApproveRefund(c *gin.Context) {
	userID, _ := getUserID(c)
	var req ReviewRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "invalid request body"})
		return
	}
	resp, err := h.service.ApproveRefund(c.Request.Context(), c.Param("refund_no"), userID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// RejectRefund handles POST /admin/payment/openbank/refunds/:refund_no/reject
func (h *Handler) RejectRefund(c *gin.Context) {
	userID, _ := getUserID(c)
	var req ReviewRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "invalid request body"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": err.Error()})
		return
	}
	resp, err := h.service.RejectRefund(c.Request.Context(), c.Param("refund_no"), userID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "success", "data": resp})
}

// writeError maps domain errors to HTTP status codes.
func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "payment order not found"})
	case errors.Is(err, ErrOrderAccessDenied):
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "payment order not found"})
	case errors.Is(err, ErrOrderNotPayable):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "payment order is not payable"})
	case errors.Is(err, ErrOrderExpired):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "payment order expired"})
	case errors.Is(err, ErrIdempotencyConflict):
		c.JSON(http.StatusConflict, gin.H{"code": "conflict", "message": "idempotency key conflict"})
	case errors.Is(err, ErrOpenBankNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "internal", "message": "openbank is not configured"})
	case errors.Is(err, ErrOpenBankSignature):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "invalid openbank signature"})
	case errors.Is(err, ErrChannelMismatch):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "payment channel mismatch"})
	case errors.Is(err, ErrOrderNotRefundable):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "payment order is not refundable"})
	case errors.Is(err, ErrRefundAmountExceeded):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "refund amount exceeds remaining order amount"})
	case errors.Is(err, ErrRefundNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "refund not found"})
	case errors.Is(err, ErrRefundPendingExists):
		c.JSON(http.StatusConflict, gin.H{"code": "conflict", "message": "a pending refund request already exists for this order"})
	case errors.Is(err, ErrRefundNotPendingReview):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "refund is not in pending_review state"})
	case errors.Is(err, ErrRefundReviewNoteMissing):
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "message": "review_note is required when rejecting a refund"})
	case errors.Is(err, ErrPlanNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "membership plan not found"})
	default:
		slog.Error("unhandled payment error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "internal", "message": "internal server error"})
	}
}


// sanitizeJSON extracts user ID from the Gin context.
// This package doesn't import the main app's middleware package,
// so we use a simple helper that the main app can inject.
var getUserID = func(c *gin.Context) (int64, bool) {
	// Default: try standard middleware key
	if v, exists := c.Get("user_id"); exists {
		if id, ok := v.(int64); ok {
			return id, true
		}
		if id, ok := v.(float64); ok {
			return int64(id), true
		}
	}
	return 0, false
}

// SetUserIDGetter allows the main application to inject a custom user ID extraction function.
func SetUserIDGetter(fn func(c *gin.Context) (int64, bool)) {
	getUserID = fn
}



