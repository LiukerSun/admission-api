package payment

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"admission-api/internal/membership"
	"admission-api/internal/platform/alipay"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fakeAlipay 是一个最小化的 alipay.Client 实现，用于退款审批流程的单元测试。
// 测试用例可通过 refundFn 覆盖 Refund 行为模拟支付宝不同响应。
type fakeAlipay struct {
	refundFn func(*alipay.RefundRequest) (*alipay.RefundResponse, error)
}

func (f *fakeAlipay) BuildPagePayURL(req *alipay.PagePayRequest) (string, error) {
	return "", nil
}
func (f *fakeAlipay) VerifySign(ctx context.Context, params url.Values) error {
	return nil
}
func (f *fakeAlipay) TradeQuery(req *alipay.TradeQueryRequest) (*alipay.TradeQueryResponse, error) {
	return nil, nil
}
func (f *fakeAlipay) Refund(req *alipay.RefundRequest) (*alipay.RefundResponse, error) {
	if f.refundFn != nil {
		return f.refundFn(req)
	}
	return &alipay.RefundResponse{
		FundChange:   "Y",
		TradeNo:      "TRADE_OK",
		OutTradeNo:   req.OutTradeNo,
		OutRequestNo: req.OutRequestNo,
		RefundFee:    req.RefundAmount,
	}, nil
}
func (f *fakeAlipay) RefundQuery(req *alipay.RefundQueryRequest) (*alipay.RefundQueryResponse, error) {
	return nil, nil
}

func makeOrder(id int64, amount int, status string) *Order {
	return &Order{
		ID:             id,
		OrderNo:        "MO-TEST-1",
		UserID:         42,
		Amount:         amount,
		Currency:       "CNY",
		OrderStatus:    status,
		PaymentStatus:  PaymentStatusPaid,
		PaymentChannel: ChannelAlipay,
	}
}

// 用户申请退款只生成 pending_review，不调用支付宝。
func TestRefundOrder_CreatesPendingReview(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, nil, "", "")

	order := makeOrder(1, 990, OrderStatusFulfilled)
	store.On("GetOrderForUser", mock.Anything, int64(42), "MO-TEST-1").
		Return(order, "monthly", nil)
	store.On("CreateRefundRequest", mock.Anything, mock.MatchedBy(func(in *CreateRefundInput) bool {
		return in.PaymentOrderID == 1 && in.RefundAmount == 990 && in.RefundReason == "test-reason"
	})).Return(&Refund{
		ID:             10,
		PaymentOrderID: 1,
		RefundNo:       "RF1",
		RefundAmount:   990,
		Status:         RefundStatusPendingReview,
	}, nil)

	resp, err := svc.RefundOrder(ctx, 42, "MO-TEST-1", RefundOrderRequest{Reason: "test-reason"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, RefundStatusPendingReview, resp.Status)
	assert.Equal(t, 990, resp.RefundAmount)
	store.AssertExpectations(t)
}

// 订单未支付不能申请退款。
func TestRefundOrder_RejectsUnpaidOrder(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, nil, "", "")

	order := makeOrder(1, 990, OrderStatusAwaitingPayment)
	store.On("GetOrderForUser", mock.Anything, int64(42), "MO-TEST-1").
		Return(order, "monthly", nil)

	_, err := svc.RefundOrder(ctx, 42, "MO-TEST-1", RefundOrderRequest{Reason: "test-reason"})
	assert.ErrorIs(t, err, ErrOrderNotRefundable)
}

// 非 alipay 订单不允许走 alipay 退款。
func TestRefundOrder_RejectsWrongChannel(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, nil, "", "")

	order := makeOrder(1, 990, OrderStatusFulfilled)
	order.PaymentChannel = ChannelMock
	store.On("GetOrderForUser", mock.Anything, int64(42), "MO-TEST-1").
		Return(order, "monthly", nil)

	_, err := svc.RefundOrder(ctx, 42, "MO-TEST-1", RefundOrderRequest{Reason: "test-reason"})
	assert.ErrorIs(t, err, ErrChannelMismatch)
}

// 拒绝退款必须填写理由。
func TestRejectRefund_RequiresReviewNote(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, nil, "", "")

	_, err := svc.RejectRefund(ctx, "RF1", 1, ReviewRefundRequest{})
	assert.ErrorIs(t, err, ErrRefundReviewNoteMissing)
}

