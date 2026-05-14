package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"github.com/emmansun/gmsm/sm2"
	"log"
	"math/big"
	"testing"
)

func TestSignatureBySM2(t *testing.T) {
	key, _ := sm2.GenerateKey(rand.Reader)

	d := new(big.Int).SetBytes(key.D.Bytes()) // here we do NOT check if the d is in (0, N) or not
	// Create private key from *big.Int
	keyCopy := new(sm2.PrivateKey)
	keyCopy.Curve = sm2.P256()
	keyCopy.D = d
	keyCopy.PublicKey.X, keyCopy.PublicKey.Y = keyCopy.ScalarBaseMult(keyCopy.D.Bytes())
	fmt.Println(keyCopy)
	if !key.Equal(keyCopy) {
		log.Println("private key and copy should be equal")
	}
	pointBytes := elliptic.Marshal(key.Curve, key.X, key.Y)
	// Create public key from point (uncompressed)
	publicKeyCopy := new(ecdsa.PublicKey)
	publicKeyCopy.Curve = sm2.P256()
	publicKeyCopy.X, publicKeyCopy.Y = elliptic.Unmarshal(publicKeyCopy.Curve, pointBytes)
	if !key.PublicKey.Equal(publicKeyCopy) {
		log.Println("public key and copy should be equal")
	}
}

func TestSM2_GenerateKeyPair(t *testing.T) {
	pair, privateKey, _ := SM2_GenerateKeyPair()
	log.Println(pair)
	log.Println(privateKey)
}
