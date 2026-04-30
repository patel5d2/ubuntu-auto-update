// Package session abstracts session storage so the API can use either an
// in-memory store (legacy + tests) or a Postgres-backed one (production with
// >1 backend). Tokens are hex-encoded random strings; the store sees only
// SHA-256 hashes so a database leak does not yield live session tokens.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Principal describes the actor on the other end of an authenticated request.
// UserID is non-zero for human users; AgentLabel is non-empty for enrollment
// tokens issued to agents (e.g. "host01"). Exactly one of those is populated.
type Principal struct {
	UserID     int32
	Username   string // for users: their username; for agents: "agent:<hostname>"
	Role       string // 'viewer' | 'operator' | 'admin' for users; 'agent' for agents
	SessionID  int32  // DB row id for the session, 0 for memory store
	AgentLabel string // hostname for agents, empty otherwise
}

// IsAgent reports whether the principal is an agent enrollment token rather
// than a human user. Agents only hit /report; everything else needs a user.
func (p Principal) IsAgent() bool { return p.AgentLabel != "" }

// Roles ordered from least to most privileged. Helpers use this order to
// implement "operator can do everything viewer can" semantics.
const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
	RoleAgent    = "agent"
)

// HasRole reports whether the principal's role grants access at least at
// the level of `required`. Admin satisfies everything; agent satisfies only
// the agent role.
func (p Principal) HasRole(required string) bool {
	if p.Role == RoleAdmin {
		return true
	}
	if p.Role == required {
		return true
	}
	if p.Role == RoleOperator && required == RoleViewer {
		return true
	}
	return false
}

// Store is the session-storage interface. Implementations must be safe for
// concurrent use.
type Store interface {
	// Create stores a session for the given principal and returns the raw
	// token (caller hands it to the client). expiry must be > 0.
	Create(ctx context.Context, p Principal, expiry time.Duration, ip, userAgent string) (token string, err error)

	// Validate looks up a session by token. Returns ok=false for missing or
	// expired sessions. Errors only surface on infrastructure failure.
	Validate(ctx context.Context, token string) (Principal, bool, error)

	// Revoke deletes a session. Idempotent — missing tokens are not errors.
	Revoke(ctx context.Context, token string) error

	// CleanExpired removes expired sessions. Safe to call on a timer.
	CleanExpired(ctx context.Context) error
}

// GenerateToken returns a 32-byte cryptographically random token, hex-encoded.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Memory store: kept primarily for tests. Production should use NewDBStore.
// ---------------------------------------------------------------------------

type memoryEntry struct {
	principal Principal
	expiresAt time.Time
}

type memoryStore struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry // key: token hash
}

// NewMemoryStore returns a process-local session store. Useful for tests.
func NewMemoryStore() Store {
	return &memoryStore{entries: make(map[string]memoryEntry)}
}

func (s *memoryStore) Create(_ context.Context, p Principal, expiry time.Duration, _, _ string) (string, error) {
	if expiry <= 0 {
		return "", errors.New("expiry must be positive")
	}
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.entries[hashToken(tok)] = memoryEntry{principal: p, expiresAt: time.Now().Add(expiry)}
	s.mu.Unlock()
	return tok, nil
}

func (s *memoryStore) Validate(_ context.Context, token string) (Principal, bool, error) {
	s.mu.RLock()
	entry, ok := s.entries[hashToken(token)]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return Principal{}, false, nil
	}
	return entry.principal, true, nil
}

func (s *memoryStore) Revoke(_ context.Context, token string) error {
	s.mu.Lock()
	delete(s.entries, hashToken(token))
	s.mu.Unlock()
	return nil
}

func (s *memoryStore) CleanExpired(_ context.Context) error {
	now := time.Now()
	s.mu.Lock()
	for k, v := range s.entries {
		if now.After(v.expiresAt) {
			delete(s.entries, k)
		}
	}
	s.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// DB store: persistent, shared across backend replicas.
// ---------------------------------------------------------------------------

type dbStore struct {
	pool *pgxpool.Pool
}

// NewDBStore returns a Postgres-backed session store. Reads/writes the
// `sessions` table from migration 000013.
func NewDBStore(pool *pgxpool.Pool) Store {
	return &dbStore{pool: pool}
}

func (s *dbStore) Create(ctx context.Context, p Principal, expiry time.Duration, ip, userAgent string) (string, error) {
	if expiry <= 0 {
		return "", errors.New("expiry must be positive")
	}
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	hashed := hashToken(tok)
	expiresAt := time.Now().Add(expiry)

	// We rely on the CHECK constraint to catch impossible (user_id=null,
	// agent_label=null) combos rather than re-validate here.
	var userID *int32
	if p.UserID != 0 {
		userID = &p.UserID
	}
	var agentLabel *string
	if p.AgentLabel != "" {
		agentLabel = &p.AgentLabel
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, agent_label, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''))`,
		hashed, userID, agentLabel, expiresAt, ip, userAgent,
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return tok, nil
}

func (s *dbStore) Validate(ctx context.Context, token string) (Principal, bool, error) {
	if token == "" {
		return Principal{}, false, nil
	}
	hashed := hashToken(token)

	var (
		sessionID  int32
		userID     *int32
		username   *string
		role       *string
		disabledAt *time.Time
		agentLabel *string
		expiresAt  time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.username, u.role, u.disabled_at,
		       s.agent_label, s.expires_at
		FROM sessions s
		LEFT JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1`,
		hashed,
	).Scan(&sessionID, &userID, &username, &role, &disabledAt, &agentLabel, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, fmt.Errorf("lookup session: %w", err)
	}
	if time.Now().After(expiresAt) {
		// Best-effort cleanup; don't bubble the delete failure.
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
		return Principal{}, false, nil
	}
	// A user disabled while their session is still valid loses access
	// immediately. Drop the session so a re-enable starts clean.
	if userID != nil && disabledAt != nil {
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
		return Principal{}, false, nil
	}

	// Bump last_seen_at lazily — fire and forget; ignore failures.
	go func() {
		bumpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = s.pool.Exec(bumpCtx, `UPDATE sessions SET last_seen_at = NOW() WHERE id = $1`, sessionID)
	}()

	p := Principal{SessionID: sessionID}
	if agentLabel != nil {
		p.AgentLabel = *agentLabel
		p.Username = "agent:" + *agentLabel
		p.Role = RoleAgent
	} else if userID != nil {
		p.UserID = *userID
		if username != nil {
			p.Username = *username
		}
		if role != nil {
			p.Role = *role
		}
	}
	return p, true, nil
}

func (s *dbStore) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (s *dbStore) CleanExpired(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	return err
}

// StartCleanup runs CleanExpired on a ticker until ctx is done. Safe to call
// multiple times for layered intervals if you want.
func StartCleanup(ctx context.Context, store Store, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_ = store.CleanExpired(cleanCtx)
				cancel()
			}
		}
	}()
}
