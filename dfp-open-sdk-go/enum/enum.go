package enum

type SignType string

const (
	SHA1WITHRSA   SignType = "SHA1WithRSA"
	SHA256WITHRSA SignType = "SHA256WithRSA"
	SM3WITHSM2    SignType = "SM3WithSM2"
)

type EncryptType string

const (
	SM4 EncryptType = "SM4"
)
