package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestKey(t *testing.T) func() {
	t.Helper()
	key := []byte("0123456789abcdef0123456789abcdef")
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "encryption.key")
	os.WriteFile(keyFile, key, 0600)
	old := os.Getenv("ENCRYPTION_KEY_FILE")
	os.Setenv("ENCRYPTION_KEY_FILE", keyFile)
	return func() {
		if old == "" {
			os.Unsetenv("ENCRYPTION_KEY_FILE")
		} else {
			os.Setenv("ENCRYPTION_KEY_FILE", old)
		}
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	defer setupTestKey(t)()
	plain := "secret SSH key content"
	enc, err := Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("encrypted should differ from plaintext")
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plain {
		t.Errorf("got %q, want %q", dec, plain)
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	defer setupTestKey(t)()
	enc, err := Encrypt("")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "" {
		t.Errorf("expected empty, got %q", dec)
	}
}

func TestDecrypt_InvalidHex(t *testing.T) {
	defer setupTestKey(t)()
	if _, err := Decrypt("not-valid-hex"); err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	defer setupTestKey(t)()
	if _, err := Decrypt("abcd"); err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	defer setupTestKey(t)()
	enc, _ := Encrypt("test")
	tampered := enc[:len(enc)-1] + "0"
	if tampered == enc {
		tampered = enc[:len(enc)-1] + "1"
	}
	if _, err := Decrypt(tampered); err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestEncrypt_MissingKeyFile(t *testing.T) {
	os.Setenv("ENCRYPTION_KEY_FILE", "/nonexistent/key")
	defer os.Unsetenv("ENCRYPTION_KEY_FILE")
	if _, err := Encrypt("test"); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	tmpDir := t.TempDir()
	kf := filepath.Join(tmpDir, "bad.key")
	os.WriteFile(kf, []byte("short"), 0600)
	os.Setenv("ENCRYPTION_KEY_FILE", kf)
	defer os.Unsetenv("ENCRYPTION_KEY_FILE")
	if _, err := Encrypt("test"); err == nil {
		t.Error("expected error for invalid key size")
	}
}

func TestEncrypt_RandomNonce(t *testing.T) {
	defer setupTestKey(t)()
	e1, _ := Encrypt("same")
	e2, _ := Encrypt("same")
	if e1 == e2 {
		t.Error("same plaintext should produce different ciphertexts")
	}
}
