package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const authCookie = "gojira"

// loadOrCreateToken returns the local access token, creating it on first run.
// The file is mode 0600, so only processes running as this user (or with the
// home directory mounted) can read it; that is the whole access-control model.
func loadOrCreateToken(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		return strings.TrimSpace(string(b)), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	return tok, os.WriteFile(path, []byte(tok+"\n"), 0o600)
}

func defaultTokenPath() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "gojira", "token")
	}
	return ".gojira-token"
}

// requireAuth rejects requests that do not carry the token cookie. Browsers
// pick the cookie up once by visiting /auth/<token>; other local processes
// cannot forge it without reading the token file.
func requireAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(authCookie); err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		// go tool pprof cannot send cookies, so profiling routes also accept
		// the token as a query parameter.
		if strings.HasPrefix(r.URL.Path, "/debug/pprof/") && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "gojira: not authorized. Run the launcher, or open /auth/<token> from the token file.", http.StatusUnauthorized)
	})
}

// authHandler sets the cookie when given the right token, then sends the
// browser to the requested page (or the index).
func authHandler(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.PathValue("token")), []byte(token)) != 1 {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: authCookie, Value: token, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteLaxMode,
			Expires: time.Now().Add(365 * 24 * time.Hour),
		})
		next := r.URL.Query().Get("next")
		if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
			next = "/"
		}
		http.Redirect(w, r, next, http.StatusFound)
	}
}
