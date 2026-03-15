package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

func Token(key, sessionToken string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(sessionToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func NewCSRFMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !requiresCSRF(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			sessionCookie, err := r.Cookie(auth.SessionCookieName)
			if err != nil || sessionCookie.Value == "" {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			provided := requestToken(r)
			if provided == "" {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			expected := Token(key, sessionCookie.Value)
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func requiresCSRF(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func requestToken(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get("X-CSRF-Token")); token != "" {
		return token
	}
	if err := r.ParseForm(); err == nil {
		return strings.TrimSpace(r.PostForm.Get("csrf_token"))
	}
	return ""
}
