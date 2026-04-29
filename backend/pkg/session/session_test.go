package session

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_CreateValidate(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	tok, err := s.Create(ctx, Principal{UserID: 1, Username: "alice", Role: RoleAdmin}, time.Hour, "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p, ok, err := s.Validate(ctx, tok)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !ok {
		t.Fatal("expected valid session")
	}
	if p.Username != "alice" || p.Role != RoleAdmin {
		t.Errorf("got principal %+v", p)
	}
}

func TestMemoryStore_Expired(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	tok, err := s.Create(ctx, Principal{UserID: 1, Username: "x", Role: RoleViewer}, time.Nanosecond, "", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	_, ok, _ := s.Validate(ctx, tok)
	if ok {
		t.Fatal("expected expired session to be invalid")
	}
}

func TestMemoryStore_Revoke(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	tok, _ := s.Create(ctx, Principal{UserID: 1, Username: "x", Role: RoleAdmin}, time.Hour, "", "")
	if err := s.Revoke(ctx, tok); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	_, ok, _ := s.Validate(ctx, tok)
	if ok {
		t.Fatal("expected revoked session to be invalid")
	}
}

func TestHasRole(t *testing.T) {
	cases := []struct {
		role, required string
		want           bool
	}{
		{RoleAdmin, RoleViewer, true},
		{RoleAdmin, RoleOperator, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleOperator, RoleViewer, true},
		{RoleOperator, RoleOperator, true},
		{RoleOperator, RoleAdmin, false},
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleOperator, false},
		{RoleViewer, RoleAdmin, false},
		{RoleAgent, RoleAgent, true},
		{RoleAgent, RoleViewer, false},
	}
	for _, c := range cases {
		got := Principal{Role: c.role}.HasRole(c.required)
		if got != c.want {
			t.Errorf("HasRole(role=%s, required=%s) = %v, want %v", c.role, c.required, got, c.want)
		}
	}
}

func TestGenerateTokenUnique(t *testing.T) {
	t1, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	t2, _ := GenerateToken()
	if t1 == t2 || len(t1) != 64 {
		t.Errorf("bad tokens: %s %s", t1, t2)
	}
}

func TestMemoryStore_CleanExpired(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	live, _ := s.Create(ctx, Principal{UserID: 1, Username: "live", Role: RoleAdmin}, time.Hour, "", "")
	expired, _ := s.Create(ctx, Principal{UserID: 2, Username: "exp", Role: RoleAdmin}, time.Nanosecond, "", "")
	time.Sleep(2 * time.Millisecond)

	if err := s.CleanExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Validate(ctx, live); !ok {
		t.Error("live session should remain")
	}
	if _, ok, _ := s.Validate(ctx, expired); ok {
		t.Error("expired session should be cleaned")
	}
}
