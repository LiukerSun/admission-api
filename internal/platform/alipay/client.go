package alipay

import (
	"context"
	"encoding/base64"
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
	if cfg.AppID == "" {
		return nil, fmt.Errorf("alipay config is incomplete: app_id is required")
	}
	if cfg.AppPrivateKey == "" && cfg.AppPrivateKeyPath == "" {
		return nil, fmt.Errorf("alipay config is incomplete: app_private_key or app_private_key_path is required")
	}
	if cfg.AppPublicCert == "" && cfg.AppPublicCertPath == "" {
		return nil, fmt.Errorf("alipay config is incomplete: app_public_cert or app_public_cert_path is required")
	}
	if cfg.AlipayPublicCert == "" && cfg.AlipayPublicCertPath == "" {
		return nil, fmt.Errorf("alipay config is incomplete: alipay_public_cert or alipay_public_cert_path is required")
	}
	if cfg.AlipayRootCert == "" && cfg.AlipayRootCertPath == "" {
		return nil, fmt.Errorf("alipay config is incomplete: alipay_root_cert or alipay_root_cert_path is required")
	}

	privateKey, err := loadCert(cfg.AppPrivateKey, cfg.AppPrivateKeyPath, "app_private_key")
	if err != nil {
		return nil, err
	}

	client, err := alipay.New(cfg.AppID, privateKey, cfg.IsProduction)
	if err != nil {
		return nil, fmt.Errorf("create alipay client: %w", err)
	}

	appPublic, err := loadCert(cfg.AppPublicCert, cfg.AppPublicCertPath, "app_public_cert")
	if err != nil {
		return nil, err
	}
	if err := client.LoadAppCertPublicKey(appPublic); err != nil {
		return nil, fmt.Errorf("load app public cert: %w", err)
	}

	alipayPublic, err := loadCert(cfg.AlipayPublicCert, cfg.AlipayPublicCertPath, "alipay_public_cert")
	if err != nil {
		return nil, err
	}
	if err := client.LoadAlipayCertPublicKey(alipayPublic); err != nil {
		return nil, fmt.Errorf("load alipay public cert: %w", err)
	}

	alipayRoot, err := loadCert(cfg.AlipayRootCert, cfg.AlipayRootCertPath, "alipay_root_cert")
	if err != nil {
		return nil, err
	}
	if err := client.LoadAliPayRootCert(alipayRoot); err != nil {
		return nil, fmt.Errorf("load alipay root cert: %w", err)
	}

	if cfg.EncryptKey != "" {
		if err := client.SetEncryptKey(cfg.EncryptKey); err != nil {
			return nil, fmt.Errorf("set alipay encrypt key: %w", err)
		}
	}

	return &alipayClient{client: client, cfg: cfg}, nil
}

// loadCert 优先使用字符串内容，否则从文件路径读取，并把内容标准化成可被
// alipay SDK 直接吃的 PEM 字符串。content 支持三种存放方式：
//   - 原始 PEM（多行，含 -----BEGIN ...-----）
//   - 单行内嵌 \n 字面量（来自 .env 单行场景）
//   - 单行 base64(PEM)（最 portable，env 安全）
func loadCert(content, path, label string) (string, error) {
	if content != "" {
		return decodeCertContent(content), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s file %q: %w", label, path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// decodeCertContent 把 env 里 string 形态的证书统一成 PEM 文本。
func decodeCertContent(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}
	// 单行带 \n 字面量 → 替换成真实换行
	if strings.Contains(s, "\\n") && !strings.Contains(s, "\n") {
		s = strings.ReplaceAll(s, "\\n", "\n")
	}
	// 已经是 PEM 直接返回
	if strings.Contains(s, "-----BEGIN") {
		return s
	}
	// 否则按 base64 解码尝试一次
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil &&
		strings.Contains(string(decoded), "-----BEGIN") {
		return string(decoded)
	}
	// 解码失败保留原文 — 错就让 alipay SDK 报具体格式错
	return s
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
