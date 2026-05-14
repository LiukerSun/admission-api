package exception

var SdkErrorTypeMap = map[string]SdkErrorType{
	"FILE_SUCCESS_RESULT":         {Code: "OPEN25801", Msg: "下载成功"},
	"CONN_ERROR":                  {Code: "OPEN25801", Msg: "通讯错误或超时，交易未决"},
	"PARAM_ERR":                   {Code: "OPEN25802", Msg: "参数校验错误"},
	"UNKNOWN":                     {Code: "OPEN25803", Msg: "未知错误，请检查是否为最新版本SDK或是否配置错误"},
	"RESPONSE_SIGN_ERR":           {Code: "OPEN25804", Msg: "响应报文验签失败，请检查验签公钥或验签算法配置"},
	"FILE_ERROR_RESULT":           {Code: "OPEN25805", Msg: "文件读取或写入失败"},
	"REQ_PARAM_ENCRYPT_ERR":       {Code: "OPEN25806", Msg: "字段加/解密失败，请检查字段加密密钥或字段值"},
	"KEY_CONFIGURE_ERR":           {Code: "OPEN25807", Msg: "当前keyId未找到配置对象信息，请检查keyConfigures集合中是否保存"},
	"CALLBACK_REQUEST_VERIFY_ERR": {Code: "OPEN25808", Msg: "回调请求报文验签失败，请检查验签公钥或验签算法配置"},
	"CALLBACK_RESPONSE_SIGN_ERR":  {Code: "OPEN25809", Msg: "回调响应报文签名失败，请检查签名私钥或签名算法配置"},
	"CALLBACK_BODY_ENCRYPT_ERR":   {Code: "OPEN25810", Msg: "回调报文加/解密失败，请检查报文加/解密钥或报文值"},
	"NET_SIGN_ERR":                {Code: "OPEN25812", Msg: "数字信封制作失败，请检查签名/验签密钥或加密算法配置"},
	"BODY_DECRYPT_ERR":            {Code: "OPEN25813", Msg: "报文加/解密失败，请检查报文加/解密钥或报文值"},
}

type SdkErrorType struct {
	Code string
	Msg  string
}

type SdkError struct {
	Code    string
	Msg     string
	traceId string
}

func (err *SdkError) GetCode() string {
	return err.Code
}

func (err *SdkError) Error() string {
	return "{\"code\":\"" + err.Code + "\",\"msg\":\"[" + err.Code + "]" + err.Msg + "\",\"traceId\":\"OPEN-00-LOCAL-" + err.Code[len(err.Code)-3:] + "\"}"
}

func NewSdkError(code string, msg string) error {
	return &SdkError{
		Code: code,
		Msg:  msg,
	}
}

func FILE_SUCCESS_RESULT() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["FILE_SUCCESS_RESULT"].Code,
		Msg: SdkErrorTypeMap["FILE_SUCCESS_RESULT"].Msg}
}
func CONN_ERROR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["CONN_ERROR"].Code,
		Msg: SdkErrorTypeMap["CONN_ERROR"].Msg}
}

func PARAM_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["PARAM_ERR"].Code,
		Msg: SdkErrorTypeMap["PARAM_ERR"].Msg}
}
func UNKNOWN() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["UNKNOWN"].Code,
		Msg: SdkErrorTypeMap["UNKNOWN"].Msg}
}

func RESPONSE_SIGN_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["RESPONSE_SIGN_ERR"].Code,
		Msg: SdkErrorTypeMap["RESPONSE_SIGN_ERR"].Msg}
}

func FILE_ERROR_RESULT() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["FILE_ERROR_RESULT"].Code,
		Msg: SdkErrorTypeMap["FILE_ERROR_RESULT"].Msg}
}
func KEY_CONFIGURE_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["KEY_CONFIGURE_ERR"].Code,
		Msg: SdkErrorTypeMap["KEY_CONFIGURE_ERR"].Msg}
}

func CALLBACK_BODY_ENCRYPT_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["CALLBACK_BODY_ENCRYPT_ERR"].Code,
		Msg: SdkErrorTypeMap["CALLBACK_BODY_ENCRYPT_ERR"].Msg}
}

func REQ_PARAM_ENCRYPT_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["REQ_PARAM_ENCRYPT_ERR"].Code,
		Msg: SdkErrorTypeMap["REQ_PARAM_ENCRYPT_ERR"].Msg}
}

func CALLBACK_REQUEST_VERIFY_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["CALLBACK_REQUEST_VERIFY_ERR"].Code,
		Msg: SdkErrorTypeMap["CALLBACK_REQUEST_VERIFY_ERR"].Msg}
}

func CALLBACK_RESPONSE_SIGN_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["CALLBACK_RESPONSE_SIGN_ERR"].Code,
		Msg: SdkErrorTypeMap["CALLBACK_RESPONSE_SIGN_ERR"].Msg}
}

func NET_SIGN_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["NET_SIGN_ERR"].Code,
		Msg: SdkErrorTypeMap["NET_SIGN_ERR"].Msg}
}

func BODY_DECRYPT_ERR() *SdkError {
	return &SdkError{Code: SdkErrorTypeMap["BODY_DECRYPT_ERR"].Code,
		Msg: SdkErrorTypeMap["BODY_DECRYPT_ERR"].Msg}
}
