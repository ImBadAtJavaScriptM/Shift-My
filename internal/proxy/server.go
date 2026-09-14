package proxy

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/netpolicy"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

// Server is the controlled TLS lab service. It accepts only hostnames owned by
// this project and never forwards requests to third-party services.
type Server struct {
	authority *pki.Authority
	store     *storage.Store
	policy    netpolicy.Policy
}

func New(authority *pki.Authority, store *storage.Store, policy netpolicy.Policy) *Server {
	return &Server{authority: authority, store: store, policy: policy}
}

func (s *Server) CertificateForHost(host string) (*tls.Certificate, error) {
	host = normalizeHost(host)
	if !s.policy.AllowedHost(host) {
		return nil, fmt.Errorf("controlled hostname %q is not allowed", host)
	}
	cert, err := s.authority.MintServerCertificate(host)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := normalizeHost(r.Host)
	if !s.policy.AllowedHost(host) {
		http.Error(w, "controlled host required", http.StatusMisdirectedRequest)
		return
	}
	if r.Method != http.MethodGet || r.URL.Path != "/v1/location" {
		http.NotFound(w, r)
		return
	}

	inst, err := s.store.Installation()
	if err != nil {
		http.Error(w, "read installation", http.StatusInternalServerError)
		return
	}
	if inst.SelectedLatitude == nil || inst.SelectedLongitude == nil {
		http.Error(w, "no target location selected", http.StatusConflict)
		return
	}
	if err := s.store.MarkProxySeen(time.Now().UTC()); err != nil {
		http.Error(w, "record lab TLS activity", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Label     string  `json:"label"`
		Revision  int64   `json:"revision"`
	}{
		Latitude:  *inst.SelectedLatitude,
		Longitude: *inst.SelectedLongitude,
		Label:     inst.SelectedLabel,
		Revision:  inst.LocationRevision,
	})
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
