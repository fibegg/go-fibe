package security

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fibegg/go-fibe/internal/config"
)

func TestNormalizeMonitorURLRejectsPrivateAndCredentialedURLs(t *testing.T) {
	cfg := config.Config{}
	if _, err := NormalizeMonitorURL(cfg, "http://127.0.0.1:3000/up"); err == nil {
		t.Fatal("expected loopback URL to be rejected")
	}
	if _, err := NormalizeMonitorURL(cfg, "https://user:pass@example.com"); err == nil {
		t.Fatal("expected credentialed URL to be rejected")
	}
}

func TestNormalizeMonitorURLAllowsPrivateWhenConfigured(t *testing.T) {
	cfg := config.Config{AllowPrivateMonitorURLs: true}
	if _, err := NormalizeMonitorURL(cfg, "http://127.0.0.1:3000/up"); err != nil {
		t.Fatalf("NormalizeMonitorURL() error = %v", err)
	}
}

func TestCSPValueUsesFrameAncestorsAndConnectSrc(t *testing.T) {
	cfg := config.Config{
		CSPMode:        config.CSPModeEnforce,
		FrameAncestors: "'self' https://*.example.com",
		CSPConnectSrc:  "'self' https://api.example.com",
	}
	value := CSPValue(cfg)
	if !containsAll(value, "frame-ancestors 'self' https://*.example.com", "connect-src 'self' https://api.example.com") {
		t.Fatalf("CSPValue() = %q", value)
	}
}

func TestRequestIsHTTPSHonorsTrustedProxyOnly(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if RequestIsHTTPS(req, false) {
		t.Fatal("untrusted proxy header should not mark request HTTPS")
	}
	if !RequestIsHTTPS(req, true) {
		t.Fatal("trusted proxy header should mark request HTTPS")
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
