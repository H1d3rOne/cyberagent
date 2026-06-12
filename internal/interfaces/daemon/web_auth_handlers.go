package daemon

import (
	"net/http"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

const webAuthCSRFHeaderName = "X-CyberAgent-CSRF"

func handleWebAuthSkeletonEndpoint(w http.ResponseWriter, r *http.Request, auth WebSessionAuthConfig, action string) {
	auth = normalizeWebSessionCookieResolverConfig(auth)
	endpoint := r.URL.Path
	if !auth.Enabled {
		writeServiceError(w, services.NewError(
			services.CodeNotFound,
			"web auth endpoint is disabled",
			webAuthSkeletonDetails(endpoint, action, auth, "disabled"),
			nil,
		))
		return
	}
	writeServiceError(w, services.NewError(
		services.CodeUnsupported,
		"web auth endpoint skeleton is not implemented",
		webAuthSkeletonDetails(endpoint, action, auth, "skeleton_not_implemented"),
		nil,
	))
}

func webAuthSkeletonDetails(endpoint, action string, auth WebSessionAuthConfig, status string) map[string]any {
	return map[string]any{
		"endpoint":                   endpoint,
		"action":                     action,
		"status":                     status,
		"web_sessions_enabled":       auth.Enabled,
		"cookie_name":                auth.CookieName,
		"cookie_http_only":           true,
		"cookie_same_site":           "strict",
		"cookie_secure_policy":       "secure_required_outside_loopback",
		"csrf_required":              true,
		"csrf_header":                webAuthCSRFHeaderName,
		"account_adapter_configured": false,
	}
}
