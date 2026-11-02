package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "Pepe12345"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("first HashPassword call failed: %v", err)
	}
	if !strings.HasPrefix(hash1, "$argon2id$") {
		t.Fatalf("unexpected hash format: %q", hash1)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("second HashPassword call failed: %v", err)
	}
	if hash1 == hash2 {
		t.Fatal("same password produced identical hashes: expected random salts")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword("Pepe12345")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	t.Run("correct password", func(t *testing.T) {
		valid, err := VerifyPassword("Pepe12345", hash)
		if err != nil {
			t.Fatalf("VerifyPassword returned an error: %v", err)
		}
		if !valid {
			t.Fatal("correct password was rejected")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		valid, err := VerifyPassword("wrong_password", hash)
		if err != nil {
			t.Fatalf("VerifyPassword returned an error: %v", err)
		}
		if valid {
			t.Fatal("wrong password was accepted")
		}
	})
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	hash, err := HashPassword("Pepe12345")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	parts := strings.Split(hash, "$")
	replacePart := func(index int, replacement string) string {
		modified := append([]string(nil), parts...)
		modified[index] = replacement
		return strings.Join(modified, "$")
	}

	tests := []struct {
		name string
		hash string
	}{
		{"empty string", ""},
		{"missing segment", strings.Join(parts[:5], "$")},
		{"invalid prefix", replacePart(0, "invalid")},
		{"unsupported algorithm", replacePart(1, "argon2i")},
		{"unsupported version", replacePart(2, "v=999")},
		{"unsupported parameters", replacePart(3, "m=999999999,t=100,p=4")},
		{"invalid salt base64", replacePart(4, "!!!")},
		{"invalid salt length", replacePart(4, base64.RawStdEncoding.EncodeToString([]byte{1}))},
		{"invalid key base64", replacePart(5, "!!!")},
		{"invalid key length", replacePart(5, base64.RawStdEncoding.EncodeToString([]byte{1}))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := VerifyPassword("Pepe12345", tt.hash)
			if err == nil {
				t.Fatal("expected an error for malformed hash, got nil")
			}
			if valid {
				t.Fatal("malformed hash was accepted")
			}
		})
	}
}

func TestVerifyPasswordModifiedKey(t *testing.T) {
	hash, err := HashPassword("Pepe12345")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	parts := strings.Split(hash, "$")
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatalf("could not decode test hash: %v", err)
	}
	key[0] ^= 1
	parts[5] = base64.RawStdEncoding.EncodeToString(key)

	valid, err := VerifyPassword("Pepe12345", strings.Join(parts, "$"))
	if err != nil {
		t.Fatalf("validly formatted but modified hash caused error: %v", err)
	}
	if valid {
		t.Fatal("modified key was accepted")
	}
}
