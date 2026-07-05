// Package users manages operator accounts: bcrypt-hashed passwords, role
// assignment, lockout after repeated failed logins. Backed by the `users`
// table from migration 000013.
package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"ubuntu-auto-update/backend/pkg/db"
)

// Lockout policy: too many failed logins triggers a temporary block. Numbers
// are intentionally generous — locking a real admin out after 3 wrong tries
// is a common ops mistake.
const (
	MaxFailedLogins = 8
	LockoutDuration = 15 * time.Minute
)

// Sentinel errors. Handlers should not pass these through verbatim — that
// would leak whether a username exists. Use `WrappedAuthError` from the
// callsite for a generic "invalid credentials" response.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrDuplicateUsername  = errors.New("username already exists")
	ErrInvalidRole        = errors.New("invalid role")
	ErrPasswordTooShort   = errors.New("password must be at least 12 characters")
)

// User mirrors the `users` table row.
type User struct {
	ID           int32      `json:"id"`
	Username     string     `json:"username"`
	Role         string     `json:"role"`
	DisabledAt   *time.Time `json:"disabled_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	FailedLogins int32      `json:"failed_logins"`
	LockedUntil  *time.Time `json:"locked_until,omitempty"`
}

// validRoles is the source of truth, kept in sync with the CHECK constraint.
var validRoles = map[string]bool{
	"viewer":   true,
	"operator": true,
	"admin":    true,
}

// IsValidRole reports whether `role` is one of the three role names. Used by
// handlers and by Create/Update to refuse bogus roles before the DB CHECK
// constraint does it more opaquely.
func IsValidRole(role string) bool { return validRoles[role] }

// HashPassword runs bcrypt at the default cost (currently 10). 12+ char min
// here is a soft floor — real strength lives in the bcrypt cost factor.
func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(hash), nil
}

// Create inserts a new user. Returns ErrDuplicateUsername on unique-violation.
func Create(ctx context.Context, db db.DBTX, username, password, role string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, errors.New("username required")
	}
	if !IsValidRole(role) {
		return User{}, ErrInvalidRole
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}

	rows, err := db.Query(ctx, `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, username, role, disabled_at, created_at, updated_at,
		          last_login_at, failed_logins, locked_until`,
		username, hash, role,
	)
	if err != nil {
		return User{}, mapUserInsertError(err)
	}
	u, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[User])
	if err != nil {
		return User{}, mapUserInsertError(err)
	}
	return u, nil
}

func mapUserInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateUsername
	}
	return err
}

// List returns all users ordered by username.
func List(ctx context.Context, db db.DBTX) ([]User, error) {
	rows, err := db.Query(ctx, `
		SELECT id, username, role, disabled_at, created_at, updated_at,
		       last_login_at, failed_logins, locked_until
		FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	users, err := pgx.CollectRows(rows, pgx.RowToStructByPos[User])
	if err != nil {
		return nil, err
	}
	if users == nil {
		users = []User{}
	}
	return users, nil
}

