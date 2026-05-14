package main

import (
	"dfp-open-sdk-go/config"
	"dfp-open-sdk-go/enum"
	"dfp-open-sdk-go/opensdk"
	"dfp-open-sdk-go/util"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"
	"net/http"
)

func main() {

	// 配置应用配置,可配置多个应用
	//keyId := "KYGdqsNCF8oPnevCh1rRtqD3"
	//keyConfigure := config.KeyConfigure{
	//	KeyId:                keyId,
	//	PriKey:               "bunzNs09MIwSW4BcjewJd9stERakj/MHnHlmxhyCYjU=",
	//	RespPubKey:           "BOAMD8CD4EGYIwWkdueVH6ajyLvUGmUSgvgmhn7Nt9TrwW5Ko1UsKUltagslnxhjWt0l/D1aXM1FYq8fv70THmU=",
	//	ReqParamEncryptKey:   "c1EcBsFkdizZC0SPaBxTLw==",
	//	KeySignType:          enum.SM3WITHSM2,
	//	RespSignAlgorithm:    enum.SM3WITHSM2,
	//	RespSignSwitch:       false,
	//	FileGatewaySwitch:    true,
	//	BodyEncryptSwitch:    false,
	//	EncryptAlgorithmEnum: enum.SM4,
	//	KeyDecryptPriKey:     "",
	//	PlatEncryptPubKey:    "",
	//}
	keyId := "KYJq4xKJ6t538TQEDuPjvadV"
	keyConfigure := config.KeyConfigure{
		KeyId:                keyId,
		PriKey:               "l0T+gTZFCzaEDRcrQI+/OKlSZ0+lRnEkadSyNQZLgCs=",
		RespPubKey:           "BCplyKGFruRV2J6WIBXDHdEJiahnGsqEhNzDdkrCAXn+atef14ms6O3XQ+nJ4xhsiVlppNTD/7U/VsVsV4aKmN0=",
		ReqParamEncryptKey:   "c1EcBsFkdizZC0SPaBxTLw==",
		KeySignType:          enum.SM3WITHSM2,
		RespSignAlgorithm:    enum.SM3WITHSM2,
		RespSignSwitch:       false,
		FileGatewaySwitch:    false,
		BodyEncryptSwitch:    false,
		EncryptAlgorithmEnum: enum.SM4,
		KeyDecryptPriKey:     "qWbco36MPZbucIGsSXEvsQawyWZ5PkUg6QnyE1OVtCg=",
		PlatEncryptPubKey:    "BGWS3bKyRoz7+HyBg3kexbYCtZL/CDR/rDcE3x86ib0Xq9QvWJfDtdJGTFsQl0aBXfBOGlN7EriqlA9XvKpCbCM=",
	}

	// 默认为测试环境
	openSdkConfig := config.NewConfig(&keyConfigure)
	// 切换至生产环境
	//openSdkConfig.SwitchToProd()
	sdk := opensdk.NewOpenSdk(openSdkConfig)

	infoMap2 := map[string]string{"mc": "false"}

	// POST表单格式请求
	response, _ := sdk.GateWayWithKeyId("/api/testApi", http.MethodPost, nil, nil, infoMap2, keyId)
	fmt.Println(response)

	// 文件上传
	//response3, _ := sdk.UploadFileWithKeyId("C:\\Users\\cy\\Desktop\\phpsm2sm3sm4-main.zip", keyId)
	//fmt.Println(response3)

	// 文件下载
	//response4, _ := sdk.DownloadFileWithKeyId("exchange/M00/14/9B/Cj_Bu2VxKluANlz1AADek3NRg8E.029e4c", "C:\\Users\\cy\\Desktop\\new_sdk\\dfp-open-sdk-go\\tmp2.zip", keyId)
	//fmt.Println(response4)
}

func queryPubKey(sdk opensdk.OpenSDK, keyId string) {
	response, err := sdk.GateWayWithKeyId("/api/open/queryPubKey", http.MethodGet, nil, nil, nil, keyId)
	if err != nil {
		fmt.Println(response)
	}
}

