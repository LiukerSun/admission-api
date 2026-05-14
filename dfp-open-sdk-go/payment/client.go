package payment

import (
	"encoding/json"

	"dfp-open-sdk-go/config"
	"dfp-open-sdk-go/enum"
	"dfp-open-sdk-go/opensdk"
	"dfp-open-sdk-go/util"
)

// Client wraps the OpenBank SDK for payment-specific API calls.
type Client interface {
	CreateNativeOrder(req *NativeOrderRequest) (*NativeOrderResponse, error)
	VerifyAndDecryptCallback(keyID, timestamp, nonce, sign, body string) (string, error)
	TradeQuery(req *TradeQueryRequest) (*TradeQueryResponse, error)
	Refund(req *RefundAPIParams) (*RefundAPIResponse, error)
	RefundQuery(req *RefundQueryAPIParams) (*RefundQueryAPIResponse, error)
	CloseOrder(req *CloseOrderAPIParams) (*CloseOrderAPIResponse, error)
}

type ClientConfig struct {
	DevEnv      bool
	KeyID       string
	PriKey      string // base64
	RespPubKey  string // base64
	DecryptKey  string // SM4 key for callback decryption
	MchID       string
}

type openbankClient struct {
	sdk         opensdk.OpenSDK
	mchID       string
	keyID       string
	decryptKey  string
	respPubKey  string
	devEnv      bool
}

func NewClient(cfg *ClientConfig) (Client, error) {
	keyCfg := &config.KeyConfigure{
		KeyId:              cfg.KeyID,
		PriKey:             cfg.PriKey,
		RespPubKey:         cfg.RespPubKey,
		KeySignType:        enum.SM3WITHSM2,
		RespSignSwitch:     true,
		RespSignAlgorithm:  enum.SM3WITHSM2,
		BodyEncryptSwitch:  false,
	}

	sdkCfg := config.DefaultConfig()
	sdkCfg.DevEnv = cfg.DevEnv
	sdkCfg.AddKeyConfigure(keyCfg)

	sdk := opensdk.NewOpenSdk(sdkCfg)

	return &openbankClient{
		sdk:        sdk,
		mchID:      cfg.MchID,
		keyID:      cfg.KeyID,
		decryptKey: cfg.DecryptKey,
		respPubKey: cfg.RespPubKey,
		devEnv:     cfg.DevEnv,
	}, nil
}

const (
	nativeTradeURL      = "/api/payment/wholeNetworkAcquiring/new/unifiedTradeNative"
	tradeQueryURL       = "/api/payment/wholeNetworkAcquiring/new/unifiedTradeQuery"
	tradeRefundURL      = "/api/payment/wholeNetworkAcquiring/new/unifiedTradeRefund"
	tradeRefundQueryURL = "/api/payment/wholeNetworkAcquiring/new/unifiedTradeRefundQuery"
	tradeCloseURL       = "/api/payment/wholeNetworkAcquiring/new/unifiedTradeClose"
)

func (c *openbankClient) CreateNativeOrder(req *NativeOrderRequest) (*NativeOrderResponse, error) {
	params := c.buildNativeOrderParams(req)
	result, err := c.sdk.GateWayWithKeyId(nativeTradeURL, "POST", nil, nil, params, c.keyID)
	if err != nil {
		return nil, err
	}
	var resp NativeOrderResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *openbankClient) buildNativeOrderParams(req *NativeOrderRequest) map[string]string {
	p := map[string]string{
		"service":       req.Service,
		"mch_id":        c.mchID,
		"out_trade_no":  req.OutTradeNo,
		"body":          req.Body,
		"total_fee":     req.TotalFee,
		"mch_create_ip": req.MchCreateIP,
		"notify_url":    req.NotifyURL,
	}
	if req.Version != "" {
		p["version"] = req.Version
	}
	if req.Attach != "" {
		p["attach"] = req.Attach
	}
	if req.TimeStart != "" {
		p["time_start"] = req.TimeStart
	}
	if req.TimeExpire != "" {
		p["time_expire"] = req.TimeExpire
	}
	if req.OpUserID != "" {
		p["op_user_id"] = req.OpUserID
	}
	if req.DeviceLocation != "" {
		p["device_location"] = req.DeviceLocation
	}
	if req.LimitCreditPay != "" {
		p["limit_credit_pay"] = req.LimitCreditPay
	}
	if req.GoodsDetail != "" {
		p["goods_detail"] = req.GoodsDetail
	}
	if req.TerminalInfoJSON != "" {
		var ti map[string]interface{}
		if err := json.Unmarshal([]byte(req.TerminalInfoJSON), &ti); err == nil {
			m := util.ConvertJsonToForm(req.TerminalInfoJSON)
			for k, v := range m {
				p["terminal_info."+k] = v
			}
		}
	}
	return p
}

