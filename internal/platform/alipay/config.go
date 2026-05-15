package alipay

// Config 描述支付宝 SDK 接入参数。
//
// 证书 / 私钥同时支持「字符串内容」与「文件路径」两种方式：
//   - 字符串字段（AppPrivateKey / AppPublicCert / AlipayPublicCert / AlipayRootCert）
//     是 12-factor 友好的部署方式，env 一个变量就装下整个 PEM。
//     格式可以是：原始 PEM 多行 / 单行内嵌 \n 字面量 / base64(PEM)
//     —— 三种都识别（见 client.go decodeCertContent）。
//   - 文件路径字段（*Path）仅在对应字符串字段为空时作为兜底使用。
//
// 优先级：字符串内容 > 文件路径。
type Config struct {
	AppID string

	// 应用私钥：内容或文件二选一
	AppPrivateKey     string
	AppPrivateKeyPath string

	// 应用公钥证书：内容或文件二选一
	AppPublicCert     string
	AppPublicCertPath string

	// 支付宝公钥证书：内容或文件二选一
	AlipayPublicCert     string
	AlipayPublicCertPath string

	// 支付宝根证书：内容或文件二选一
	AlipayRootCert     string
	AlipayRootCertPath string

	NotifyURL    string
	ReturnURL    string
	IsProduction bool
	EncryptKey   string
	DecryptKey   string
}