// CountUsers reports the total number of rows. Used at boot to decide whether
// to seed the bootstrap admin account.
func CountUsers(ctx context.Context, db db.DBTX) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// Authenticate verifies username + password. On success it bumps last_login_at
// and zeroes failed_logins. On wrong-password it increments failed_logins and
// locks the account if MaxFailedLogins is reached. Always returns
// ErrInvalidCredentials for the wrong-user / wrong-password / locked /
// disabled cases — the handler then returns a single 401 to the caller.
func Authenticate(ctx context.Context, db db.DBTX, username, password string) (User, error) {
	var (
		id           int32
		passwordHash string
		role         string
		disabledAt   *time.Time
		failedLogins int32
		lockedUntil  *time.Time
	)
	err := db.QueryRow(ctx, `
		SELECT id, password_hash, role, disabled_at, failed_logins, locked_until
		FROM users WHERE username = $1`, username,
	).Scan(&id, &passwordHash, &role, &disabledAt, &failedLogins, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		// Run a dummy bcrypt to keep the response time roughly constant
		// regardless of whether the username exists. Cheap and obvious.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalid.hash.to.equalize.timing.AAAAAAAAAAAAAAAAAAAAAA"), []byte(password))
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}

	if disabledAt != nil {
		return User{}, ErrInvalidCredentials
	}
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		return User{}, ErrAccountLocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		// Detached ctx: a brute-force attack will close connections quickly.
		// We still need the failure counter to advance so the lockout actually
		// engages, even if the original request times out or is cancelled.
		bumpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		newFailed := failedLogins + 1
		if newFailed >= MaxFailedLogins {
			lock := time.Now().Add(LockoutDuration)
			_, _ = db.Exec(bumpCtx, `
				UPDATE users SET failed_logins = $2, locked_until = $3, updated_at = NOW()
				WHERE id = $1`, id, newFailed, lock)
		} else {
			_, _ = db.Exec(bumpCtx, `
				UPDATE users SET failed_logins = $2, updated_at = NOW()
				WHERE id = $1`, id, newFailed)
		}
		return User{}, ErrInvalidCredentials
	}

	// Success: reset counters and update last_login_at. Same detached-ctx
	// reason — the user is logged in regardless of whether this side-effect
	// completes.
	bumpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = db.Exec(bumpCtx, `
		UPDATE users
		SET failed_logins = 0, locked_until = NULL,
		    last_login_at = NOW(), updated_at = NOW()
		WHERE id = $1`, id)

	return User{ID: id, Username: username, Role: role}, nil
}

// SetPassword updates the password (and only the password). Used both from
// /api/v1/users/{id}/password and from the bootstrap-admin path.
func SetPassword(ctx context.Context, db db.DBTX, userID int32, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = NOW(),
		                 failed_logins = 0, locked_until = NULL
		WHERE id = $1`, userID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetRole changes a user's role. Useful for promoting/demoting via /users API.
func SetRole(ctx context.Context, db db.DBTX, userID int32, role string) error {
	if !IsValidRole(role) {
		return ErrInvalidRole
	}
	tag, err := db.Exec(ctx, `
		UPDATE users SET role = $2, updated_at = NOW() WHERE id = $1`,
		userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetDisabled flips the disabled_at column. We keep the row so audit trail
// references survive (foreign keys are ON DELETE SET NULL anyway).
func SetDisabled(ctx context.Context, db db.DBTX, userID int32, disabled bool) error {
	var ts interface{}
	if disabled {
		ts = time.Now()
	}
	tag, err := db.Exec(ctx, `
		UPDATE users SET disabled_at = $2, updated_at = NOW() WHERE id = $1`,
		userID, ts)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// Delete removes a user row. Sessions cascade via FK; audit_log preserves
// actor_label so the historical trail keeps the username even after deletion.
func Delete(ctx context.Context, db db.DBTX, userID int32) error {
	tag, err := db.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// EnsureBootstrapAdmin creates the first admin from ADMIN_USERNAME /
// ADMIN_PASSWORD env vars if and only if the users table is empty. Idempotent
// and safe to call on every startup. Returns whether a user was created.
//
// Returns ErrPasswordTooShort directly (not wrapped) when ADMIN_PASSWORD is
// shorter than the policy floor — startup callers usually want to surface
// that as a fatal misconfiguration rather than continue with no admin.
func EnsureBootstrapAdmin(ctx context.Context, db db.DBTX, username, password string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return false, nil
	}
	n, err := CountUsers(ctx, db)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return false, nil
	}
	if _, err := Create(ctx, db, username, password, "admin"); err != nil {
		if errors.Is(err, ErrPasswordTooShort) {
			return false, ErrPasswordTooShort
		}
		return false, fmt.Errorf("create bootstrap admin: %w", err)
	}
	return true, nil
}
