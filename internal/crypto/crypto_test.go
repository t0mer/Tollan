package crypto

import (
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	for _, plain := range []string{"", "slack://token@channel", "hunter2"} {
		enc, err := c.Encrypt(plain)
		if err != nil {
			t.Fatal(err)
		}
		if plain != "" && !IsEncrypted(enc) {
			t.Errorf("expected enc prefix for %q", plain)
		}
		dec, err := c.Decrypt(enc)
		if err != nil {
			t.Fatal(err)
		}
		if dec != plain {
			t.Errorf("round-trip = %q, want %q", dec, plain)
		}
	}
	// Encrypt is idempotent on already-encrypted values.
	enc, _ := c.Encrypt("x")
	again, _ := c.Encrypt(enc)
	if enc != again {
		t.Errorf("double-encrypt changed value")
	}
}

func TestKeyPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	k1, _ := LoadOrCreateKey(path)
	k2, _ := LoadOrCreateKey(path)
	if string(k1) != string(k2) {
		t.Fatal("key not persisted")
	}
}
