package appauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fibegg/go-fibe/internal/app"
	"github.com/fibegg/go-fibe/internal/config"
)

func TestSessionCookieValueUsesEmbeddedCookieAttributes(t *testing.T) {
	rt := &app.App{Config: config.Config{
		CookieSecure:      config.CookieSecureAlways,
		CookieSameSite:    "none",
		CookiePartitioned: true,
	}}
	req := httptest.NewRequest(http.MethodPost, "https://app.example.test/auth/login", nil)

	cookie := sessionCookieValue(rt, req, "token")

	if !cookie.Secure {
		t.Fatal("expected secure cookie")
	}
	if cookie.SameSite != http.SameSiteNoneMode {
		t.Fatalf("SameSite = %v, want None", cookie.SameSite)
	}
	if !cookie.Partitioned {
		t.Fatal("expected partitioned cookie")
	}
	if !strings.Contains(cookie.String(), "Partitioned") {
		t.Fatalf("cookie header = %q, want Partitioned", cookie.String())
	}
}

func TestPartitionedSessionCookieForcesSecure(t *testing.T) {
	rt := &app.App{Config: config.Config{
		CookieSecure:      config.CookieSecureNever,
		CookieSameSite:    "none",
		CookiePartitioned: true,
	}}
	req := httptest.NewRequest(http.MethodPost, "http://app.example.test/auth/login", nil)

	cookie := sessionCookieValue(rt, req, "token")

	if !cookie.Secure {
		t.Fatal("partitioned cookies must be secure")
	}
}
