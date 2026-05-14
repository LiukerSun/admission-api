package util

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"dfp-open-sdk-go/config"
	"dfp-open-sdk-go/exception"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/x509"
	"log"
	"math/big"
	"net/url"
)

// SignatureByRSA RSA签名
func SignatureByRSA(content string, privateKey string) string {
	decodeString, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return ""
	}
	hash := sha256.New()
	hash.Write([]byte(content))
	//Step3：获得明文的散列值
	hashText := hash.Sum(nil)

	private, err := x509.ParsePKCS1PrivateKey(decodeString)
	sign, err := rsa.SignPKCS1v15(
		rand.Reader,
		private,
		crypto.SHA256,
		hashText,
	)

	return base64.StdEncoding.EncodeToString(sign)
}

type sm2Signature struct {
	R, S *big.Int
}

type Sm2Signature struct {
	R, S *big.Int
}

// SignatureBySM2 SM2签名
func SignatureBySM2(content string, privateKey string) (string, error) {
	decodeString, _ := base64.StdEncoding.DecodeString(privateKey)
	pri, err := FormatPri(decodeString)
	if err != nil {
		return "", err
	}

	sign, _ := pri.Sign(rand.Reader, []byte(content), nil)
	signature := sm2Signature{}
	_, err = asn1.Unmarshal(sign, &signature)
	if err != nil {
		return "", err
	}
	rbytes := signature.R.Bytes()
	sbytes := signature.S.Bytes()
	resultbytes := append(rbytes, sbytes...)

	return base64.StdEncoding.EncodeToString(resultbytes), nil
}

func VerifyBySM2(contentBytes []byte, sign string, publicKey string) (bool, error) {
	pubKey := FromPublicKey(publicKey)
	// r||s格式转换为r||s的asn1格式
	rsbyte, _ := base64.StdEncoding.DecodeString(sign)
	r := rsbyte[0:32]
	var rBigInt big.Int

	// 将字节数组转换为big.Int
	rBigInt.SetBytes(r)

	s := rsbyte[32:]
	var sBigInt big.Int
	//
	//// 将字节数组转换为big.Int
	sBigInt.SetBytes(s)
	var sm2Sign sm2Signature
	sm2Sign.R = &rBigInt
	sm2Sign.S = &sBigInt
	asn1Sm2SignBytes, _ := asn1.Marshal(sm2Sign)
	verify := pubKey.Verify(contentBytes, asn1Sm2SignBytes)
	return verify, nil
}

// EncryptBySM2PublicKey sm2公钥加密, 结果为base64格式
func EncryptBySM2PublicKey(encodedPublicKey string, contentBytes []byte) (string, error) {
	decodeString, err2 := base64.StdEncoding.DecodeString(encodedPublicKey)
	if err2 != nil {
		return "", nil
	}
	public, err := x509.ReadPublicKeyFromHex(hex.EncodeToString(decodeString))
	if err != nil {
		return "", err
	}
	//
	asn1Encrypt, err := public.EncryptAsn1(contentBytes, rand.Reader)
	if err != nil {
		return "", err
	}
	unmarshal, err := sm2.CipherUnmarshal(asn1Encrypt)
	return base64.StdEncoding.EncodeToString(unmarshal), nil
}

