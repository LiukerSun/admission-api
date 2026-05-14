package opensdk

import (
	"bytes"
	"crypto/tls"
	"dfp-open-sdk-go/config"
	"dfp-open-sdk-go/exception"
	"dfp-open-sdk-go/util"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type OpenSDK struct {
	Configure config.OpenSdkConfigure
}

func NewOpenSdk(config config.OpenSdkConfigure) OpenSDK {
	return OpenSDK{Configure: config}
}

// post表单格式请求
func (sdk OpenSDK) PostWithKeyId(reqUri string, bodyParams map[string]string, keyId string) (string, error) {
	if keyId == "" {
		return "", nil
	}

	keyConfigure := sdk.Configure.KeyConfigures[keyId]
	if keyConfigure == nil {
		return "", exception.KEY_CONFIGURE_ERR()
	}
	result := sdk.exec(reqUri, http.MethodPost, nil, nil, bodyParams, "", *keyConfigure, "")
	return result, nil
}

// post json格式请求
func (sdk OpenSDK) PostJsonWithKeyId(reqUri string, bodyJson string, keyId string) (string, error) {
	keyConfigure := sdk.Configure.GetKeyConfigure(keyId)

	if keyConfigure == nil {
		return "", exception.KEY_CONFIGURE_ERR()
	}

	if !json.Valid([]byte(bodyJson)) {
		return "", exception.PARAM_ERR()
	}

	// 发送请求
	exec := sdk.exec(reqUri, http.MethodPost, nil, nil, nil, "", *keyConfigure, bodyJson)
	return exec, nil
}

func (sdk OpenSDK) GateWayWithKeyId(reqUri string, reqMethod string, headParams map[string]string,
	urlParams map[string]string, bodyParams map[string]string, keyId string) (string, error) {
	if keyId == "" {
		return "", nil
	}

	keyConfigure := sdk.Configure.KeyConfigures[keyId]
	if keyConfigure == nil {
		return "", exception.KEY_CONFIGURE_ERR()
	}
	result := sdk.exec(reqUri, reqMethod, headParams, urlParams, bodyParams, "", *keyConfigure, "")
	return result, nil
}

func (sdk OpenSDK) GatewayJsonWithKeyId(reqUri string, reqMethod string, headParams map[string]string,
	urlParams map[string]string, bodyJson string, keyId string) (string, error) {
	keyConfigure := sdk.Configure.GetKeyConfigure(keyId)

	if keyConfigure == nil {
		return "", exception.KEY_CONFIGURE_ERR()
	}

	if !json.Valid([]byte(bodyJson)) {
		return "", exception.PARAM_ERR()
	}

	// 发送请求
	exec := sdk.exec(reqUri, reqMethod, headParams, urlParams, nil, "", *keyConfigure, bodyJson)
	return exec, nil
}

func (sdk OpenSDK) UploadFileWithKeyId(path string, keyId string) (string, error) {
	keyConfigure := sdk.Configure.GetKeyConfigure(keyId)

	if keyConfigure == nil {
		return "", &exception.SdkError{Code: "", Msg: ""}
	}
	reqUri := config.UPLOAD_FILE_URL
	hash := util.FileHash(path)
	if util.EmptyString(hash) {
		return "", exception.FILE_ERROR_RESULT()
	}
	var bodyParamMap = map[string]string{
		"fileHashValue": hash,
	}
	if keyConfigure.FileGatewaySwitch == true {
		bodyParamMap["keyId"] = keyConfigure.KeyId
	}
	exec := sdk.exec(reqUri, http.MethodPost, nil, nil, bodyParamMap, path, *keyConfigure, "")
	return exec, nil
}

func (sdk OpenSDK) DownloadFileWithKeyId(fileId string, saveFilePath string, keyId string) (string, error) {
	keyConfigure := sdk.Configure.GetKeyConfigure(keyId)

	if keyConfigure == nil {
		return "", exception.KEY_CONFIGURE_ERR()
	}
	reqUri := config.DOWNLOAD_FILE_URL
	var body string
	if keyConfigure.FileGatewaySwitch == true {
		fileName := saveFilePath[strings.LastIndex(saveFilePath, string(os.PathSeparator))+1:]
		bodyJson := fmt.Sprintf("{\"keyId\":\"%s\",\"fileName\":\"%s\",\"fileId\":\"%s\"}", keyId, fileName, fileId)
		body = sdk.exec(reqUri, http.MethodPost, nil, nil, nil, saveFilePath, *keyConfigure, string(bodyJson))
	} else {
		var urlParamMap = map[string]string{
			"fileId": fileId,
		}
		body = sdk.exec(reqUri, http.MethodGet, nil, urlParamMap, nil, saveFilePath, *keyConfigure, "")
	}

	return body, nil
}

func (sdk OpenSDK) exec(url string, method string, headParams map[string]string, urlParams map[string]string,
	bodyParamMap map[string]string, filePath string, keyConfigure config.KeyConfigure, bodyJson string) string {
	signParams := make(map[string]string)

	if headParams != nil {
		for k, v := range headParams {
			signParams[k] = v
		}
	}

	if urlParams != nil {
		for k, v := range urlParams {
			signParams[k] = v
		}
	}

	if bodyParamMap != nil {
		for k, v := range bodyParamMap {
			signParams[k] = v
		}
	}

	if !util.EmptyString(bodyJson) {
		signParams["BODY"] = bodyJson
	}

	reqUri := util.CheckReqUri(url)
	nonce := getNonce(32)
	authorization := sdk.getAuthInfo(method, reqUri, nonce, signParams, keyConfigure)

	// 拼接请求参数并进行编码
	urlParamString := util.JointMap(urlParams)

	buffer := bytes.Buffer{}

	buffer.WriteString(sdk.getPostUrl(reqUri, keyConfigure))
	buffer.WriteString(reqUri)

	switch method {
	case http.MethodGet, http.MethodPost:
		if urlParamString != "" {
			if strings.Contains(buffer.String(), "?") {
				buffer.WriteString("&")
			} else {
				buffer.WriteString("?")
			}
			buffer.WriteString(urlParamString)
		}
		break
	default:
		break
	}

	response := sdk.send(buffer.String(), method, authorization, headParams, filePath, bodyParamMap, keyConfigure, bodyJson)

	return response
}

// 获取签名信息
func (sdk OpenSDK) getAuthInfo(method string, reqUri string, nonce string, signParams map[string]string, keyConfigure config.KeyConfigure) string {
	timestamp := time.Now().Format("20060102150405")
	signstr := keyConfigure.KeyId + "&" + timestamp + "&" + nonce + "&" + method + "&" + reqUri

	keys := make([]string, 0, len(signParams))
	for k := range signParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		signstr = signstr + "&" + k + "=" + signParams[k]
	}

	user := keyConfigure.KeyId + "_" + timestamp + "_" + nonce

	pwd, _ := util.SignatureBySM2(signstr, keyConfigure.PriKey)

	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pwd))
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// 获取n位随机字符串
func getNonce(n int) string {
	b := make([]rune, n)
	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// 发送请求
func (sdk OpenSDK) send(urlPath string, method string, authInfo string, headParams map[string]string, filePath string,
	bodyParamMap map[string]string, keyConfigure config.KeyConfigure, bodyJson string) string {
	var client *http.Client

	var tr http.Transport
	// 测试环境跳过证书校验
	if sdk.Configure.DevEnv {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		client = &http.Client{Transport: &tr}
	}

	// 设置代理
	if sdk.Configure.ProxyAddress != "" {
		proxyUrl, _ := url.Parse(sdk.Configure.ProxyAddress)
		tr.Proxy = http.ProxyURL(proxyUrl)
	}

	client = &http.Client{Transport: &tr, Timeout: 30 * time.Second}

	var request *http.Request
	if method == http.MethodGet {
		request, _ = http.NewRequest(http.MethodGet, urlPath, nil)
	} else {
		if filePath != "" && strings.HasSuffix(urlPath, config.UPLOAD_FILE_URL) {
			file, _ := os.Open(filePath)
			defer func(file *os.File) {
				err := file.Close()
				if err != nil {
					log.Fatal(err)
				}
			}(file)
			var requestBody bytes.Buffer
			multipartWriter := multipart.NewWriter(&requestBody)

			part, err := multipartWriter.CreateFormFile("file", filepath.Base(filePath))
			if err != nil {
				fmt.Println("Failed to create form file:", err)
			}

			// 将文件内容复制到multipart.Part中
			_, err = io.Copy(part, file)
			if err != nil {
				fmt.Println("Failed to copy file content:", err)
			}

			// 添加其他表单字段
			for k, v := range bodyParamMap {
				err := multipartWriter.WriteField(k, v)
				if err != nil {
					return ""
				}
			}

			err = multipartWriter.Close()
			if err != nil {
				return ""
			}
			request, _ = http.NewRequest(method, urlPath, &requestBody)
			request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		} else {
			if bodyJson != "" {
				if keyConfigure.BodyEncryptSwitch {
					body, err := util.EncryptBody(bodyJson, keyConfigure, true)
					if err != nil {
						log.Fatalln("全报文加密失败:", err)
						return ""
					}
					request, _ = http.NewRequest(method, urlPath, bytes.NewReader([]byte(body)))
				} else {
					request, _ = http.NewRequest(method, urlPath, bytes.NewReader([]byte(bodyJson)))
				}

				request.Header.Add("Content-Type", "application/json; charset=UTF-8")
			} else {
				// form表单格式
				if keyConfigure.BodyEncryptSwitch {
					jsContent, err := json.Marshal(bodyParamMap)
					if err != nil {
						log.Println("Json Marshal", err)
						return ""
					}
					urlValues, err := util.EncryptFormBody(string(jsContent), keyConfigure)
					if err != nil {
						log.Println("EncryptFormBody", err)
						return ""
					}
					request, _ = http.NewRequest(method, urlPath, bytes.NewReader([]byte(urlValues.Encode())))
				} else {
					urlValues := url.Values{}
					for key, value := range bodyParamMap {
						urlValues.Set(key, value)
					}
					request, _ = http.NewRequest(method, urlPath, bytes.NewReader([]byte(urlValues.Encode())))
				}

				request.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
			}
		}
	}
	client.Timeout = time.Second * 30

	// 设置请求头信息
	sdk.setHeader(authInfo, keyConfigure, request, headParams)

	// 发送HTTP请求
	response, err := client.Do(request)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println(err)
		}
	}(response.Body)
	if err != nil {
		log.Println(err)
	}

	timestamp := response.Header.Get("Timestamp")
	nonce := response.Header.Get("Nonce")
	signature := response.Header.Get("Signature")

	var result string

	// 如果是文件下载, 就保存文件到本地, 不论是文件网关还是API网关下载文件都采用流式下载
	if response.Header.Get("Content-Type") == "application/octet-stream" {
		out, err := os.Create(filePath)
		if err != nil {
			return ""
		}
		defer out.Close()
		// 创建缓冲区
		buffer := make([]byte, 1024*1024) // 1MB buffer

		// 读取响应体并写入文件
		for {
			n, err := response.Body.Read(buffer)
			if err != nil && err != io.EOF {
				panic(err)
			}

			// 将数据写入文件
			if _, err := out.Write(buffer[:n]); err != nil {
				panic(err)
			}

			if err == io.EOF {
				break
			}
		}
		result = config.FILE_SUCCESS_RESULT
	} else {
		// 获取返回结果
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Fatal(err)
		}
		result = string(body)
	}

	// 响应报文解密
	if keyConfigure.BodyEncryptSwitch && !strings.Contains(result, "\"code\":\"OPEN") {
		result, err = util.DecryptBody(result, keyConfigure)
		if err != nil {
			return result
		}
	}

	// 响应报文验签
	if keyConfigure.RespSignSwitch && !strings.Contains(result, "\"code\":\"OPEN") {
		var verify bool
		var err error
		if keyConfigure.FileGatewaySwitch {
			hash := util.FileHash(filePath)
			verify, err = util.VerifyBySM2([]byte(keyConfigure.KeyId+"&"+timestamp+"&"+nonce+"&"+hash), signature, keyConfigure.RespPubKey)
		} else {
			verify, err = util.VerifyBySM2([]byte(keyConfigure.KeyId+"&"+timestamp+"&"+nonce+"&"+result), signature, keyConfigure.RespPubKey)
		}
		if err != nil || !verify {
			if keyConfigure.FileGatewaySwitch {
				err := os.Remove(filePath)
				if err != nil {
					log.Fatalln(err)
				}
			}
			log.Println(result)
			return ""
		}
	}

	return result
}

