package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandlerCSRF() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCSRF_GETPasses(t *testing.T) {
	h := CSRFMiddleware("auth_token")(okHandlerCSRF())
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Errorf("GET should pass CSRF, got %d", rw.Code)
	}
}

func TestCSRF_BearerBypasses(t *testing.T) {
	h := CSRFMiddleware("auth_token")(okHandlerCSRF())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Authorization", "Bearer abc")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Errorf("Bearer auth should bypass CSRF, got %d", rw.Code)
	}
}

func TestCSRF_CookieMissingHeader(t *testing.T) {
	h := CSRFMiddleware("auth_token")(okHandlerCSRF())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "abc"})
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusForbidden {
		t.Errorf("missing header should 403, got %d", rw.Code)
	}
}

func TestCSRF_HeaderMismatch(t *testing.T) {
	h := CSRFMiddleware("auth_token")(okHandlerCSRF())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "abc"})
	r.Header.Set(CSRFHeader, "xyz")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusForbidden {
		t.Errorf("mismatched token should 403, got %d", rw.Code)
	}
}

func TestCSRF_HappyPath(t *testing.T) {
	h := CSRFMiddleware("auth_token")(okHandlerCSRF())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "abc"})
	r.Header.Set(CSRFHeader, "abc")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Errorf("matching token should pass, got %d", rw.Code)
	}
}

func TestGenerateCSRFTokenUnique(t *testing.T) {
	a, _ := GenerateCSRFToken()
	b, _ := GenerateCSRFToken()
	if a == b || len(a) != 48 {
		t.Errorf("bad CSRF tokens %s %s", a, b)
	}
}
