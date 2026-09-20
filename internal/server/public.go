package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// NewPublic combines the authenticated dashboard/profile UI with the tokenized
// DoH endpoint and rejects requests whose Host header is not the configured
// public hostname. DoH remains exempt from Basic auth because its path token is
// the resolver credential used by the installed profile.
func NewPublic(publicHost, adminPassword string, dashboardHandler, dohHandler, acmeHandler http.Handler) http.Handler {
	publicHost = normalizeHost(publicHost)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if normalizeHost(r.Host) != publicHost {
			http.Error(w, "configured public host required", http.StatusMisdirectedRequest)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/dns-query/") {
			dohHandler.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/acme/device/") {
			if acmeHandler == nil {
				http.NotFound(w, r)
				return
			}
			acmeHandler.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/profile/standard.mobileconfig" {
			// iOS retrieves Stage 2 automatically and cannot answer dashboard
			// Basic Auth. The dashboard handler validates its dedicated token.
			dashboardHandler.ServeHTTP(w, r)
			return
		}
		username, password, ok := r.BasicAuth()
		if !ok || !constantTimeEqual(username, "shiftmy") || !constantTimeEqual(password, adminPassword) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Shift-My Lab", charset="UTF-8"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		dashboardHandler.ServeHTTP(w, r)
	})
}

func constantTimeEqual(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