// DecryptBySM2PrivateKey sm2私钥解密
func DecryptBySM2PrivateKey(encodedPrivateKey string, contentBytes []byte) ([]byte, error) {
	privateKeyBytes, err := base64.StdEncoding.DecodeString(encodedPrivateKey)
	if err != nil {
		return nil, err
	}
	privateKey, err := x509.ReadPrivateKeyFromHex(hex.EncodeToString(privateKeyBytes))
	if err != nil {
		return nil, err
	}

	plaintext, err := privateKey.Decrypt(rand.Reader, contentBytes, nil)

	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// FormatPri 从私钥base64格式生成对应私钥格式
func FormatPri(priByte []byte) (*sm2.PrivateKey, error) {
	// 椭圆曲线
	c := sm2.P256Sm2()
	// k倍点
	k := new(big.Int).SetBytes(priByte)
	priv := new(sm2.PrivateKey)
	priv.PublicKey.Curve = c
	priv.D = k
	priv.PublicKey.X, priv.PublicKey.Y = c.ScalarBaseMult(k.Bytes())
	return priv, nil
}

func FromPublicKey(publicKey string) *sm2.PublicKey {
	decodeKey, _ := base64.StdEncoding.DecodeString(publicKey)

	hexPublic := hex.EncodeToString(decodeKey)
	pubKey, _ := x509.ReadPublicKeyFromHex(hexPublic)
	return pubKey
}

func SM2_GenerateKeyPair() (publicKey string, privateKey string, err error) {
	privKey, err := sm2.GenerateKey(rand.Reader) // 生成密钥对
	if err != nil {
		return
	}
	privateKey = x509.WritePrivateKeyToHex(privKey)
	decodeString, _ := hex.DecodeString(privateKey)
	publicKey = x509.WritePublicKeyToHex(&privKey.PublicKey)
	bytes, _ := hex.DecodeString(publicKey)
	privateKey = base64.StdEncoding.EncodeToString(decodeString)
	publicKey = base64.StdEncoding.EncodeToString(bytes)
	return
}

// EncryptBody 加密请求报文内容 json 形式
func EncryptBody(bodyParamString string, keyConfigure config.KeyConfigure, flag bool) (string, error) {
	encryptBodyParamMap := make(map[string]string, 10)
	var encData string
	var encPassword string
	var err error

	if keyConfigure.RespSignAlgorithm == "sm4" {
		// 生成SM4密钥
		pwd := SM4_GenerateKeyPair()

		// 使用SM4算法加密请求报文内容
		encData, err = Sm4Encrypt(bodyParamString, pwd)
		if err != nil {
			return "", err
		}
		// sm2加密SM4密钥
		encPassword, err = EncryptBySM2PublicKey(keyConfigure.PlatEncryptPubKey, []byte(pwd))
		if err != nil {
			return "", err
		}
	}

	encryptBodyParamMap["encData"] = encData
	encryptBodyParamMap["encPassword"] = encPassword
	// 根据flag的值决定返回JSON字符串还是拼接的字符串
	if flag {
		return JSONMarshal(encryptBodyParamMap)
	} else {
		return JointMap(encryptBodyParamMap), nil
	}
}

// JSONMarshal 将map转换为JSON字符串
func JSONMarshal(m map[string]string) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EncryptFormBody  加密请求报文内容 form 形式
func EncryptFormBody(bodyParamString string, keyConfigure config.KeyConfigure) (formData url.Values, err error) {
	var encData string
	var encPassword string
	formData = url.Values{}

	// 生成SM4密钥
	pwd := SM4_GenerateKeyPair()
	// 使用SM4算法加密请求报文内容
	encData, err = Sm4Encrypt(bodyParamString, pwd)
	if err != nil {
		return
	}
	fmt.Println("bodyParamString=", bodyParamString)
	fmt.Println("pwd=", pwd)

	encPassword, err = EncryptBySM2PublicKey(keyConfigure.PlatEncryptPubKey, []byte(pwd))
	if err != nil {
		return
	}
	fmt.Println("RespPubKey = ", keyConfigure.PlatEncryptPubKey)
	formData.Set("encData", encData)
	formData.Set("encPassword", encPassword)
	fmt.Println("encData=", encData)
	fmt.Println("encPassword=", encPassword)

	return
}

// 全报文解密
func DecryptBody(result string, keyConfigure config.KeyConfigure) (string, error) {
	var bodyMap map[string]string
	err := json.Unmarshal([]byte(result), &bodyMap)
	if err != nil {
		log.Fatalln(err)
		return "", exception.BODY_DECRYPT_ERR()
	}

	if bodyMap["encData"] == "" || bodyMap["encPassword"] == "" {
		return "", exception.CONN_ERROR()
	}

	encPassword, _ := base64.StdEncoding.DecodeString(bodyMap["encPassword"])
	pwd, err := DecryptBySM2PrivateKey(keyConfigure.KeyDecryptPriKey, encPassword)
	if err != nil {
		log.Fatalln(err)
		return "", exception.BODY_DECRYPT_ERR()
	}

	decrypt, err := Sm4Decrypt(bodyMap["encData"], string(pwd))
	if err != nil {
		log.Fatalln(err)
		return "", exception.BODY_DECRYPT_ERR()
	}

	return decrypt, nil
}
