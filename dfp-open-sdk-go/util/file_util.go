package util

import (
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"log"
	"os"
)

// FileHash 计算文件hash 使用sha256
func FileHash(filePath string) string {
	_, err := os.Stat(filePath)
	if err != nil {
		log.Fatal(err)
		return ""
	}

	if os.IsNotExist(err) {
		log.Fatal(err)
		return ""
	}

	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatal(err)
		return ""
	}
	h := sha256.New()
	h.Write(file)
	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}
