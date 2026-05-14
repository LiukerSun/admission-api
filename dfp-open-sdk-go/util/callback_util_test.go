package util

import (
	"testing"
)

func TestCallbackEncrypt(t *testing.T) {

}

func TestCallbackSignature(t *testing.T) {

}

func TestDecryptAndVerify(t *testing.T) {
	//var keyid = "KYLxo7PUriwwvr436Pvms1zj"
	//var timestamp = "20240514140352"
	//var nonce = "CGXanb4Lm6AY7A1arMi9QXrs"
	//var signvalue = "0o/lgrPc9oQ+WPShmYehCNl87VfTBd/bU+uw0aCDJN8b89X087MkHEmah/pAwP+tMnNeDI+GziLGBAEqgMqpQQ=="
	//var body = "ZomY/SeGJNimC6dwuwbOiGXwmw14ZymmaMAwk0IMDqSP2Tg3V3G2jRX3E+zr1HwY"
	//var decrypt_key = "k40Dt+bgq56QYSheW0B3Lg=="
	//var public_key = "BMWZI1sHxDBM0MVVKZn18ni7hPEwfxQoobZRQBEh0ea0oEr5sCLDnipF4bIjLeuNhj6bU5QdnP8EvnIxoL2u8YY="
	//decryptBody, _ := DecryptAndVerify(keyid, timestamp, nonce, signvalue, body, decrypt_key, public_key)
	//if decryptBody != "" {
	//	t.Log(decryptBody)
	//} else {
	//	t.Log("验签失败")
	//}

	v2 := DecryptAndVerifyV2("KYXizb3KsWUAQ7CYcoDoxfu6", "20240930161147", "CGPtpqtUfNZa63MAEmmRg1np", "BAerf7yvG502wjye+VLb/POWL6Kp2TgwZQZ+rgInsvyBtQxq7FE2XqhS8CP372wuCQlZh+d5SdtgiSYTvueNvp5gAugpeRMBSKAwY7lsDZi58oG91q5ynq1zrehr2PlB8jdCFZilwYbRao+YbgCAGiktVuBQ12jlcQ==", "PUnrnkA/qwQpJcgMDF+rfe2gaxyGrhL0LfpgACyGDHr2ZQLc8U52tgfnfOmuZJdTJKZ/d+wkj+IbEsMWz6iiBA==", "BaGZqLF7mlb5j+pyQ0gHetxlC1E8nrpI663SvvQ9yt0P54Szm34Zt6qw1IAOzxa2PAY9myjSpMNvwrbJSEpsk/voKgQW+3f1jLn48QJ5CMsoqXrxEH4nlmaJo3/hDK46",
		"BeqYVMYcUnJbw5UHbHU3hySEnxMO+H5oYocmOocqrSA=", "BIyLQ52BTrrkwSprUHlbf/W0z2zLgU+WaluOqRUwelpJAg7/fC+WGfEzpz2UlABtHPOf13iByoj2cbgZjerxPaY=")
	t.Log(v2)
}

func TestDecryptAndVerify2(t *testing.T) {

}
