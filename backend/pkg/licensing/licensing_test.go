package licensing

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestNewManagerNoKey(t *testing.T) {
	m := NewManager(nil)
	if m.HasFeature(FeatureAdvancedViz) {
		t.Error("expected no features without a key")
	}
	info := m.Info()
	if info.Valid {
		t.Error("expected invalid license with no key")
	}
}

func TestLoadTokenAndHasFeature(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	token := generateTestToken(t, privKey, Claims{
		Issuer:    "test",
		Subject:   "tenant-1",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		MaxHosts:  100,
		Features:  []Feature{FeatureAdvancedViz, FeatureFullRBAC},
		LicenseID: "lic-001",
	})

	m := NewManager(&privKey.PublicKey)
	claims, err := m.LoadToken(token)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if claims.LicenseID != "lic-001" {
		t.Errorf("expected lic-001, got %s", claims.LicenseID)
	}
	if !m.HasFeature(FeatureAdvancedViz) {
		t.Error("expected FeatureAdvancedViz to be present")
	}
	if m.HasFeature(FeatureSSO) {
		t.Error("expected FeatureSSO to be absent")
	}
	info := m.Info()
	if !info.Valid || info.MaxHosts != 100 {
		t.Errorf("unexpected info: %+v", info)
	}
}

func TestExpiredToken(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := generateTestToken(t, privKey, Claims{
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
		Features:  []Feature{FeatureSSO},
	})

	m := NewManager(&privKey.PublicKey)
	_, err := m.LoadToken(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestInvalidSignature(t *testing.T) {
	privKey1, _ := rsa.GenerateKey(rand.Reader, 2048)
	privKey2, _ := rsa.GenerateKey(rand.Reader, 2048)

	token := generateTestToken(t, privKey1, Claims{
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	})

	m := NewManager(&privKey2.PublicKey)
	_, err := m.LoadToken(token)
	if err == nil {
		t.Error("expected error for wrong key")
	}
}

// --- helpers ---

func generateTestToken(t *testing.T, key *rsa.PrivateKey, claims Claims) string {
	t.Helper()

	header := `{"alg":"RS256","typ":"JWT"}`
	hdrB64 := base64.RawURLEncoding.EncodeToString([]byte(header))

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	clmB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := hdrB64 + "." + clmB64
	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64
}
