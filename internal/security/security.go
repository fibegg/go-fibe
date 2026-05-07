package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fibegg/go-fibe/internal/app"
	"github.com/fibegg/go-fibe/internal/config"
)

var ErrUnsafeMonitorURL = errors.New("monitor URL is not allowed")

func HostGuard(rt *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rt.Config.AllowsAllHosts() {
				host := r.Host
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				if !hostAllowed(host, rt.Config.AllowedHosts) {
					http.Error(w, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeaders(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if cfg.HSTS {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			if cfg.XFrameOptions != "" {
				h.Set("X-Frame-Options", cfg.XFrameOptions)
			}
			if csp := CSPValue(cfg); csp != "" {
				if cfg.CSPMode == config.CSPModeReportOnly {
					h.Set("Content-Security-Policy-Report-Only", csp)
				}
				if cfg.CSPMode == config.CSPModeEnforce {
					h.Set("Content-Security-Policy", csp)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimit(rt *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/up" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/assets/") {
				next.ServeHTTP(w, r)
				return
			}

			limit := rt.Config.RateLimitGlobalPerMinute
			if strings.HasPrefix(r.URL.Path, "/auth/") {
				limit = rt.Config.RateLimitAuthPerMinute
			}
			key := fmt.Sprintf("rate:%s:%d", rateLimitKey(r, rt.Config.TrustProxyHeaders), time.Now().Unix()/60)
			count, err := rt.Redis.Incr(r.Context(), key).Result()
			if err != nil {
				slog.Warn("rate limiter failed", "error", err)
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
				return
			}
			if count == 1 {
				_ = rt.Redis.Expire(r.Context(), key, 75*time.Second).Err()
			}
			if count > int64(limit) {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CSPValue(cfg config.Config) string {
	if cfg.CSPMode == config.CSPModeOff {
		return ""
	}
	frameAncestors := cfg.FrameAncestors
	if frameAncestors == "" {
		frameAncestors = "*"
	}
	connectSrc := cfg.CSPConnectSrc
	if connectSrc == "" && cfg.PublicOrigin != "" {
		connectSrc = "'self' " + cfg.PublicOrigin
	}
	if connectSrc == "" {
		connectSrc = "'self' http: https: ws: wss:"
	}
	return fmt.Sprintf("default-src 'self'; base-uri 'self'; frame-ancestors %s; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src %s", frameAncestors, connectSrc)
}

func NormalizeMonitorURL(cfg config.Config, raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", errors.New("monitor URL must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("monitor URL must use http or https")
	}
	if parsed.User != nil {
		return "", errors.New("monitor URL must not include credentials")
	}
	if !cfg.AllowPrivateMonitorURLs && hostIsPrivateOrLocal(parsed.Hostname()) {
		return "", errors.New("monitor URL must target a public host")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func EnsureMonitorURLAllowed(ctx context.Context, cfg config.Config, raw string) error {
	normalized, err := NormalizeMonitorURL(cfg, raw)
	if err != nil {
		return err
	}
	if cfg.AllowPrivateMonitorURLs {
		return nil
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return ErrUnsafeMonitorURL
	}
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return ErrUnsafeMonitorURL
		}
	}
	resolver := net.Resolver{}
	addrs, err := resolver.LookupNetIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addrs) == 0 {
		return ErrUnsafeMonitorURL
	}
	for _, addr := range addrs {
		if ipIsPrivateOrLocal(addr) {
			return ErrUnsafeMonitorURL
		}
	}
	_, err = strconv.Atoi(port)
	if err != nil {
		return ErrUnsafeMonitorURL
	}
	return nil
}

func RequestIsHTTPS(r *http.Request, trustProxy bool) bool {
	if r.TLS != nil {
		return true
	}
	return trustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func rateLimitKey(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			return strings.ReplaceAll(forwarded, ":", "_")
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return strings.ReplaceAll(host, ":", "_")
	}
	return "local"
}

func hostAllowed(host string, allowed []string) bool {
	for _, item := range allowed {
		if strings.EqualFold(host, item) {
			return true
		}
	}
	return false
}

func hostIsPrivateOrLocal(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return ipIsPrivateOrLocal(addr)
	}
	return false
}

func ipIsPrivateOrLocal(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified()
}