func testSignature() {
	//priv, err := sm2.GenerateKey(rand.Reader) // 生成密钥对
	//if err != nil {
	//	log.Fatal(err)
	//}
	//fmt.Println(base64.StdEncoding.EncodeToString(priv.D.Bytes()))
	//bytes := priv.PublicKey.X.Bytes()
	//bytes2 := priv.PublicKey.Y.Bytes()
	//i := append(bytes, bytes2...)
	//fmt.Println(base64.StdEncoding.EncodeToString(i))

	bytes, _ := base64.StdEncoding.DecodeString("0WeJfbzJwVSrwGITgII1geoNEs+HpKHj7qvzwkT91D8=")
	pri, _ := util.FormatPri(bytes)
	fmt.Println(base64.StdEncoding.EncodeToString(pri.D.Bytes()))

	msg := "123"
	sign := "MEUCIQDzy3MXxYmjFeUQRCIcaA7YojFsYQ0UB/waaLr5+0a+0wIgLbX1w3peHwvbFyPo1ub3s9RWuWBsgD23VZn7riRKpLM="
	decodeString, _ := base64.StdEncoding.DecodeString(sign)
	verify := pri.PublicKey.Verify([]byte(msg), decodeString)
	fmt.Println(verify)

	// 接下来将rs字节数组格式转换为asn1格式

	rs := "88tzF8WJoxXlEEQiHGgO2KIxbGENFAf8Gmi6+ftGvtMttfXDel4fC9sXI+jW5vez1Fa5YGyAPbdVmfuuJEqksw=="
	rsbyte, _ := base64.StdEncoding.DecodeString(rs)
	r := rsbyte[0:32]
	s := rsbyte[32:]
	var bigInt big.Int
	//
	//// 将字节数组转换为big.Int
	bigInt.SetBytes(r)

	var bigInt2 big.Int
	//
	//// 将字节数组转换为big.Int
	bigInt2.SetBytes(s)
	var sm2Sign util.Sm2Signature
	sm2Sign.R = &bigInt
	sm2Sign.S = &bigInt2
	marshal, _ := asn1.Marshal(sm2Sign)
	verify2 := pri.PublicKey.Verify([]byte(msg), marshal)
	fmt.Println(verify2)

	key := util.FromPublicKey("Uk5Q8y4G3Huoz03nMjs6FInmSDHvusr/SDXXhj/Mt3TqvI3aHKG9Ia6jkneE9qkxjF73ZjxSqfxrFjV9F0b3bg==")
	b := key.Verify([]byte(msg), marshal)
	fmt.Println(b)
}

func execParam(sdk opensdk.OpenSDK, keyId string) {
	var bodyParam = map[string]string{
		"stringParam":             "Hello World",
		"intParam":                "111",
		"byteParam":               "1",
		"shortParam":              "1111",
		"longParam":               "111111111",
		"floatParam":              "111.111",
		"doubleParam":             "111.111111",
		"boolParam":               "false",
		"charParam":               "A",
		"mockBean[0].intParam":    "111",
		"mockBean[0].stringParam": "ascb",

		"stringList[0]": "stringList0",
		"stringList[1]": "stringList1",

		// 嵌套类型
		"mockBeanList[0].intParam":    "111",
		"mockBeanList[0].doubleParam": "111.111",
		"mockBeanList[0].stringParam": "group1/M00/97/68/CmAFGVpd3CGAREPDAAAECO1Nr3I123.txt",
		"mockBeanList[1].intParam":    "222",
		"mockBeanList[1].doubleParam": "222.222",
		"mockBeanList[1].stringParam": "group2/M00/97/68/CmAFGVpd3CGAREPDAAAECO1Nr3I123.txt",
	}
	response, _ := sdk.PostWithKeyId("/api/testApi", bodyParam, keyId)
	fmt.Println(response)
}
func execJson(sdk opensdk.OpenSDK, keyId string) {
	bodyJson := "{\"key\":\"value\"}"
	response, err := sdk.PostJsonWithKeyId("/api/testApi", bodyJson, keyId)
	if err != nil {
		panic(err)
	}
	fmt.Println(response)
}

func uploadFileWithKeyId(sdk opensdk.OpenSDK, keyId string) {
	var filePath = "C:\\Users\\test\\Readme.txt"
	response, err := sdk.UploadFileWithKeyId(filePath, keyId)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(response)
}

func downloadFileWithKeyId(sdk opensdk.OpenSDK, keyId string) {
	fileId := "bucketxxx"
	saveFilePath := "C:\\Users\\test\\Readme.txt"
	response, _ := sdk.DownloadFileWithKeyId(fileId, saveFilePath, keyId)
	fmt.Println(response)
}
