package sms

import (
	"context"
	"fmt"

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
}

type AliyunClient struct {
	client       *dysmsapi.Client
	signName     string
	templateCode string
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
	}, nil
}

func (c *AliyunClient) SendVerificationCode(ctx context.Context, phone, code string) error {
	req := &dysmsapi.SendSmsRequest{
		PhoneNumbers:  tea.String(phone),
		SignName:      tea.String(c.signName),
		TemplateCode:  tea.String(c.templateCode),
		TemplateParam: tea.String(fmt.Sprintf(`{"code":%q}`, code)),
	}

	resp, err := c.client.SendSmsWithOptions(req, &dara.RuntimeOptions{})
	if err != nil {
		return fmt.Errorf("send sms: %w", err)
	}

	if resp.Body == nil || resp.Body.Code == nil || *resp.Body.Code != "OK" {
		if resp.Body != nil && resp.Body.Message != nil {
			return fmt.Errorf("send sms failed: %s", *resp.Body.Message)
		}
		return fmt.Errorf("send sms failed")
	}

	return nil
}
