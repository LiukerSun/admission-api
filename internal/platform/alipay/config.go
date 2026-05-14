package alipay

type Config struct {
	AppID             string
	AppPrivateKey     string
	AppPrivateKeyPath string
	AppPublicCert     string
	AlipayPublicCert  string
	AlipayRootCert    string
	NotifyURL         string
	ReturnURL         string
	EncryptKey        string
	DecryptKey        string
}
