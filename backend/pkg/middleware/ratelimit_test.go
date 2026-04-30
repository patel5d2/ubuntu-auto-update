package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRateLimiter_AllowsBurst(t *testing.T) {
	l := NewLoginRateLimiter()
	for i := 0; i < 5; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Error("6th attempt should be blocked")
	}
}

func TestLoginRateLimiter_PerKey(t *testing.T) {
	l := NewLoginRateLimiter()
	for i := 0; i < 5; i++ {
		l.Allow("a")
	}
	// Same key -> blocked, different key -> allowed.
	if l.Allow("a") {
		t.Error("key 'a' should be exhausted")
	}
	if !l.Allow("b") {
		t.Error("key 'b' should still be allowed")
	}
}

func TestIPAllowlist_Empty(t *testing.T) {
	a, err := NewIPAllowlist("")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Allow("1.2.3.4:1234") {
		t.Error("empty allowlist should allow all")
	}
}

func TestIPAllowlist_BareIPAndCIDR(t *testing.T) {
	a, err := NewIPAllowlist("10.0.0.0/8, 192.168.1.5")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		addr string
		want bool
	}{
		{"10.5.5.5:1234", true},
		{"192.168.1.5:1", true},
		{"192.168.1.6:1", false},
		{"8.8.8.8:1", false},
	}
	for _, c := range cases {
		if got := a.Allow(c.addr); got != c.want {
			t.Errorf("Allow(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func TestIPAllowlist_BadInput(t *testing.T) {
	if _, err := NewIPAllowlist("not-an-ip"); err == nil {
		t.Error("expected error for non-IP input")
	}
}

func TestIPAllowlistMiddleware_Blocks(t *testing.T) {
	a, _ := NewIPAllowlist("10.0.0.0/8")
	h := IPAllowlistMiddleware(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusForbidden {
		t.Errorf("blocked IP should get 403, got %d", rw.Code)
	}

	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.5.5.5:1234"
	rw2 := httptest.NewRecorder()
	h.ServeHTTP(rw2, r2)
	if rw2.Code != http.StatusOK {
		t.Errorf("allowed IP should pass, got %d", rw2.Code)
	}
}

// Behind a load balancer the proxy hits us with its own IP; the real client
// is in X-Forwarded-For. With TRUST_FORWARDED_FOR the middleware must use it.
func TestIPAllowlistMiddleware_HonorsXFF(t *testing.T) {
	a, _ := NewIPAllowlist("10.0.0.0/8")
	h := IPAllowlistMiddleware(a)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Setenv("TRUST_FORWARDED_FOR", "true")

	// Disallowed proxy, but the real client (XFF) is allowed.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "8.8.8.8:1"
	r.Header.Set("X-Forwarded-For", "10.0.0.5")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Errorf("XFF-trusted client in CIDR should pass, got %d", rw.Code)
	}

	// Allowed proxy, but the real client (XFF) isn't.
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.0.0.1:1"
	r2.Header.Set("X-Forwarded-For", "8.8.8.8")
	rw2 := httptest.NewRecorder()
	h.ServeHTTP(rw2, r2)
	if rw2.Code != http.StatusForbidden {
		t.Errorf("XFF-trusted client outside CIDR should be blocked, got %d", rw2.Code)
	}
}

func TestLoginRateLimiter_CleanIdleDropsOldBuckets(t *testing.T) {
	l := NewLoginRateLimiter()
	l.Allow("10.0.0.1")
	l.Allow("10.0.0.2")
	if got := len(l.buckets); got != 2 {
		t.Fatalf("expected 2 buckets, got %d", got)
	}
	// CleanIdle with a negative window deletes everything.
	l.CleanIdle(-time.Hour)
	if got := len(l.buckets); got != 0 {
		t.Errorf("expected 0 buckets after CleanIdle, got %d", got)
	}
}

func TestClientIP_DefaultStripsPort(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:12345"
	if ip := ClientIP(r); ip != "1.2.3.4" {
		t.Errorf("ClientIP = %s", ip)
	}
}

func TestClientIP_TrustsXFFOnlyWhenEnabled(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:1"
	r.Header.Set("X-Forwarded-For", "9.9.9.9, 5.5.5.5")

	// default off
	if ip := ClientIP(r); ip != "1.2.3.4" {
		t.Errorf("default should ignore XFF; got %s", ip)
	}

	// opt-in
	t.Setenv("TRUST_FORWARDED_FOR", "true")
	if ip := ClientIP(r); ip != "9.9.9.9" {
		t.Errorf("with TRUST_FORWARDED_FOR=true want 9.9.9.9, got %s", ip)
	}
}
