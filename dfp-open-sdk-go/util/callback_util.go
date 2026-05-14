package util

import (
	"dfp-open-sdk-go/exception"
	"encoding/base64"
	"fmt"
)

// DecryptAndVerify 回调响应解密验签
func DecryptAndVerify(keyId string, timestamp string, nonce string, signValue string,
	body string, decryptKey string, publicKey string) (string, error) {
	if keyId == "" || timestamp == "" || nonce == "" || signValue == "" ||
		body == "" || decryptKey == "" || publicKey == "" {
		return "", exception.CALLBACK_BODY_ENCRYPT_ERR()
	}
	decryptedBody, err := Sm4Decrypt(body, decryptKey)

	if err != nil {
		return "", exception.CALLBACK_REQUEST_VERIFY_ERR()
	}

	signParams := fmt.Sprintf("%s&%s&%s&%s", keyId, timestamp, nonce, decryptedBody)
	// 校验
	verify, err := VerifyBySM2([]byte(signParams), signValue, publicKey)
	if err != nil {
		return "", exception.CALLBACK_REQUEST_VERIFY_ERR()
	}
	if verify {
		return decryptedBody, nil
	}
	return "", exception.CALLBACK_REQUEST_VERIFY_ERR()
}

// DecryptAndVerifyV2 动态密钥模式解密
func DecryptAndVerifyV2(keyId string, timestamp string, nonce string, pwd string, signValue string,
	body string, privateKey string, publicKey string) map[string]string {
	var resultMap = make(map[string]string)
	decodeString, _ := base64.StdEncoding.DecodeString(pwd)
	key, _ := DecryptBySM2PrivateKey(privateKey, decodeString)
	sm4Key := string(key)
	decryptBody, _ := Sm4Decrypt(body, sm4Key)

	resultMap["pwd"] = pwd

	if !EmptyString(signValue) {
		signParams := keyId + "&" + timestamp + "&" + nonce + "&" + sm4Key + "&" + decryptBody
		verify, _ := VerifyBySM2([]byte(signParams), signValue, publicKey)

		if !verify {
			return nil
		}

		resultMap["body"] = decryptBody
	}

	return resultMap
}

// CallbackSignature 回调响应签名
func CallbackSignature(keyId string, timestamp string, nonce string, body string, privateKey string) (string, error) {
	if keyId != "" || timestamp != "" || nonce != "" || body != "" {
		return "", exception.CALLBACK_RESPONSE_SIGN_ERR()
	}
	signParams := fmt.Sprintf("%s&%s&%s&%s", keyId, timestamp, nonce, body)

	return SignatureBySM2(signParams, privateKey)
}

// CallbackEncrypt 回调响应加密
func CallbackEncrypt(body string, encryptKey string) (string, error) {
	encrypt, err := Sm4Encrypt(body, encryptKey)
	return encrypt, err
}