func (c *openbankClient) VerifyAndDecryptCallback(keyID, timestamp, nonce, sign, body string) (string, error) {
	return util.DecryptAndVerify(keyID, timestamp, nonce, sign, body, c.decryptKey, c.respPubKey)
}

func (c *openbankClient) TradeQuery(req *TradeQueryRequest) (*TradeQueryResponse, error) {
	params := map[string]string{
		"service": req.Service,
		"mch_id":  c.mchID,
	}
	if req.OutTradeNo != "" {
		params["out_trade_no"] = req.OutTradeNo
	}
	if req.TransactionID != "" {
		params["transaction_id"] = req.TransactionID
	}
	if req.Version != "" {
		params["version"] = req.Version
	}

	result, err := c.sdk.GateWayWithKeyId(tradeQueryURL, "POST", nil, nil, params, c.keyID)
	if err != nil {
		return nil, err
	}
	var resp TradeQueryResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *openbankClient) Refund(req *RefundAPIParams) (*RefundAPIResponse, error) {
	params := map[string]string{
		"service":      req.Service,
		"mch_id":       c.mchID,
		"out_refund_no": req.OutRefundNo,
		"total_fee":    req.TotalFee,
		"refund_fee":   req.RefundFee,
		"op_user_id":   req.OpUserID,
	}
	if req.Version != "" {
		params["version"] = req.Version
	}
	if req.OutTradeNo != "" {
		params["out_trade_no"] = req.OutTradeNo
	}
	if req.TransactionID != "" {
		params["transaction_id"] = req.TransactionID
	}
	if req.RefundChannel != "" {
		params["refund_channel"] = req.RefundChannel
	}

	result, err := c.sdk.GateWayWithKeyId(tradeRefundURL, "POST", nil, nil, params, c.keyID)
	if err != nil {
		return nil, err
	}
	var resp RefundAPIResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *openbankClient) RefundQuery(req *RefundQueryAPIParams) (*RefundQueryAPIResponse, error) {
	params := map[string]string{
		"service": req.Service,
		"mch_id":  c.mchID,
	}
	if req.Version != "" {
		params["version"] = req.Version
	}
	if req.OutTradeNo != "" {
		params["out_trade_no"] = req.OutTradeNo
	}
	if req.TransactionID != "" {
		params["transaction_id"] = req.TransactionID
	}
	if req.OutRefundNo != "" {
		params["out_refund_no"] = req.OutRefundNo
	}
	if req.RefundID != "" {
		params["refund_id"] = req.RefundID
	}

	result, err := c.sdk.GateWayWithKeyId(tradeRefundQueryURL, "POST", nil, nil, params, c.keyID)
	if err != nil {
		return nil, err
	}
	var resp RefundQueryAPIResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *openbankClient) CloseOrder(req *CloseOrderAPIParams) (*CloseOrderAPIResponse, error) {
	params := map[string]string{
		"service":      req.Service,
		"mch_id":       c.mchID,
		"out_trade_no": req.OutTradeNo,
	}
	if req.Version != "" {
		params["version"] = req.Version
	}

	result, err := c.sdk.GateWayWithKeyId(tradeCloseURL, "POST", nil, nil, params, c.keyID)
	if err != nil {
		return nil, err
	}
	var resp CloseOrderAPIResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
