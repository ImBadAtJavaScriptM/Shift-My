package server

import (
	"net"
	"net/http"
	"strings"
)

// NewPublic combines the dashboard/profile UI with the controlled DoH endpoint
// and rejects requests whose Host header is not the configured public hostname.
func NewPublic(publicHost string, dashboardHandler, dohHandler http.Handler) http.Handler {
	publicHost = normalizeHost(publicHost)
	mux := http.NewServeMux()
	mux.Handle("/dns-query/", dohHandler)
	mux.Handle("/", dashboardHandler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if normalizeHost(r.Host) != publicHost {
			http.Error(w, "configured public host required", http.StatusMisdirectedRequest)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
