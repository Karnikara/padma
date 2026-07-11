// Package auth holds padma's authentication helpers: merchant API-key
// generation/hashing/verification and webhook secret generation. Merchant keys
// are high-entropy random tokens, so a fast SHA-256 with a constant-time compare
// is sufficient; only the hash is ever persisted.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// randHex returns n random bytes as a lowercase hex string.
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on supported platforms;
		// panicking here is correct because we cannot mint a secure token.
		panic("auth: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// NewAPIKey returns a fresh plaintext merchant API key (shown to the merchant
// once) and its SHA-256 hash (stored in merchants.api_key_hash).
func NewAPIKey() (plaintext, hash string) {
	plaintext = randHex(32)
	return plaintext, HashAPIKey(plaintext)
}

// HashAPIKey returns the lowercase hex SHA-256 of a plaintext API key.
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// VerifyAPIKey reports whether plaintext hashes to the stored hash, comparing in
// constant time.
func VerifyAPIKey(plaintext, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashAPIKey(plaintext)), []byte(hash)) == 1
}

// NewWebhookSecret returns a fresh per-endpoint secret used to sign outbound
// deliveries (via kanaka's webhook.Sign).
func NewWebhookSecret() string {
	return randHex(32)
}
