package config

import "dfp-open-sdk-go/enum"

const (
	PROD_URL            = "https://open.cibfintech.com"
	PROD_OPENFILE_URL   = "https://openfile.cibfintech.com"
	DEV_URL             = "https://open.test.cibfintech.com"
	DEV_OPENFILE_URL    = "https://openfile.test.cibfintech.com"
	UPLOAD_FILE_URL     = "/api/open/uploadFile"
	DOWNLOAD_FILE_URL   = "/api/open/downloadFile"
	FILE_SUCCESS_RESULT = "{\"code\":\"OPEN25800\",\"msg\":\"下载成功\",\"traceId\":\"OPEN-00-LOCAL-800\"}"
)

type OpenSdkConfigure struct {
	ProdUrl       string                   // 生产环境地址
	ProdFileUrl   string                   // 文件网关生产地址
	DevEnv        bool                     // 是否是测试环境
	DevUrl        string                   // 测试环境地址
	DevFileUrl    string                   // 文件网关测试地址
	KeyConfigures map[string]*KeyConfigure // 多租户密钥配置
	ProxyAddress  string
}

type KeyConfigure struct {
	KeyId                string           // keyId
	PriKey               string           // 私钥
	RespPubKey           string           // 公钥
	ReqParamEncryptKey   string           // 对称加密密钥/字段加密秘钥
	KeySignType          enum.SignType    // 签名类型
	RespSignSwitch       bool             // 是否开启响应报文签名
	RespSignAlgorithm    enum.SignType    // 响应报文签名算法
	FileGatewaySwitch    bool             // 文件网关开关
	BodyEncryptSwitch    bool             // 全报文加密开关
	EncryptAlgorithmEnum enum.EncryptType // 全报文加密算法
	KeyDecryptPriKey     string           // 应用解密私钥
	PlatEncryptPubKey    string           // 平台加密公钥
	CertProtectionPwd    string
	GrayConfigure        map[string]string
	XCfcaBasic           string
	EnterpriseBearer     string
}

// 创建一个不带KeyConfigure的默认OpenConfig
func DefaultConfig() OpenSdkConfigure {
	configure := OpenSdkConfigure{
		DevEnv:      true,
		ProdUrl:     PROD_URL,
		ProdFileUrl: PROD_OPENFILE_URL,
		DevUrl:      DEV_URL,
		DevFileUrl:  DEV_OPENFILE_URL,
	}

	return configure
}

// 根据keyConfigure创建个新的Configure
func NewConfig(config *KeyConfigure) OpenSdkConfigure {
	configure := OpenSdkConfigure{
		DevEnv:      true,
		ProdUrl:     PROD_URL,
		ProdFileUrl: PROD_OPENFILE_URL,
		DevUrl:      DEV_URL,
		DevFileUrl:  DEV_OPENFILE_URL,
	}

	configure.KeyConfigures = make(map[string]*KeyConfigure)
	configure.KeyConfigures[config.KeyId] = config
	return configure
}

func (config *OpenSdkConfigure) SwitchToProd() {
	config.DevEnv = false
}

func (config *OpenSdkConfigure) SwitchToDev() {
	config.DevEnv = true
}

func (config *OpenSdkConfigure) AddKeyConfigure(configure *KeyConfigure) {
	if config.KeyConfigures == nil {
		config.KeyConfigures = make(map[string]*KeyConfigure)
	}
	config.KeyConfigures[configure.KeyId] = configure
}

func (config *OpenSdkConfigure) GetKeyConfigure(keyId string) *KeyConfigure {
	return config.KeyConfigures[keyId]
}
