package alipay

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/smartwalle/alipay/v3"
)

type RefundRequest struct {
	OutTradeNo   string
	RefundAmount string
	RefundReason string
	OutRequestNo string
}

type RefundResponse struct {
	FundChange   string
	RefundFee    string
	SendBackFee  string
	TradeNo      string
	OutTradeNo   string
	OutRequestNo string
}

type RefundQueryRequest struct {
	OutTradeNo   string
	OutRequestNo string
}

type RefundQueryResponse struct {
	RefundAmount string
	RefundStatus string
	OutTradeNo   string
	OutRequestNo string
	TradeNo      string
}

type PagePayRequest struct {
	OutTradeNo  string
	Subject     string
	TotalAmount string
	ReturnURL   string
}

type TradeQueryRequest struct {
	OutTradeNo string
	TradeNo    string
}

type TradeQueryResponse struct {
	TradeNo       string
	OutTradeNo    string
	TradeStatus   string
	TotalAmount   string
	ReceiptAmount string
}

type Client interface {
	BuildPagePayURL(req *PagePayRequest) (string, error)
	VerifySign(ctx context.Context, params url.Values) error
	TradeQuery(req *TradeQueryRequest) (*TradeQueryResponse, error)
	Refund(req *RefundRequest) (*RefundResponse, error)
	RefundQuery(req *RefundQueryRequest) (*RefundQueryResponse, error)
}

type alipayClient struct {
	client *alipay.Client
	cfg    *Config
}

func NewClient(cfg *Config) (Client, error) {
	if cfg.AppID == "" ||
		(cfg.AppPrivateKey == "" && cfg.AppPrivateKeyPath == "") ||
		cfg.AppPublicCertPath == "" || cfg.AlipayPublicCertPath == "" || cfg.AlipayRootCertPath == "" {
		return nil, fmt.Errorf("alipay config is incomplete: app_id, app_private_key (or app_private_key_path), and cert paths are required")
	}

	privateKey := cfg.AppPrivateKey
	if privateKey == "" && cfg.AppPrivateKeyPath != "" {
		data, err := os.ReadFile(cfg.AppPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read app private key file: %w", err)
		}
		privateKey = strings.TrimSpace(string(data))
	}

	client, err := alipay.New(cfg.AppID, privateKey, cfg.IsProduction)
	if err != nil {
		return nil, fmt.Errorf("create alipay client: %w", err)
	}

	if err := client.LoadAppCertPublicKeyFromFile(cfg.AppPublicCertPath); err != nil {
		return nil, fmt.Errorf("load app public cert: %w", err)
	}
	if err := client.LoadAlipayCertPublicKeyFromFile(cfg.AlipayPublicCertPath); err != nil {
		return nil, fmt.Errorf("load alipay public cert: %w", err)
	}
	if err := client.LoadAliPayRootCertFromFile(cfg.AlipayRootCertPath); err != nil {
		return nil, fmt.Errorf("load alipay root cert: %w", err)
	}

	if cfg.EncryptKey != "" {
		if err := client.SetEncryptKey(cfg.EncryptKey); err != nil {
			return nil, fmt.Errorf("set alipay encrypt key: %w", err)
		}
	}

	return &alipayClient{client: client, cfg: cfg}, nil
}

func (c *alipayClient) BuildPagePayURL(req *PagePayRequest) (string, error) {
	var p = alipay.TradePagePay{}
	p.Subject = req.Subject
	p.OutTradeNo = req.OutTradeNo
	p.TotalAmount = req.TotalAmount
	p.ProductCode = "FAST_INSTANT_TRADE_PAY"
	p.NotifyURL = c.cfg.NotifyURL
	p.ReturnURL = c.resolveReturnURL(req.ReturnURL)

	result, err := c.client.TradePagePay(p)
	if err != nil {
		return "", fmt.Errorf("alipay build page pay url: %w", err)
	}
	return result.String(), nil
}

func (c *alipayClient) resolveReturnURL(requestReturnURL string) string {
	if requestReturnURL != "" {
		return requestReturnURL
	}
	return c.cfg.ReturnURL
}

func (c *alipayClient) VerifySign(ctx context.Context, params url.Values) error {
	return c.client.VerifySign(ctx, params)
}

func (c *alipayClient) TradeQuery(req *TradeQueryRequest) (*TradeQueryResponse, error) {
	var p = alipay.TradeQuery{}
	p.OutTradeNo = req.OutTradeNo
	p.TradeNo = req.TradeNo

	rsp, err := c.client.TradeQuery(context.Background(), p)
	if err != nil {
		return nil, fmt.Errorf("alipay trade query: %w", err)
	}
	return &TradeQueryResponse{
		TradeNo:       rsp.TradeNo,
		OutTradeNo:    rsp.OutTradeNo,
		TradeStatus:   string(rsp.TradeStatus),
		TotalAmount:   rsp.TotalAmount,
		ReceiptAmount: rsp.ReceiptAmount,
	}, nil
}

func (c *alipayClient) Refund(req *RefundRequest) (*RefundResponse, error) {
	var p = alipay.TradeRefund{}
	p.OutTradeNo = req.OutTradeNo
	p.RefundAmount = req.RefundAmount
	p.RefundReason = req.RefundReason
	p.OutRequestNo = req.OutRequestNo

	rsp, err := c.client.TradeRefund(context.Background(), p)
	if err != nil {
		return nil, fmt.Errorf("alipay trade refund: %w", err)
	}
	return &RefundResponse{
		FundChange:   rsp.FundChange,
		RefundFee:    rsp.RefundFee,
		SendBackFee:  rsp.SendBackFee,
		TradeNo:      rsp.TradeNo,
		OutTradeNo:   rsp.OutTradeNo,
		OutRequestNo: req.OutRequestNo,
	}, nil
}

func (c *alipayClient) RefundQuery(req *RefundQueryRequest) (*RefundQueryResponse, error) {
	var p = alipay.TradeFastPayRefundQuery{}
	p.OutTradeNo = req.OutTradeNo
	p.OutRequestNo = req.OutRequestNo

	rsp, err := c.client.TradeFastPayRefundQuery(context.Background(), p)
	if err != nil {
		return nil, fmt.Errorf("alipay refund query: %w", err)
	}
	return &RefundQueryResponse{
		RefundAmount: rsp.RefundAmount,
		RefundStatus: rsp.RefundStatus,
		OutTradeNo:   rsp.OutTradeNo,
		OutRequestNo: rsp.OutRequestNo,
		TradeNo:      rsp.TradeNo,
	}, nil
}
