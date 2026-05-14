package util

import "testing"

func TestFileHash(t *testing.T) {
	hash := FileHash("file_util")
	if hash != "" {
		t.Log(hash)
	} else {
		t.Log("计算hash失败")
	}
}
