package auth

import (
	"testing"
	"time"
)

func TestPasswordHashVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery staple", h) {
		t.Error("verify should succeed")
	}
	if VerifyPassword("wrong", h) {
		t.Error("verify should fail for wrong password")
	}
}

func TestTokenGeneration(t *testing.T) {
	plain, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashToken(plain) != hash {
		t.Error("hash mismatch")
	}
}

func TestSessionSignVerify(t *testing.T) {
	s := NewSessioner([]byte("0123456789abcdef0123456789abcdef"))
	v := s.Sign("user-123", time.Hour)
	uid, ok := s.Verify(v)
	if !ok || uid != "user-123" {
		t.Fatalf("verify = %q %v", uid, ok)
	}
	if _, ok := s.Verify(v + "x"); ok {
		t.Error("tampered cookie should not verify")
	}
	expired := s.Sign("u", -time.Hour)
	if _, ok := s.Verify(expired); ok {
		t.Error("expired cookie should not verify")
	}
}
