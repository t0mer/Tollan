// Package crypto encrypts notification-channel credentials at rest with
// AES-256-GCM. The key is a 32-byte file created in the data directory on first
// run; operators must preserve it alongside the databases.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// prefix marks an encrypted value.
const prefix = "enc:"

// Cipher encrypts and decrypts secret strings.
type Cipher struct {
	gcm cipher.AEAD
}

// LoadOrCreateKey reads a 32-byte key from path, creating a random one if it
// does not exist.
func LoadOrCreateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("key file %q must be 32 bytes, got %d", path, len(data))
		}
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("writing key file: %w", err)
	}
	return key, nil
}

// New builds a Cipher from a 32-byte key.
func New(key []byte) (*Cipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

// IsEncrypted reports whether s is an encrypted value.
func IsEncrypted(s string) bool { return strings.HasPrefix(s, prefix) }

// Encrypt returns an "enc:"-prefixed base64 ciphertext. Empty and already-
// encrypted inputs are returned unchanged.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" || IsEncrypted(plaintext) {
		return plaintext, nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt. Non-encrypted input is returned unchanged (so plain
// legacy values still work).
func (c *Cipher) Decrypt(s string) (string, error) {
	if !IsEncrypted(s) {
		return s, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil {
		return "", err
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}
