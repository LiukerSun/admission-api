package candidate

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMasterKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNewIDCardCipher_KeyValidation(t *testing.T) {
	t.Run("rejects empty key", func(t *testing.T) {
		_, err := NewIDCardCipher("")
		assert.Error(t, err)
	})

	t.Run("rejects non-hex key", func(t *testing.T) {
		_, err := NewIDCardCipher("not-hex-zzz")
		assert.Error(t, err)
	})

	t.Run("rejects wrong length", func(t *testing.T) {
		_, err := NewIDCardCipher("abcd")
		assert.Error(t, err)
	})

	t.Run("accepts 32-byte hex", func(t *testing.T) {
		c, err := NewIDCardCipher(testMasterKey)
		require.NoError(t, err)
		assert.NotNil(t, c)
		assert.Len(t, c.aeadKey, 32)
		assert.Len(t, c.hmacKey, 32)
	})

	t.Run("derives distinct subkeys from master", func(t *testing.T) {
		c, err := NewIDCardCipher(testMasterKey)
		require.NoError(t, err)
		assert.NotEqual(t, c.aeadKey, c.hmacKey)
	})
}

func TestIDCardCipher_EncryptDecryptRoundtrip(t *testing.T) {
	c, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)

	plain := "110105200001011234"
	blob, err := c.Encrypt(plain)
	require.NoError(t, err)
	assert.True(t, len(blob) >= gcmNonceBytes+len(plain)+16, "blob should contain nonce + ct + tag")

	got, err := c.Decrypt(blob)
	require.NoError(t, err)
	assert.Equal(t, plain, got)
}

func TestIDCardCipher_EncryptIsRandomized(t *testing.T) {
	c, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)

	plain := "110105200001011234"
	blob1, err := c.Encrypt(plain)
	require.NoError(t, err)
	blob2, err := c.Encrypt(plain)
	require.NoError(t, err)

	assert.NotEqual(t, blob1, blob2, "same plaintext should produce different ciphertext (random nonce)")
}

func TestIDCardCipher_DecryptRejectsTampered(t *testing.T) {
	c, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)

	blob, err := c.Encrypt("110105200001011234")
	require.NoError(t, err)

	blob[len(blob)-1] ^= 0xff
	_, err = c.Decrypt(blob)
	assert.Error(t, err, "tampered blob should fail authentication")
}

func TestIDCardCipher_DecryptRejectsShort(t *testing.T) {
	c, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)
	_, err = c.Decrypt([]byte{0x01, 0x02})
	assert.Error(t, err)
}

func TestIDCardCipher_HashIsDeterministic(t *testing.T) {
	c, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)

	h1 := c.Hash("110105200001011234")
	h2 := c.Hash("110105200001011234")
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 64, "sha256 hex digest is 64 chars")

	_, err = hex.DecodeString(h1)
	assert.NoError(t, err, "hash output should be valid hex")
}

func TestIDCardCipher_HashChangesWithKey(t *testing.T) {
	c1, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)
	c2, err := NewIDCardCipher(strings.Repeat("a", 64))
	require.NoError(t, err)

	plain := "110105200001011234"
	assert.NotEqual(t, c1.Hash(plain), c2.Hash(plain), "different master keys should produce different hashes")
}

func TestIDCardCipher_HashDistinctInputs(t *testing.T) {
	c, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)
	assert.NotEqual(t, c.Hash("110105200001011234"), c.Hash("110105200001011235"))
}

func TestMaskIDCard(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"1", "*"},
		{"1234567", "*******"},
		{"12345678", "123*5678"},
		{"110105200001011234", "110***********1234"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, MaskIDCard(tc.in), "input=%q", tc.in)
	}
}

func TestMaskPhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"138", "***"},
		{"13800138000", "138****8000"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, MaskPhone(tc.in), "input=%q", tc.in)
	}
}
