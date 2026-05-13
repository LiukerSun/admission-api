package alipay

type Config struct {
	AppID                string
	AppPrivateKey        string
	AppPrivateKeyPath    string
	AppPublicCertPath    string
	AlipayPublicCertPath string
	AlipayRootCertPath   string
	NotifyURL            string
	ReturnURL            string
	IsProduction         bool
	EncryptKey           string
	DecryptKey           string
}
