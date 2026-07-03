package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
)

// Key sourcing precedence (first non-empty wins):
//
//  1. ENCRYPTION_KEY    — hex-encoded 16/24/32-byte key in the environment.
//     Use this for KMS-injected secrets so the key never
//     touches the filesystem.
//  2. ENCRYPTION_KEY_FILE — path to a binary file holding the raw key.
//     Default for local dev / docker-volume deployments.
//  3. encryption.key     — process working-directory fallback for old configs.
//
// Once a key is loaded it is cached for the life of the process; rotating
// the key requires a restart. That trade is fine because rotating means
// re-encrypting every stored ciphertext — there's no online path today.
var (
	keyOnce sync.Once
	keyVal  []byte
	keyErr  error
)

// resetKeyCacheForTest is exposed only inside the package for tests that need
// to override the cached key between calls. Production code never calls it.
func resetKeyCacheForTest() {
	keyOnce = sync.Once{}
	keyVal = nil
	keyErr = nil
}

func getEncryptionKey() ([]byte, error) {
	keyOnce.Do(func() {
		keyVal, keyErr = loadKey()
	})
	return keyVal, keyErr
}

func loadKey() ([]byte, error) {
	// 1) Environment-supplied hex key (preferred for KMS / Docker secrets).
	if env := os.Getenv("ENCRYPTION_KEY"); env != "" {
		decoded, err := hex.DecodeString(env)
		if err != nil {
			return nil, fmt.Errorf("ENCRYPTION_KEY must be hex-encoded: %w", err)
		}
		if err := validateKeyLength(decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}

	// 2) File on disk.
	keyPath := os.Getenv("ENCRYPTION_KEY_FILE")
	if keyPath == "" {
		// 3) Working-directory fallback. Old config; still supported.
		keyPath = "encryption.key"
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read encryption key from %s: %w", keyPath, err)
	}
	if err := validateKeyLength(key); err != nil {
		return nil, err
	}
	return key, nil
}

func validateKeyLength(key []byte) error {
	switch len(key) {
	case 16, 24, 32:
		return nil
	default:
		return fmt.Errorf("invalid encryption key size: %d bytes (must be 16, 24, or 32)", len(key))
	}
}

// Encrypt encrypts a string using AES-GCM and returns the hex-encoded ciphertext.
func Encrypt(stringToEncrypt string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	plaintext := []byte(stringToEncrypt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return fmt.Sprintf("%x", ciphertext), nil
}

// Decrypt decrypts a hex-encoded AES-GCM ciphertext and returns the plaintext.
func Decrypt(encryptedString string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	enc, err := hex.DecodeString(encryptedString)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(enc) < nonceSize {
		return "", fmt.Errorf("ciphertext too short: %d bytes, minimum %d", len(enc), nonceSize)
	}
	nonce, ciphertext := enc[:nonceSize], enc[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	return string(plaintext), nil
}
