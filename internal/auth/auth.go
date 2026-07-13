// Package auth implements Tollan's local authentication primitives: argon2id
// password hashing, random API-token generation with hashed storage, and
// HMAC-signed stateless session cookies.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (interactive-strength).
const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword returns a PHC-formatted argon2id hash of the password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the encoded argon2id hash.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var mem, tim uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &tim, &par); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, tim, mem, par, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// GenerateToken returns a new random API token (shown once) and its storage hash.
func GenerateToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = "tol_" + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, HashToken(plaintext), nil
}

// HashToken returns the deterministic storage hash for a token (safe to look up
// by, since tokens carry full entropy).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Sessioner signs and verifies stateless session cookies.
type Sessioner struct {
	key []byte
}

// NewSessioner builds a Sessioner from a signing key.
func NewSessioner(key []byte) *Sessioner { return &Sessioner{key: key} }

// Sign creates a cookie value binding a user id for ttl.
func (s *Sessioner) Sign(userID string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	payload := userID + "." + strconv.FormatInt(exp, 10)
	return payload + "." + s.mac(payload)
}

// Verify validates a cookie value and returns the user id.
func (s *Sessioner) Verify(value string) (string, bool) {
	i := strings.LastIndexByte(value, '.')
	if i < 0 {
		return "", false
	}
	payload, sig := value[:i], value[i+1:]
	if subtle.ConstantTimeCompare([]byte(sig), []byte(s.mac(payload))) != 1 {
		return "", false
	}
	j := strings.LastIndexByte(payload, '.')
	if j < 0 {
		return "", false
	}
	userID, expStr := payload[:j], payload[j+1:]
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return userID, true
}

func (s *Sessioner) mac(payload string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
