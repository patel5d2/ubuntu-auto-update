package users

import "testing"

func TestIsValidRole(t *testing.T) {
	for _, role := range []string{"viewer", "operator", "admin"} {
		if !IsValidRole(role) {
			t.Errorf("expected %q to be valid", role)
		}
	}
	for _, role := range []string{"", "root", "user", "Admin"} {
		if IsValidRole(role) {
			t.Errorf("expected %q to be invalid", role)
		}
	}
}

func TestHashPassword_TooShort(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordTooShort {
		t.Errorf("got %v, want ErrPasswordTooShort", err)
	}
}

func TestHashPassword_Roundtrip(t *testing.T) {
	hash, err := HashPassword("a-very-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" || len(hash) < 50 {
		t.Errorf("expected bcrypt hash, got %q", hash)
	}
}
