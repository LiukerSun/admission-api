package util

import (
	"log"
	"testing"
)

func TestSm4Encrypt(t *testing.T) {
	encrypt, _ := Sm4Encrypt("1234", "9A9A4QyNz/pc8kZZXzhXOQ==")
	log.Println(encrypt)

	decrypt, _ := Sm4Decrypt("dX+gclDKCddGUMzT3emC/g==", "9A9A4QyNz/pc8kZZXzhXOQ==")
	log.Println(decrypt)

	//key, _ := base64.StdEncoding.DecodeString("9A9A4QyNz/pc8kZZXzhXOQ==")
	//plaintext := []byte("1234")
	//
	//block, err := sm4.NewCipher(key)
	//if err != nil {
	//	panic(err)
	//}
	//
	//// CBC mode works on blocks so plaintexts may need to be padded to the
	//// next whole block. For an example of such padding, see
	//// https://tools.ietf.org/html/rfc5246#section-6.2.3.2.
	//pkcs7 := padding.NewPKCS7Padding(sm4.BlockSize)
	//paddedPlainText := pkcs7.Pad(plaintext)
	//log.Println(paddedPlainText)
	//
	//// The IV needs to be unique, but not secure. Therefore, it's common to
	//// include it at the beginning of the ciphertext.
	//ciphertext := make([]byte, sm4.BlockSize+len(paddedPlainText))
	//iv := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	//
	//mode := cipher.NewCBCEncrypter(block, iv)
	//mode.CryptBlocks(ciphertext[sm4.BlockSize:], paddedPlainText)
	//
	//log.Println(base64.StdEncoding.EncodeToString(ciphertext))
}
