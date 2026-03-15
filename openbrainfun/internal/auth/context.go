package auth

import (
	"context"
	"net/http"
	"time"
)

const SessionCookieName = "openbrain_session"

type ContextUserKey struct{}

type ContextSessionTokenKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(ContextUserKey{}).(User)
	return user, ok
}

func SessionTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(ContextSessionTokenKey{}).(string)
	return token, ok
}

func NewSessionCookie(token string, expiresAt time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	}
}

func ExpiredSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	}
}
