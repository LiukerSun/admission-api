package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"dfp-open-sdk-go/exception"
	"encoding/base64"
	"github.com/tjfoc/gmsm/sm4"
	"io"
	"log"
	"math/big"
)

// SM4_GenerateKeyPair 生成sm4秘钥 结果为base64格式
func SM4_GenerateKeyPair() string {
	str := "qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM1234567890"
	buffer := make([]byte, 16)
	for i := 0; i < 16; i++ {
		nextInt, _ := rand.Int(rand.Reader, big.NewInt(int64(len(str))))
		buffer[i] = str[nextInt.Int64()]
	}
	return base64.StdEncoding.EncodeToString(buffer)
}

func AesEncrypt(content string, reqParamEncryptKey string) (string, error) {
	key := []byte(reqParamEncryptKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	//填充原文
	blockSize := block.BlockSize()
	content = string(PKCS5Padding([]byte(content), blockSize))
	//初始向量IV必须是唯一，但不需要保密
	cipherText := make([]byte, blockSize+len(content))
	//block大小 16
	iv := cipherText[:blockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	//block大小和初始向量大小一定要一致
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText[blockSize:], []byte(content))

	return string(cipherText), nil
}

func AesDecrypt(content string, reqParamEncryptKye string) (string, error) {
	decodeKey, err := base64.StdEncoding.DecodeString(reqParamEncryptKye)
	block, err := aes.NewCipher(decodeKey)

	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(content)

	iv := ciphertext[:aes.BlockSize]

	ciphertext = ciphertext[aes.BlockSize:]
	mode := cipher.NewCBCDecrypter(block, iv)

	mode.CryptBlocks(ciphertext, ciphertext)

	result := base64.StdEncoding.EncodeToString(ciphertext)
	return result, nil
}

func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

// Sm4Encrypt sm4加密
// content 待加密内容
// reqParamEncryptKey 对称加密密钥 base64格式
func Sm4Encrypt(content string, reqParamEncryptKey string) (string, error) {
	iv := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	err := sm4.SetIV(iv)
	if err != nil {
		return "", exception.REQ_PARAM_ENCRYPT_ERR()
	}

	decodeString, err := base64.StdEncoding.DecodeString(reqParamEncryptKey)
	if err != nil {
		log.Fatalln(err)
		return "", exception.REQ_PARAM_ENCRYPT_ERR()
	}
	out, err := sm4.Sm4Cbc(decodeString, []byte(content), true)

	if err != nil {
		log.Fatalln(err)
		return "", exception.REQ_PARAM_ENCRYPT_ERR()
	}
	return base64.StdEncoding.EncodeToString(out), nil

}

// Sm4Decrypt sm4加密
// content 待加密内容 base64格式
// reqParamEncryptKey 对称加密密钥 base64格式
func Sm4Decrypt(content string, reqParamEncryptKey string) (string, error) {
	iv := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	err := sm4.SetIV(iv)

	if err != nil {
		return "", exception.REQ_PARAM_ENCRYPT_ERR()
	}

	decodeKey, err := base64.StdEncoding.DecodeString(reqParamEncryptKey)
	decodeContent, _ := base64.StdEncoding.DecodeString(content)
	out, err := sm4.Sm4Cbc(decodeKey, decodeContent, false)

	if err != nil {
		log.Fatalln(err)
		return "", exception.REQ_PARAM_ENCRYPT_ERR()
	}

	return string(out), nil
}
