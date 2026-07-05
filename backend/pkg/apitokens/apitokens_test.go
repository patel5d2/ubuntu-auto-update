package apitokens_test

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"ubuntu-auto-update/backend/pkg/apitokens"
)

func nowT() time.Time { return time.Now() }

func rows(mock pgxmock.PgxPoolIface) *pgxmock.Rows {
	return mock.NewRows([]string{"id", "name", "role", "created_by", "created_at", "last_used_at"})
}

func TestCreateThenValidateRoundTrip(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	mock.ExpectQuery(`INSERT INTO api_tokens`).
		WithArgs("ci", pgxmock.AnyArg(), "operator", "admin").
		WillReturnRows(rows(mock).AddRow(int32(1), "ci", "operator", "admin", nowT(), nil))

	tok, raw, err := apitokens.Create(context.Background(), mock, "ci", "operator", "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tok.Name != "ci" || raw == "" {
		t.Fatalf("unexpected create result: %+v / %q", tok, raw)
	}
	if len(raw) < 20 || raw[:4] != apitokens.Prefix {
		t.Fatalf("raw token %q must carry the %q prefix", raw, apitokens.Prefix)
	}
	// Validate: same raw token hashes to the same lookup key and bumps
	// last_used_at in one statement.
	mock.ExpectQuery(`UPDATE api_tokens SET last_used_at = NOW\(\)`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(rows(mock).AddRow(int32(1), "ci", "operator", "admin", nowT(), nil))

	got, ok, err := apitokens.Validate(context.Background(), mock, raw)
	if err != nil || !ok {
		t.Fatalf("validate: ok=%v err=%v", ok, err)
	}
	if got.Role != "operator" {
		t.Errorf("role = %q", got.Role)
	}

	// A non-prefixed token is rejected without touching the DB.
	if _, ok, _ := apitokens.Validate(context.Background(), mock, "session-token"); ok {
		t.Error("non-uat_ token must not validate")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestValidateUnknownToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	mock.ExpectQuery(`UPDATE api_tokens SET last_used_at = NOW\(\)`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(rows(mock)) // zero rows

	_, ok, err := apitokens.Validate(context.Background(), mock, "uat_deadbeef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("unknown token must not validate")
	}
}
