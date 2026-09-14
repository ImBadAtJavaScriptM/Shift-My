package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicHandlerAcceptsConfiguredHost(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test/", nil)
	req.Host = "lab.example.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent { t.Fatalf("status=%d", rr.Code) }
}

func TestPublicHandlerRejectsUnexpectedHost(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://evil.example/", nil)
	req.Host = "evil.example"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMisdirectedRequest { t.Fatalf("status=%d", rr.Code) }
}

func TestPublicHandlerAcceptsConfiguredHostWithPort(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewPublic("lab.example.test", ok, ok)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test:443/", nil)
	req.Host = "lab.example.test:443"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent { t.Fatalf("status=%d", rr.Code) }
}
