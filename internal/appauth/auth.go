package appauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/fibegg/go-fibe/internal/app"
	"github.com/fibegg/go-fibe/internal/models"
	"github.com/fibegg/go-fibe/internal/security"
)

const (
	sessionCookie     = "uptime_session"
	sessionTTLSeconds = 60 * 60 * 24 * 7
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	User *models.CurrentUser `json:"user"`
}

func Login(rt *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input loginRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		user, err := rt.Store.UserByEmail(r.Context(), input.Email)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		match, verifyErr := argon2id.ComparePasswordAndHash(input.Password, user.PasswordHash)
		if verifyErr != nil || !match {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		token, err := newSessionToken()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		current := currentUser(user)
		key := sessionKey(rt.Config.Secret, token)
		if err := rt.Redis.Set(r.Context(), key, user.ID, sessionTTLSeconds*time.Second).Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		http.SetCookie(w, sessionCookieValue(rt, r, token))
		writeJSON(w, http.StatusOK, sessionResponse{User: &current})
	}
}

func Logout(rt *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			_ = rt.Redis.Del(r.Context(), sessionKey(rt.Config.Secret, cookie.Value)).Err()
		}
		http.SetCookie(w, sessionCookieHeader(rt, r, "", -1))
		writeJSON(w, http.StatusOK, sessionResponse{})
	}
}

func Session(rt *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := CurrentUser(rt, r)
		if user.ID == "" {
			writeJSON(w, http.StatusOK, sessionResponse{})
			return
		}
		writeJSON(w, http.StatusOK, sessionResponse{User: &user})
	}
}

func CurrentUser(rt *app.App, r *http.Request) (models.CurrentUser, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return models.CurrentUser{}, false
	}
	userID, err := rt.Redis.Get(r.Context(), sessionKey(rt.Config.Secret, cookie.Value)).Result()
	if err != nil {
		return models.CurrentUser{}, false
	}
	user, err := rt.Store.UserByID(r.Context(), userID)
	if err != nil {
		return models.CurrentUser{}, false
	}
	return currentUser(user), true
}

func currentUser(user models.User) models.CurrentUser {
	return models.CurrentUser{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}
}

func sessionCookieValue(rt *app.App, r *http.Request, token string) *http.Cookie {
	return sessionCookieHeader(rt, r, token, sessionTTLSeconds)
}

func sessionCookieHeader(rt *app.App, r *http.Request, value string, maxAge int) *http.Cookie {
	secure := rt.Config.CookieSecureEnabled(security.RequestIsHTTPS(r, rt.Config.TrustProxyHeaders))
	if rt.Config.CookiePartitioned {
		secure = true
	}
	return &http.Cookie{
		Name:        sessionCookie,
		Value:       value,
		Path:        "/",
		HttpOnly:    true,
		Secure:      secure,
		SameSite:    sameSite(rt.Config.CookieSameSite),
		MaxAge:      maxAge,
		Partitioned: rt.Config.CookiePartitioned,
	}
}

func sameSite(raw string) http.SameSite {
	switch strings.ToLower(raw) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func sessionKey(secret, token string) string {
	return "session:" + stableSecretFingerprint(secret) + ":" + token
}

func stableSecretFingerprint(secret string) string {
	var hash uint64 = 14695981039346656037
	for _, b := range []byte(secret) {
		hash ^= uint64(b)
		hash *= 1099511628211
	}
	return base64.RawURLEncoding.EncodeToString([]byte{
		byte(hash >> 56), byte(hash >> 48), byte(hash >> 40), byte(hash >> 32),
		byte(hash >> 24), byte(hash >> 16), byte(hash >> 8), byte(hash),
	})
}

func newSessionToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
