package candidate

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	idCardKeyBytes = 32
	gcmNonceBytes  = 12
	hkdfInfoAEAD   = "idcard-aead-v1"
	hkdfInfoHMAC   = "idcard-hmac-v1"
)

// IDCardCipher handles ID-card encryption (AES-256-GCM) and search hashing (HMAC-SHA256).
// Two keys are derived from a single master key via HKDF-SHA256 with versioned info labels.
type IDCardCipher struct {
	aeadKey []byte
	hmacKey []byte
}

// NewIDCardCipher constructs a cipher from the hex-encoded master key.
// The master key must decode to exactly 32 bytes (256 bits).
func NewIDCardCipher(masterKeyHex string) (*IDCardCipher, error) {
	if masterKeyHex == "" {
		return nil, errors.New("idcard master key is empty")
	}
	master, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode idcard master key: %w", err)
	}
	if len(master) != idCardKeyBytes {
		return nil, fmt.Errorf("idcard master key must be %d bytes (got %d)", idCardKeyBytes, len(master))
	}

	aeadKey, err := hkdfExpand(master, hkdfInfoAEAD)
	if err != nil {
		return nil, fmt.Errorf("derive aead key: %w", err)
	}
	hmacKey, err := hkdfExpand(master, hkdfInfoHMAC)
	if err != nil {
		return nil, fmt.Errorf("derive hmac key: %w", err)
	}

	return &IDCardCipher{aeadKey: aeadKey, hmacKey: hmacKey}, nil
}

func hkdfExpand(master []byte, info string) ([]byte, error) {
	r := hkdf.New(sha256.New, master, nil, []byte(info))
	out := make([]byte, idCardKeyBytes)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Encrypt encrypts the plaintext with AES-256-GCM and returns nonce||ciphertext||tag.
func (c *IDCardCipher) Encrypt(plain string) ([]byte, error) {
	if plain == "" {
		return nil, errors.New("encrypt: empty plaintext")
	}
	block, err := aes.NewCipher(c.aeadKey)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcmNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

// Decrypt reverses Encrypt. The blob layout is nonce||ciphertext||tag.
func (c *IDCardCipher) Decrypt(blob []byte) (string, error) {
	if len(blob) < gcmNonceBytes {
		return "", errors.New("decrypt: blob too short")
	}
	block, err := aes.NewCipher(c.aeadKey)
	if err != nil {
		return "", fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce, ct := blob[:gcmNonceBytes], blob[gcmNonceBytes:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open: %w", err)
	}
	return string(plain), nil
}

// Hash returns the deterministic HMAC-SHA256 hex digest used for ID-card lookup.
func (c *IDCardCipher) Hash(plain string) string {
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write([]byte(plain))
	return hex.EncodeToString(mac.Sum(nil))
}

// MaskIDCard masks a Chinese ID-card number, preserving the first 3 and last 4 characters.
// Inputs shorter than 8 characters are returned as full asterisks.
func MaskIDCard(plain string) string {
	n := len(plain)
	if n == 0 {
		return ""
	}
	if n <= 7 {
		return strings.Repeat("*", n)
	}
	return plain[:3] + strings.Repeat("*", n-7) + plain[n-4:]
}

// MaskPhone masks a phone number, keeping the first 3 and last 4 digits ("138****8000").
func MaskPhone(plain string) string {
	n := len(plain)
	if n == 0 {
		return ""
	}
	if n <= 7 {
		return strings.Repeat("*", n)
	}
	return plain[:3] + strings.Repeat("*", n-7) + plain[n-4:]
}
