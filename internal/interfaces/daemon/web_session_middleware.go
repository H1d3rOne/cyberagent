package daemon

import (
	"context"
	"net/http"
	"strings"
	"time"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

type webSessionContextKey struct{}

func webSessionFromContext(ctx context.Context) (coretenant.Session, bool) {
	if ctx == nil {
		return coretenant.Session{}, false
	}
	session, ok := ctx.Value(webSessionContextKey{}).(coretenant.Session)
	if !ok {
		return coretenant.Session{}, false
	}
	return session.Clone(), true
}

func withWebSessionContext(ctx context.Context, session coretenant.Session) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, webSessionContextKey{}, session.Clone())
}

func withWebSessionCookieResolver(auth WebSessionAuthConfig, store coretenant.SessionStore, next http.Handler) http.Handler {
	return withWebSessionCookieResolverClock(auth, store, time.Now, next)
}

func withWebSessionCookieResolverClock(auth WebSessionAuthConfig, store coretenant.SessionStore, now func() time.Time, next http.Handler) http.Handler {
	if next == nil {
		next = http.HandlerFunc(http.NotFound)
	}
	auth = normalizeWebSessionCookieResolverConfig(auth)
	if !auth.Enabled {
		return next
	}
	if store == nil {
		store = coretenant.DisabledSessionStore{}
	}
	if now == nil {
		now = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		sessionID := strings.TrimSpace(cookie.Value)
		if sessionID == "" {
			next.ServeHTTP(w, r)
			return
		}
		session, ok, err := store.GetSession(r.Context(), sessionID)
		if err != nil || !ok || !webSessionUsableAt(session, now().UTC()) {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(withWebSessionContext(r.Context(), session)))
	})
}

func normalizeWebSessionCookieResolverConfig(auth WebSessionAuthConfig) WebSessionAuthConfig {
	auth.CookieName = strings.TrimSpace(auth.CookieName)
	auth.StoragePath = strings.TrimSpace(auth.StoragePath)
	if !auth.Enabled {
		return auth
	}
	if auth.CookieName == "" {
		auth.CookieName = DefaultWebSessionCookieName
	}
	if auth.TTLSeconds <= 0 {
		auth.TTLSeconds = DefaultWebSessionTTLSeconds
	}
	return auth
}

func webSessionUsableAt(session coretenant.Session, now time.Time) bool {
	if session.AuthMethod != coretenant.AuthMethodWebSession {
		return false
	}
	return session.ActiveAt(now)
}