// 设置请求头
func (sdk OpenSDK) setHeader(authInfo string, configure config.KeyConfigure, request *http.Request, headParams map[string]string) {
	if configure.EnterpriseBearer != "" {
		authInfo = authInfo + "," + configure.EnterpriseBearer
	}
	request.Header.Add("Authorization", authInfo)
	if configure.XCfcaBasic != "" {
		request.Header.Add("X-Cfca-Basic", configure.XCfcaBasic)
	}

	if headParams != nil {
		for k, v := range headParams {
			request.Header.Add(k, v)
		}
	}

	// 灰度发布配置
	if configure.GrayConfigure != nil && len(configure.GrayConfigure) != 0 {
		for k, v := range configure.GrayConfigure {
			request.Header.Add(k, v)
		}
	}
}

//func (sdk OpenSDK) createRequest(urlPath string, method string, bodyJson string) *http.Request {
//	if method == http.MethodGet {
//		//request, err := http.NewRequest(http.MethodGet, urlPath, nil)
//	} else if method == http.MethodPost {
//		if bodyJson != "" {
//			request, _ = http.NewRequest(method, urlPath, bytes.NewReader([]byte(bodyJson)))
//			request.Header.Add("Content-Type", "application/json; charset=UTF-8")
//		} else {
//			request, _ = http.NewRequest(method, urlPath, strings.NewReader(urlValues.Encode()))
//		}
//	}
//	return nil
//}

func (sdk OpenSDK) getPostUrl(reqUri string, keyConfigure config.KeyConfigure) string {
	var urlPath = ""
	if sdk.Configure.DevEnv {
		urlPath = sdk.Configure.DevUrl
	} else {
		urlPath = sdk.Configure.ProdUrl
	}

	// 判断是否从文件网关下载
	if keyConfigure.FileGatewaySwitch == true && (strings.Contains(reqUri, config.UPLOAD_FILE_URL) || strings.Contains(reqUri, config.DOWNLOAD_FILE_URL)) {
		if sdk.Configure.DevEnv {
			urlPath = sdk.Configure.DevFileUrl
		} else {
			urlPath = sdk.Configure.ProdFileUrl
		}
	}

	if urlPath != "" && strings.HasSuffix(urlPath, "/") {
		return urlPath[0 : len(urlPath)-1]
	}

	return urlPath
}
