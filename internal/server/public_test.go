package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicHandlerRequiresDashboardAuthentication(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", "secret-password", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test/", nil)
	req.Host = "lab.example.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Fatalf("status=%d", rr.Code) }
	if rr.Header().Get("WWW-Authenticate") == "" { t.Fatal("expected Basic auth challenge") }
}

func TestPublicHandlerAcceptsAuthenticatedDashboardRequest(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", "secret-password", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test/", nil)
	req.Host = "lab.example.test"
	req.SetBasicAuth("shiftmy", "secret-password")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent { t.Fatalf("status=%d", rr.Code) }
}

func TestPublicHandlerAllowsTokenizedDoHWithoutDashboardAuthentication(t *testing.T) {
	dashboard := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("dashboard handler called for DoH") })
	dohHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", "secret-password", dashboard, dohHandler)
	req := httptest.NewRequest(http.MethodPost, "https://lab.example.test/dns-query/token-1", nil)
	req.Host = "lab.example.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent { t.Fatalf("status=%d", rr.Code) }
}

func TestPublicHandlerRejectsWrongDashboardPassword(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", "secret-password", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test/api/status", nil)
	req.Host = "lab.example.test"
	req.SetBasicAuth("shiftmy", "wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized { t.Fatalf("status=%d", rr.Code) }
}

func TestPublicHandlerRejectsUnexpectedHost(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", "secret-password", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://evil.example/", nil)
	req.Host = "evil.example"
	req.SetBasicAuth("shiftmy", "secret-password")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMisdirectedRequest { t.Fatalf("status=%d", rr.Code) }
}

func TestPublicHandlerAcceptsConfiguredHostWithPort(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", "secret-password", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test:443/", nil)
	req.Host = "lab.example.test:443"
	req.SetBasicAuth("shiftmy", "secret-password")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent { t.Fatalf("status=%d", rr.Code) }
}
