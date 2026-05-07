package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

type CSPMode string

const (
	CSPModeOff        CSPMode = "off"
	CSPModeReportOnly CSPMode = "report-only"
	CSPModeEnforce    CSPMode = "enforce"
)

type CookieSecure string

const (
	CookieSecureAuto   CookieSecure = "auto"
	CookieSecureAlways CookieSecure = "true"
	CookieSecureNever  CookieSecure = "false"
)

type Config struct {
	Env                      string
	Port                     int
	Secret                   string
	DatabaseURL              string
	DBMaxConnections         int32
	DBMinConnections         int32
	DBAcquireTimeout         time.Duration
	RedisURL                 string
	AllowedHosts             []string
	CORSAllowedOrigins       []string
	PublicOrigin             string
	TrustProxyHeaders        bool
	ForceHTTPS               bool
	AssumeHTTPS              bool
	HSTS                     bool
	FrameAncestors           string
	XFrameOptions            string
	CSPMode                  CSPMode
	CSPConnectSrc            string
	CookieSecure             CookieSecure
	CookieSameSite           string
	DemoPassword             string
	AllowPrivateMonitorURLs  bool
	WorkerConcurrency        int
	MonitorCheckInterval     time.Duration
	MonitorHTTPTimeout       time.Duration
	RateLimitAuthPerMinute   int
	RateLimitGlobalPerMinute int
}

func FromEnv() (Config, error) {
	env := envOr("APP_ENV", "development")
	cspDefault := "off"
	if env == "production" {
		cspDefault = "report-only"
	}

	cfg := Config{
		Env:                      env,
		Port:                     intEnv("APP_PORT", 3000),
		Secret:                   envOr("APP_SECRET", "dev-secret-change-me"),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		DBMaxConnections:         int32(intEnv("DB_MAX_CONNECTIONS", 5)),
		DBMinConnections:         int32(intEnv("DB_MIN_CONNECTIONS", 0)),
		DBAcquireTimeout:         time.Duration(intEnv("DB_ACQUIRE_TIMEOUT_MS", 1500)) * time.Millisecond,
		RedisURL:                 os.Getenv("REDIS_URL"),
		AllowedHosts:             listEnv("APP_ALLOWED_HOSTS", "*"),
		CORSAllowedOrigins:       listEnv("APP_CORS_ALLOWED_ORIGINS", "*"),
		PublicOrigin:             strings.TrimSpace(os.Getenv("APP_PUBLIC_ORIGIN")),
		TrustProxyHeaders:        boolEnv("APP_TRUST_PROXY_HEADERS", true),
		ForceHTTPS:               boolEnv("APP_FORCE_HTTPS", false),
		AssumeHTTPS:              boolEnv("APP_ASSUME_HTTPS", false),
		HSTS:                     boolEnv("APP_HSTS", false),
		FrameAncestors:           strings.TrimSpace(os.Getenv("APP_FRAME_ANCESTORS")),
		XFrameOptions:            strings.TrimSpace(os.Getenv("APP_X_FRAME_OPTIONS")),
		CSPMode:                  CSPMode(envOr("APP_CSP_MODE", cspDefault)),
		CSPConnectSrc:            strings.TrimSpace(os.Getenv("APP_CSP_CONNECT_SRC")),
		CookieSecure:             CookieSecure(envOr("APP_COOKIE_SECURE", "auto")),
		CookieSameSite:           envOr("APP_COOKIE_SAMESITE", "lax"),
		DemoPassword:             envOr("PUBLIC_DEMO_PASSWORD", "password"),
		AllowPrivateMonitorURLs:  boolEnv("APP_ALLOW_PRIVATE_MONITOR_URLS", false),
		WorkerConcurrency:        intEnv("WORKER_CONCURRENCY", 2),
		MonitorCheckInterval:     time.Duration(intEnv("MONITOR_CHECK_INTERVAL_SECONDS", 30)) * time.Second,
		MonitorHTTPTimeout:       time.Duration(intEnv("MONITOR_HTTP_TIMEOUT_SECONDS", 8)) * time.Second,
		RateLimitAuthPerMinute:   intEnv("RATE_LIMIT_AUTH_PER_MINUTE", 30),
		RateLimitGlobalPerMinute: intEnv("RATE_LIMIT_GLOBAL_PER_MINUTE", 240),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return Config{}, errors.New("REDIS_URL is required")
	}
	switch cfg.CSPMode {
	case CSPModeOff, CSPModeReportOnly, CSPModeEnforce:
	default:
		return Config{}, fmt.Errorf("APP_CSP_MODE must be off, report-only, or enforce")
	}
	switch cfg.CookieSecure {
	case CookieSecureAuto, CookieSecureAlways, CookieSecureNever:
	default:
		return Config{}, fmt.Errorf("APP_COOKIE_SECURE must be auto, true, or false")
	}
	return cfg, nil
}

func (c Config) BindAddr() string {
	return fmt.Sprintf("0.0.0.0:%d", c.Port)
}

func (c Config) IsDevelopment() bool {
	return c.Env != "production"
}

func (c Config) AllowsAllHosts() bool {
	return includes(c.AllowedHosts, "*")
}

func (c Config) AllowsAllOrigins() bool {
	return includes(c.CORSAllowedOrigins, "*")
}

func (c Config) CookieSecureEnabled(requestIsHTTPS bool) bool {
	switch c.CookieSecure {
	case CookieSecureAlways:
		return true
	case CookieSecureNever:
		return false
	default:
		return requestIsHTTPS || c.AssumeHTTPS || c.ForceHTTPS
	}
}

func (c Config) RedisAsynqOpt() (asynq.RedisClientOpt, error) {
	u, err := url.Parse(c.RedisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, err
	}
	db := 0
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		db, err = strconv.Atoi(path)
		if err != nil {
			return asynq.RedisClientOpt{}, err
		}
	}
	password, _ := u.User.Password()
	host := u.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "6379")
	}
	return asynq.RedisClientOpt{Addr: host, Username: u.User.Username(), Password: password, DB: db}, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func listEnv(key, fallback string) []string {
	raw := envOr(key, fallback)
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func includes(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