// 拒绝只允许处于 pending_review 状态。
func TestRejectRefund_OnlyPendingReview(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, nil, "", "")

	store.On("GetRefundByNo", mock.Anything, "RF1").
		Return(&Refund{ID: 10, Status: RefundStatusSuccess}, nil)

	_, err := svc.RejectRefund(ctx, "RF1", 1, ReviewRefundRequest{ReviewNote: "bad"})
	assert.ErrorIs(t, err, ErrRefundNotPendingReview)
}

// approve 只允许 pending_review。
func TestApproveRefund_NotPendingReview(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, &fakeAlipay{}, "", "")

	store.On("GetRefundByNo", mock.Anything, "RF1").
		Return(&Refund{ID: 10, Status: RefundStatusRejected}, nil)

	_, err := svc.ApproveRefund(ctx, "RF1", 1, ReviewRefundRequest{})
	assert.ErrorIs(t, err, ErrRefundNotPendingReview)
}

// 没配置 alipay client → 拒绝。
func TestApproveRefund_NoAlipayClient(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member, nil, "", "")

	_, err := svc.ApproveRefund(ctx, "RF1", 1, ReviewRefundRequest{})
	assert.ErrorIs(t, err, ErrAlipayNotConfigured)
}

// 全额退款审批通过后会撤销会员并更新订单。
func TestApproveRefund_FullAmountRevokesMembership(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)

	order := makeOrder(1, 990, OrderStatusFulfilled)
	pending := &Refund{
		ID: 10, PaymentOrderID: 1, RefundNo: "RF1", OutRequestNo: "RF1",
		Channel: ChannelAlipay, RefundAmount: 990, RefundReason: "ok",
		Status: RefundStatusPendingReview,
	}

	store.On("GetRefundByNo", mock.Anything, "RF1").Return(pending, nil)
	store.On("GetOrderByID", mock.Anything, int64(1)).
		Return(order, "monthly", nil)
	store.On("MarkRefundApproved", mock.Anything, int64(10), int64(99), mock.Anything, mock.Anything).
		Return(&Refund{ID: 10, RefundNo: "RF1", Status: RefundStatusApproved}, nil)
	store.On("MarkRefundProcessing", mock.Anything, int64(10)).
		Return(&Refund{ID: 10, RefundNo: "RF1", Status: RefundStatusProcessing}, nil)
	store.On("UpdateRefundSuccess", mock.Anything, int64(10), "TRADE_OK", mock.Anything).
		Return(&Refund{ID: 10, RefundNo: "RF1", RefundAmount: 990, Status: RefundStatusSuccess}, nil)

	member.On("RevokeFromOrder", mock.Anything, mock.MatchedBy(func(req membership.RevokeRequest) bool {
		return req.UserID == 42 && req.PaymentOrderID == 1
	})).Return(&membership.UserMembership{}, (*membership.Grant)(nil), nil)

	store.On("MarkOrderRefunded", mock.Anything, int64(1)).
		Return(&Order{ID: 1, OrderNo: "MO-TEST-1", OrderStatus: OrderStatusRefunded}, nil)

	svc := NewService(store, member, &fakeAlipay{}, "", "")

	resp, err := svc.ApproveRefund(ctx, "RF1", 99, ReviewRefundRequest{ReviewNote: "OK to refund"})
	require.NoError(t, err)
	assert.Equal(t, RefundStatusSuccess, resp.Status)

	member.AssertCalled(t, "RevokeFromOrder", mock.Anything, mock.AnythingOfType("membership.RevokeRequest"))
	store.AssertCalled(t, "MarkOrderRefunded", mock.Anything, int64(1))
}

