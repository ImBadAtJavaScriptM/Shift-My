package doh

import (
	"bytes"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/netpolicy"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	"github.com/miekg/dns"
)

func newTestDoH(t *testing.T, publicIP string) (*storage.Store, http.Handler) {
	t.Helper()
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureInstallation("token-1"); err != nil { t.Fatal(err) }
	policy, err := netpolicy.NewControlled("lab.example.test")
	if err != nil { t.Fatal(err) }
	return store, New(store, net.ParseIP(publicIP), policy)
}

func wireQuery(t *testing.T, name string, qtype uint16) []byte {
	t.Helper()
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	wire, err := msg.Pack()
	if err != nil { t.Fatal(err) }
	return wire
}

func decodeDNSResponse(t *testing.T, rr *httptest.ResponseRecorder) *dns.Msg {
	t.Helper()
	if rr.Code != http.StatusOK { t.Fatalf("http status=%d body=%s", rr.Code, rr.Body.String()) }
	msg := new(dns.Msg)
	if err := msg.Unpack(rr.Body.Bytes()); err != nil { t.Fatal(err) }
	return msg
}

func TestPOSTAllowedAQueryReturnsExperimentIPv4AndMarksSeen(t *testing.T) {
	store, h := newTestDoH(t, "203.0.113.10")
	wire := wireQuery(t, "loc-a.lab.example.test", dns.TypeA)
	req := httptest.NewRequest(http.MethodPost, "/dns-query/token-1", bytes.NewReader(wire))
	req.Header.Set("Content-Type", "application/dns-message")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	msg := decodeDNSResponse(t, rr)
	if msg.Rcode != dns.RcodeSuccess || len(msg.Answer) != 1 { t.Fatalf("rcode=%d answers=%v", msg.Rcode, msg.Answer) }
	a, ok := msg.Answer[0].(*dns.A)
	if !ok || !a.A.Equal(net.ParseIP("203.0.113.10")) { t.Fatalf("answer=%v", msg.Answer[0]) }
	inst, err := store.Installation()
	if err != nil { t.Fatal(err) }
	if inst.DoHSeenAt == nil { t.Fatal("expected DoH seen timestamp") }
}

func TestAllowedAAAAQueryReturnsExperimentIPv6AndMarksSeen(t *testing.T) {
	store, h := newTestDoH(t, "2001:db8::10")
	wire := wireQuery(t, "loc-b.lab.example.test", dns.TypeAAAA)
	req := httptest.NewRequest(http.MethodPost, "/dns-query/token-1", bytes.NewReader(wire))
	req.Header.Set("Content-Type", "application/dns-message")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	msg := decodeDNSResponse(t, rr)
	if msg.Rcode != dns.RcodeSuccess || len(msg.Answer) != 1 { t.Fatalf("rcode=%d answers=%v", msg.Rcode, msg.Answer) }
	a, ok := msg.Answer[0].(*dns.AAAA)
	if !ok || !a.AAAA.Equal(net.ParseIP("2001:db8::10")) { t.Fatalf("answer=%v", msg.Answer[0]) }
	inst, err := store.Installation()
	if err != nil { t.Fatal(err) }
	if inst.DoHSeenAt == nil { t.Fatal("expected DoH seen timestamp") }
}

func TestAddressFamilyMismatchReturnsNODATA(t *testing.T) {
	_, h := newTestDoH(t, "2001:db8::10")
	wire := wireQuery(t, "loc-a.lab.example.test", dns.TypeA)
	req := httptest.NewRequest(http.MethodPost, "/dns-query/token-1", bytes.NewReader(wire))
	req.Header.Set("Content-Type", "application/dns-message")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	msg := decodeDNSResponse(t, rr)
	if msg.Rcode != dns.RcodeSuccess || len(msg.Answer) != 0 { t.Fatalf("rcode=%d answers=%v", msg.Rcode, msg.Answer) }
}

func TestNonAllowedHostIsRefused(t *testing.T) {
	_, h := newTestDoH(t, "2001:db8::10")
	wire := wireQuery(t, "example.com", dns.TypeAAAA)
	req := httptest.NewRequest(http.MethodPost, "/dns-query/token-1", bytes.NewReader(wire))
	req.Header.Set("Content-Type", "application/dns-message")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	msg := decodeDNSResponse(t, rr)
	if msg.Rcode != dns.RcodeRefused { t.Fatalf("rcode=%d", msg.Rcode) }
}

func TestInvalidTokenReturns404(t *testing.T) {
	_, h := newTestDoH(t, "2001:db8::10")
	req := httptest.NewRequest(http.MethodPost, "/dns-query/wrong-token", bytes.NewReader([]byte("not dns")))
	req.Header.Set("Content-Type", "application/dns-message")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound { t.Fatalf("status=%d", rr.Code) }
}

func TestGETDiagnosticQueryUsesBase64URL(t *testing.T) {
	_, h := newTestDoH(t, "2001:db8::10")
	wire := wireQuery(t, "device-loc.lab.example.test", dns.TypeAAAA)
	encoded := base64.RawURLEncoding.EncodeToString(wire)
	req := httptest.NewRequest(http.MethodGet, "/dns-query/token-1?dns="+encoded, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	msg := decodeDNSResponse(t, rr)
	if len(msg.Answer) != 1 { t.Fatalf("answers=%v", msg.Answer) }
}
