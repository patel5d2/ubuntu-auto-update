package config

import (
	"os"
	"testing"
	"time"
)

func TestGetEnvString(t *testing.T) {
	os.Setenv("TEST_STR", "hello")
	defer os.Unsetenv("TEST_STR")
	if v := getEnvString("TEST_STR", "default"); v != "hello" {
		t.Errorf("got %q, want hello", v)
	}
	if v := getEnvString("MISSING_KEY", "default"); v != "default" {
		t.Errorf("got %q, want default", v)
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")
	if v := getEnvInt("TEST_INT", 0); v != 42 {
		t.Errorf("got %d, want 42", v)
	}
	if v := getEnvInt("MISSING_INT", 99); v != 99 {
		t.Errorf("got %d, want 99", v)
	}
	os.Setenv("TEST_INT_BAD", "notanumber")
	defer os.Unsetenv("TEST_INT_BAD")
	if v := getEnvInt("TEST_INT_BAD", 10); v != 10 {
		t.Errorf("got %d, want 10 for invalid int", v)
	}
}

func TestGetEnvBool(t *testing.T) {
	os.Setenv("TEST_BOOL_T", "true")
	os.Setenv("TEST_BOOL_F", "false")
	os.Setenv("TEST_BOOL_BAD", "maybe")
	defer func() {
		os.Unsetenv("TEST_BOOL_T")
		os.Unsetenv("TEST_BOOL_F")
		os.Unsetenv("TEST_BOOL_BAD")
	}()
	if v := getEnvBool("TEST_BOOL_T", false); !v {
		t.Error("expected true")
	}
	if v := getEnvBool("TEST_BOOL_F", true); v {
		t.Error("expected false")
	}
	if v := getEnvBool("TEST_BOOL_BAD", true); !v {
		t.Error("expected default true for invalid bool")
	}
	if v := getEnvBool("MISSING_BOOL", true); !v {
		t.Error("expected default true")
	}
}

func TestGetEnvDuration(t *testing.T) {
	os.Setenv("TEST_DUR", "5s")
	defer os.Unsetenv("TEST_DUR")
	if v := getEnvDuration("TEST_DUR", time.Minute); v != 5*time.Second {
		t.Errorf("got %v, want 5s", v)
	}
	if v := getEnvDuration("MISSING_DUR", time.Minute); v != time.Minute {
		t.Errorf("got %v, want 1m", v)
	}
}

func TestGetEnvStringSlice(t *testing.T) {
	os.Setenv("TEST_SLICE", "a,b,c")
	defer os.Unsetenv("TEST_SLICE")
	v := getEnvStringSlice("TEST_SLICE", []string{"x"})
	if len(v) != 3 || v[0] != "a" || v[1] != "b" || v[2] != "c" {
		t.Errorf("got %v, want [a b c]", v)
	}
	v = getEnvStringSlice("MISSING_SLICE", []string{"x"})
	if len(v) != 1 || v[0] != "x" {
		t.Errorf("got %v, want [x]", v)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	defer os.Unsetenv("DATABASE_URL")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port should be 8080, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("default log level should be info, got %s", cfg.Logging.Level)
	}
}

func TestValidate_MissingDatabaseURL(t *testing.T) {
	cfg := &Config{Database: DatabaseConfig{URL: ""}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for missing DATABASE_URL")
	}
}

func TestValidate_JWTSecretRequiredInProd(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{URL: "postgres://localhost/test"},
		Server:   ServerConfig{Environment: "production"},
		Auth:     AuthConfig{JWTSecret: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing JWT_SECRET in production")
	}
}

func TestValidate_HTTPSRequiresCerts(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{URL: "postgres://localhost/test"},
		Security: SecurityConfig{EnableHTTPS: true, TLSCertFile: "", TLSKeyFile: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for HTTPS without certs")
	}
}
