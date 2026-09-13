package server

import "net/http"

// NewPublic combines the dashboard/profile UI with the controlled DoH endpoint.
// More restrictive Host validation is added at the deployment edge in a later task.
func NewPublic(dashboardHandler, dohHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/dns-query/", dohHandler)
	mux.Handle("/", dashboardHandler)
	return mux
}