// 部分退款不撤销会员。
func TestApproveRefund_PartialAmountKeepsMembership(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)

	order := makeOrder(1, 990, OrderStatusFulfilled)
	pending := &Refund{
		ID: 10, PaymentOrderID: 1, RefundNo: "RF1", OutRequestNo: "RF1",
		Channel: ChannelAlipay, RefundAmount: 100,
		Status: RefundStatusPendingReview,
	}

	store.On("GetRefundByNo", mock.Anything, "RF1").Return(pending, nil)
	store.On("GetOrderByID", mock.Anything, int64(1)).
		Return(order, "monthly", nil)
	store.On("MarkRefundApproved", mock.Anything, int64(10), int64(99), mock.Anything, mock.Anything).
		Return(&Refund{ID: 10, Status: RefundStatusApproved}, nil)
	store.On("MarkRefundProcessing", mock.Anything, int64(10)).
		Return(&Refund{ID: 10, Status: RefundStatusProcessing}, nil)
	store.On("UpdateRefundSuccess", mock.Anything, int64(10), "TRADE_OK", mock.Anything).
		Return(&Refund{ID: 10, RefundNo: "RF1", RefundAmount: 100, Status: RefundStatusSuccess}, nil)

	svc := NewService(store, member, &fakeAlipay{}, "", "")

	resp, err := svc.ApproveRefund(ctx, "RF1", 99, ReviewRefundRequest{})
	require.NoError(t, err)
	assert.Equal(t, RefundStatusSuccess, resp.Status)

	member.AssertNotCalled(t, "RevokeFromOrder", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "MarkOrderRefunded", mock.Anything, mock.Anything)
}

// 支付宝返回 fund_change=N → refund 标记 failed。
func TestApproveRefund_AlipayFundChangeN(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)

	order := makeOrder(1, 990, OrderStatusFulfilled)
	pending := &Refund{
		ID: 10, PaymentOrderID: 1, RefundNo: "RF1", OutRequestNo: "RF1",
		Channel: ChannelAlipay, RefundAmount: 990,
		Status: RefundStatusPendingReview,
	}

	store.On("GetRefundByNo", mock.Anything, "RF1").Return(pending, nil)
	store.On("GetOrderByID", mock.Anything, int64(1)).Return(order, "monthly", nil)
	store.On("MarkRefundApproved", mock.Anything, int64(10), int64(99), mock.Anything, mock.Anything).
		Return(&Refund{ID: 10, Status: RefundStatusApproved}, nil)
	store.On("MarkRefundProcessing", mock.Anything, int64(10)).
		Return(&Refund{ID: 10, Status: RefundStatusProcessing}, nil)
	store.On("UpdateRefundFailed", mock.Anything, int64(10)).Return(nil)

	fakeCli := &fakeAlipay{
		refundFn: func(req *alipay.RefundRequest) (*alipay.RefundResponse, error) {
			return &alipay.RefundResponse{FundChange: "N"}, nil
		},
	}
	svc := NewService(store, member, fakeCli, "", "")

	_, err := svc.ApproveRefund(ctx, "RF1", 99, ReviewRefundRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fund_change")

	store.AssertCalled(t, "UpdateRefundFailed", mock.Anything, int64(10))
	member.AssertNotCalled(t, "RevokeFromOrder", mock.Anything, mock.Anything)
}

// 支付宝调用失败 → refund 标记 failed。
func TestApproveRefund_AlipayAPIError(t *testing.T) {
	ctx := context.Background()
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)

	order := makeOrder(1, 990, OrderStatusFulfilled)
	pending := &Refund{
		ID: 10, PaymentOrderID: 1, RefundNo: "RF1", OutRequestNo: "RF1",
		Channel: ChannelAlipay, RefundAmount: 990,
		Status: RefundStatusPendingReview,
	}

	store.On("GetRefundByNo", mock.Anything, "RF1").Return(pending, nil)
	store.On("GetOrderByID", mock.Anything, int64(1)).Return(order, "monthly", nil)
	store.On("MarkRefundApproved", mock.Anything, int64(10), int64(99), mock.Anything, mock.Anything).
		Return(&Refund{ID: 10, Status: RefundStatusApproved}, nil)
	store.On("MarkRefundProcessing", mock.Anything, int64(10)).
		Return(&Refund{ID: 10, Status: RefundStatusProcessing}, nil)
	store.On("UpdateRefundFailed", mock.Anything, int64(10)).Return(nil)

	apiErr := errors.New("network down")
	fakeCli := &fakeAlipay{
		refundFn: func(req *alipay.RefundRequest) (*alipay.RefundResponse, error) {
			return nil, apiErr
		},
	}
	svc := NewService(store, member, fakeCli, "", "")

	_, err := svc.ApproveRefund(ctx, "RF1", 99, ReviewRefundRequest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, apiErr)

	store.AssertCalled(t, "UpdateRefundFailed", mock.Anything, int64(10))
}
