package sms

import (
	"context"
	"fmt"
	"log/slog"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"
)

type AliyunConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	Endpoint        string
	SignName        string
	TemplateCode    string
	ParamFormat     string // "json" (default, {"code":"xxx"}) or "direct" (raw code string)
}

type AliyunClient struct {
	client       *dysmsapi.Client
	signName     string
	templateCode string
	paramFormat  string
}

func NewAliyunClient(cfg *AliyunConfig) (Client, error) {
	if cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.SignName == "" || cfg.TemplateCode == "" {
		return nil, fmt.Errorf("aliyun sms config is incomplete")
	}

	openAPIConfig := &openapiutil.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		Endpoint:        tea.String(cfg.Endpoint),
	}

	client, err := dysmsapi.NewClient(openAPIConfig)
	if err != nil {
		return nil, fmt.Errorf("create aliyun sms client: %w", err)
	}

	return &AliyunClient{
		client:       client,
		signName:     cfg.SignName,
		templateCode: cfg.TemplateCode,
		paramFormat:  cfg.ParamFormat,
	}, nil
}

func (c *AliyunClient) SendVerificationCode(ctx context.Context, phone, code string) error {
	var templateParam string
	if c.paramFormat == "direct" {
		templateParam = code
	} else {
		templateParam = fmt.Sprintf(`{"code":%q}`, code)
	}

	req := &dysmsapi.SendSmsRequest{
		PhoneNumbers:  tea.String(phone),
		SignName:      tea.String(c.signName),
		TemplateCode:  tea.String(c.templateCode),
		TemplateParam: tea.String(templateParam),
	}

	resp, err := c.client.SendSmsWithOptions(req, &dara.RuntimeOptions{})
	if err != nil {
		return fmt.Errorf("send sms: %w", err)
	}

	// Log Aliyun tracking IDs even on success — operators search the console by
	// BizId / RequestId to debug delivery failures (Aliyun's "OK" just means
	// the request was queued, not that the carrier accepted it).
	var (
		respCode    string
		respMessage string
		bizID       string
		requestID   string
	)
	if resp.Body != nil {
		if resp.Body.Code != nil {
			respCode = *resp.Body.Code
		}
		if resp.Body.Message != nil {
			respMessage = *resp.Body.Message
		}
		if resp.Body.BizId != nil {
			bizID = *resp.Body.BizId
		}
		if resp.Body.RequestId != nil {
			requestID = *resp.Body.RequestId
		}
	}

	if respCode != "OK" {
		slog.Error("aliyun sms send rejected",
			"phone", phone, "code", respCode, "message", respMessage,
			"biz_id", bizID, "request_id", requestID)
		if respMessage != "" {
			return fmt.Errorf("send sms failed: %s (code=%s)", respMessage, respCode)
		}
		return fmt.Errorf("send sms failed (code=%s)", respCode)
	}

	slog.Info("aliyun sms accepted",
		"phone", phone, "biz_id", bizID, "request_id", requestID,
		"template", c.templateCode, "sign_name", c.signName)

	return nil
}
