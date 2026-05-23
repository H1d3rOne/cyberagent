package daemon

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func bearerAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearerToken(r.Header.Get("Authorization"), token) {
			writeServiceError(w, services.NewError(services.CodeUnauthorized, "missing or invalid bearer token", nil, nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validBearerToken(header, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if got == "" {
		return false
	}
	if len(got) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
