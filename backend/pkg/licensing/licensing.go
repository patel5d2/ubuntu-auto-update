// Package licensing implements RS256-signed license token verification for
// the paid feature gating described in the enterprise blueprint.
//
// License tokens are JWTs signed by the vendor's private key. The backend
// validates them using the vendor's public key (embedded or loaded from
// config). Features gated behind a license check call HasFeature; the UI
// queries /api/v1/license to discover which features are available.
package licensing

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Feature is an enumerated paid feature flag.
type Feature string

const (
	FeatureAdvancedViz     Feature = "advanced_viz"
	FeatureCollaboration   Feature = "collaboration"
	FeatureFullRBAC        Feature = "full_rbac"
	FeatureSSO             Feature = "sso_oidc"
	FeatureExtendedAudit   Feature = "extended_audit"
	FeatureManagedHosting  Feature = "managed_hosting"
	FeatureEnterpriseSLA   Feature = "enterprise_sla"
)

// Claims is the JWT payload for a license token.
type Claims struct {
	Issuer     string    `json:"iss"`
	Subject    string    `json:"sub"` // tenant / org ID
	IssuedAt   int64     `json:"iat"`
	ExpiresAt  int64     `json:"exp"`
	MaxHosts   int       `json:"max_hosts,omitempty"`
	Features   []Feature `json:"features"`
	LicenseID  string    `json:"license_id"`
}

// LicenseInfo is the public-facing summary returned to the UI.
type LicenseInfo struct {
	Valid      bool      `json:"valid"`
	ExpiresAt  string    `json:"expires_at,omitempty"`
	MaxHosts   int       `json:"max_hosts,omitempty"`
	Features   []Feature `json:"features"`
	LicenseID  string    `json:"license_id,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// Manager holds the vendor's public key and the currently active license.
type Manager struct {
	mu        sync.RWMutex
	publicKey *rsa.PublicKey
	claims    *Claims
	rawToken  string
}

var (
	ErrNoPublicKey     = errors.New("licensing: no vendor public key configured")
	ErrInvalidToken    = errors.New("licensing: invalid or corrupted license token")
	ErrExpiredLicense  = errors.New("licensing: license has expired")
	ErrMalformedJWT    = errors.New("licensing: token is not a valid 3-part JWT")
)

// NewManager creates a license manager. publicKey may be nil if the
// deployment is free-tier only (all paid features will be denied).
func NewManager(publicKey *rsa.PublicKey) *Manager {
	return &Manager{publicKey: publicKey}
}

// LoadToken validates and stores a license token. Returns the decoded claims
// on success. Thread-safe.
func (m *Manager) LoadToken(tokenStr string) (*Claims, error) {
	if m.publicKey == nil {
		return nil, ErrNoPublicKey
	}

	claims, err := verifyRS256(tokenStr, m.publicKey)
	if err != nil {
		return nil, err
	}

	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrExpiredLicense
	}

	m.mu.Lock()
	m.claims = claims
	m.rawToken = tokenStr
	m.mu.Unlock()

	return claims, nil
}

// HasFeature returns true if the current license includes the given feature.
func (m *Manager) HasFeature(f Feature) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.claims == nil {
		return false
	}
	if m.claims.ExpiresAt > 0 && time.Now().Unix() > m.claims.ExpiresAt {
		return false
	}
	for _, feat := range m.claims.Features {
		if feat == f {
			return true
		}
	}
	return false
}

// Info returns the current license summary for the UI.
func (m *Manager) Info() LicenseInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.claims == nil {
		return LicenseInfo{Valid: false, Error: "No license installed"}
	}
	if m.claims.ExpiresAt > 0 && time.Now().Unix() > m.claims.ExpiresAt {
		return LicenseInfo{Valid: false, Error: "License expired"}
	}
	return LicenseInfo{
		Valid:     true,
		ExpiresAt: time.Unix(m.claims.ExpiresAt, 0).UTC().Format(time.RFC3339),
		MaxHosts:  m.claims.MaxHosts,
		Features:  m.claims.Features,
		LicenseID: m.claims.LicenseID,
	}
}

// MaxHosts returns the host cap from the license (0 = unlimited / no license).
func (m *Manager) MaxHosts() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims == nil {
		return 0
	}
	return m.claims.MaxHosts
}

// ---------------------------------------------------------------------------
// Minimal RS256 JWT verification (no third-party JWT library needed).
// ---------------------------------------------------------------------------

func verifyRS256(token string, pub *rsa.PublicKey) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedJWT
	}

	// Verify signature.
	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: bad signature encoding", ErrInvalidToken)
	}

	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sigBytes); err != nil {
		return nil, fmt.Errorf("%w: signature verification failed", ErrInvalidToken)
	}

	// Decode header — ensure alg is RS256.
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: bad header encoding", ErrInvalidToken)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("%w: malformed header", ErrInvalidToken)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("%w: expected RS256, got %s", ErrInvalidToken, header.Alg)
	}

	// Decode claims.
	claimBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: bad payload encoding", ErrInvalidToken)
	}
	var claims Claims
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return nil, fmt.Errorf("%w: malformed payload", ErrInvalidToken)
	}

	return &claims, nil
}

func base64URLDecode(s string) ([]byte, error) {
	// JWT uses base64url without padding.
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
