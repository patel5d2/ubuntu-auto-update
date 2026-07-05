// Package apitokens implements long-lived operator API tokens ("PATs") for
// automation: admin-minted, role-scoped, stored as SHA-256 hashes. The raw
// token (uat_…) is shown exactly once at creation.
package apitokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ubuntu-auto-update/backend/pkg/db"
)

// Prefix distinguishes API tokens from session tokens in the auth middleware.
const Prefix = "uat_"

type Token struct {
	ID         int32      `json:"id" db:"id"`
	Name       string     `json:"name" db:"name"`
	Role       string     `json:"role" db:"role"`
	CreatedBy  string     `json:"created_by" db:"created_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at" db:"last_used_at"`
}

const cols = `id, name, role, created_by, created_at, last_used_at`

func hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Create mints a token and returns the row plus the raw secret — the only
// time it is ever available.
func Create(ctx context.Context, dbx db.DBTX, name, role, createdBy string) (Token, string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return Token{}, "", err
	}
	raw := Prefix + hex.EncodeToString(buf)
	rows, err := dbx.Query(ctx, `
		INSERT INTO api_tokens (name, token_hash, role, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+cols,
		name, hash(raw), role, createdBy)
	if err != nil {
		return Token{}, "", err
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Token])
	if err != nil {
		return Token{}, "", err
	}
	return t, raw, nil
}

func List(ctx context.Context, dbx db.DBTX) ([]Token, error) {
	rows, err := dbx.Query(ctx, `SELECT `+cols+` FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	toks, err := pgx.CollectRows(rows, pgx.RowToStructByName[Token])
	if err != nil {
		return nil, err
	}
	if toks == nil {
		toks = []Token{}
	}
	return toks, nil
}

func Delete(ctx context.Context, dbx db.DBTX, id int32) (int64, error) {
	tag, err := dbx.Exec(ctx, `DELETE FROM api_tokens WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Validate resolves a presented raw token. It also bumps last_used_at in the
// same statement — one round trip, and the ledger shows dormant tokens.
func Validate(ctx context.Context, dbx db.DBTX, raw string) (Token, bool, error) {
	if !strings.HasPrefix(raw, Prefix) {
		return Token{}, false, nil
	}
	rows, err := dbx.Query(ctx, `
		UPDATE api_tokens SET last_used_at = NOW()
		WHERE token_hash = $1
		RETURNING `+cols,
		hash(raw))
	if err != nil {
		return Token{}, false, err
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Token])
	if err != nil {
		if err == pgx.ErrNoRows {
			return Token{}, false, nil
		}
		return Token{}, false, err
	}
	return t, true, nil
}
