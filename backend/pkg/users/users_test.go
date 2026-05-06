package users

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func newMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return mock
}

// ── HashPassword / IsValidRole ────────────────────────────────────────────

func TestHashPassword_TooShort(t *testing.T) {
	_, err := HashPassword("short")
	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestHashPassword_Valid(t *testing.T) {
	hash, err := HashPassword("longpassword1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hash) == 0 {
		t.Error("expected non-empty hash")
	}
}

func TestIsValidRole(t *testing.T) {
	for _, r := range []string{"viewer", "operator", "admin"} {
		if !IsValidRole(r) {
			t.Errorf("IsValidRole(%q) = false, want true", r)
		}
	}
	for _, r := range []string{"superadmin", "", "ROOT"} {
		if IsValidRole(r) {
			t.Errorf("IsValidRole(%q) = true, want false", r)
		}
	}
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreate_Success(t *testing.T) {
	mock := newMock(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "username", "role", "disabled_at", "created_at", "updated_at",
		"last_login_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), "alice", "viewer", nil, now, now, nil, int32(0), nil)
	mock.ExpectQuery(`INSERT INTO users`).
		WithArgs("alice", pgxmock.AnyArg(), "viewer").
		WillReturnRows(rows)

	u, err := Create(context.Background(), mock, "alice", "longpassword1234", "viewer")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Username != "alice" || u.Role != "viewer" {
		t.Errorf("unexpected user: %+v", u)
	}
}

func TestCreate_InvalidRole(t *testing.T) {
	mock := newMock(t)
	_, err := Create(context.Background(), mock, "alice", "longpassword1234", "badrole")
	if err != ErrInvalidRole {
		t.Errorf("expected ErrInvalidRole, got %v", err)
	}
}

func TestCreate_EmptyUsername(t *testing.T) {
	mock := newMock(t)
	_, err := Create(context.Background(), mock, "   ", "longpassword1234", "viewer")
	if err == nil {
		t.Error("expected error for empty username")
	}
}

func TestCreate_ShortPassword(t *testing.T) {
	mock := newMock(t)
	_, err := Create(context.Background(), mock, "alice", "short", "viewer")
	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────

func TestList_Success(t *testing.T) {
	mock := newMock(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "username", "role", "disabled_at", "created_at", "updated_at",
		"last_login_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), "alice", "admin", nil, now, now, nil, int32(0), nil).
		AddRow(int32(2), "bob", "viewer", nil, now, now, nil, int32(0), nil)
	mock.ExpectQuery(`SELECT .+ FROM users ORDER BY username`).WillReturnRows(rows)

	users, err := List(context.Background(), mock)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestList_Empty(t *testing.T) {
	mock := newMock(t)
	rows := mock.NewRows([]string{"id", "username", "role", "disabled_at", "created_at", "updated_at",
		"last_login_at", "failed_logins", "locked_until"})
	mock.ExpectQuery(`SELECT .+ FROM users ORDER BY username`).WillReturnRows(rows)

	users, err := List(context.Background(), mock)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if users == nil || len(users) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", users)
	}
}

// ── GetByUsername ─────────────────────────────────────────────────────────

func TestGetByUsername_NotFound(t *testing.T) {
	mock := newMock(t)
	rows := mock.NewRows([]string{"id", "username", "role", "disabled_at", "created_at", "updated_at",
		"last_login_at", "failed_logins", "locked_until"})
	mock.ExpectQuery(`SELECT .+ FROM users WHERE username`).WithArgs("ghost").WillReturnRows(rows)

	_, err := GetByUsername(context.Background(), mock, "ghost")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// ── SetPassword ───────────────────────────────────────────────────────────

func TestSetPassword_TooShort(t *testing.T) {
	mock := newMock(t)
	err := SetPassword(context.Background(), mock, 1, "short")
	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestSetPassword_NotFound(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs(int32(99), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := SetPassword(context.Background(), mock, 99, "longpassword1234")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// ── SetRole ───────────────────────────────────────────────────────────────

func TestSetRole_InvalidRole(t *testing.T) {
	mock := newMock(t)
	err := SetRole(context.Background(), mock, 1, "superuser")
	if err != ErrInvalidRole {
		t.Errorf("expected ErrInvalidRole, got %v", err)
	}
}

func TestSetRole_Success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE users SET role`).
		WithArgs(int32(1), "operator").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := SetRole(context.Background(), mock, 1, "operator")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetRole_NotFound(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE users SET role`).
		WithArgs(int32(99), "operator").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := SetRole(context.Background(), mock, 99, "operator")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// ── SetDisabled ───────────────────────────────────────────────────────────

func TestSetDisabled_Success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE users SET disabled_at`).
		WithArgs(int32(1), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := SetDisabled(context.Background(), mock, 1, true); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetDisabled_NotFound(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`UPDATE users SET disabled_at`).
		WithArgs(int32(99), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := SetDisabled(context.Background(), mock, 99, true)
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDelete_Success(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`DELETE FROM users WHERE id`).
		WithArgs(int32(1)).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	if err := Delete(context.Background(), mock, 1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	mock := newMock(t)
	mock.ExpectExec(`DELETE FROM users WHERE id`).
		WithArgs(int32(99)).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	if err := Delete(context.Background(), mock, 99); err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// ── CountUsers ────────────────────────────────────────────────────────────

func TestCountUsers(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(mock.NewRows([]string{"count"}).AddRow(5))

	n, err := CountUsers(context.Background(), mock)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
}

// ── EnsureBootstrapAdmin ──────────────────────────────────────────────────

func TestEnsureBootstrapAdmin_EmptyCredentials(t *testing.T) {
	mock := newMock(t)
	created, err := EnsureBootstrapAdmin(context.Background(), mock, "", "")
	if err != nil || created {
		t.Errorf("expected (false,nil), got (%v,%v)", created, err)
	}
}

func TestEnsureBootstrapAdmin_SkipsIfUsersExist(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(mock.NewRows([]string{"count"}).AddRow(3))

	created, err := EnsureBootstrapAdmin(context.Background(), mock, "admin", "longpassword1234")
	if err != nil || created {
		t.Errorf("expected (false,nil) when users exist, got (%v,%v)", created, err)
	}
}

func TestEnsureBootstrapAdmin_ShortPassword(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(mock.NewRows([]string{"count"}).AddRow(0))

	_, err := EnsureBootstrapAdmin(context.Background(), mock, "admin", "short")
	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}
}

// ── Authenticate ──────────────────────────────────────────────────────────

func TestAuthenticate_UserNotFound(t *testing.T) {
	mock := newMock(t)
	mock.ExpectQuery(`SELECT id, password_hash, role`).
		WithArgs("ghost").
		WillReturnRows(mock.NewRows([]string{"id", "password_hash", "role", "disabled_at", "failed_logins", "locked_until"})) // empty = not found

	_, err := Authenticate(context.Background(), mock, "ghost", "anypassword")
	// pgx.CollectExactlyOneRow returns pgx.ErrNoRows → ErrInvalidCredentials
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

func TestAuthenticate_LockedAccount(t *testing.T) {
	mock := newMock(t)
	locked := time.Now().Add(10 * time.Minute)
	row := mock.NewRows([]string{"id", "password_hash", "role", "disabled_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), "$2a$10$invalid", "viewer", nil, int32(5), &locked)
	mock.ExpectQuery(`SELECT id, password_hash, role`).WithArgs("alice").WillReturnRows(row)

	_, err := Authenticate(context.Background(), mock, "alice", "anypassword")
	if err != ErrAccountLocked {
		t.Errorf("expected ErrAccountLocked, got %v", err)
	}
}

func TestAuthenticate_WrongPassword(t *testing.T) {
	mock := newMock(t)
	hash, _ := HashPassword("correctpassword!")
	row := mock.NewRows([]string{"id", "password_hash", "role", "disabled_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), hash, "viewer", nil, int32(0), nil)
	mock.ExpectQuery(`SELECT id, password_hash, role`).WithArgs("alice").WillReturnRows(row)
	// Expect the failed-logins bump
	mock.ExpectExec(`UPDATE users SET failed_logins`).
		WithArgs(int32(1), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	_, err := Authenticate(context.Background(), mock, "alice", "wrongpassword!")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticate_DisabledAccount(t *testing.T) {
	mock := newMock(t)
	hash, _ := HashPassword("correctpassword!")
	disabledAt := time.Now()
	row := mock.NewRows([]string{"id", "password_hash", "role", "disabled_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), hash, "viewer", &disabledAt, int32(0), nil)
	mock.ExpectQuery(`SELECT id, password_hash, role`).WithArgs("alice").WillReturnRows(row)

	_, err := Authenticate(context.Background(), mock, "alice", "correctpassword!")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials for disabled account, got %v", err)
	}
}

func TestAuthenticate_Success(t *testing.T) {
	mock := newMock(t)
	hash, _ := HashPassword("correctpassword!")
	row := mock.NewRows([]string{"id", "password_hash", "role", "disabled_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), hash, "admin", nil, int32(0), nil)
	mock.ExpectQuery(`SELECT id, password_hash, role`).WithArgs("alice").WillReturnRows(row)
	// Expect the success bump (reset counters)
	mock.ExpectExec(`UPDATE users`).
		WithArgs(int32(1)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	u, err := Authenticate(context.Background(), mock, "alice", "correctpassword!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Username != "alice" || u.Role != "admin" {
		t.Errorf("unexpected user: %+v", u)
	}
}

// Make sure the pgx import doesn't trigger "imported and not used".
var _ = pgx.ErrNoRows
