package proxy

import (
	"bytes"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/capture"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/netpolicy"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/wloc"
)

const capturedLegacyWLOCRequestHex = "00010005656e5f55530013636f6d2e6170706c652e6c6f636174696f6e64000a382e312e313242343131000000010000001912130a1133343a44423a46443a34333a45333a413118002001"

type recordingSpy struct {
	requests  int
	responses int
	lastHost  string
}

func (s *recordingSpy) RecordRequest(meta capture.Metadata, body []byte) error {
	s.requests++
	s.lastHost = meta.Host
	return nil
}

func (s *recordingSpy) RecordResponse(meta capture.Metadata, body []byte) error {
	s.responses++
	s.lastHost = meta.Host
	return nil
}

func testServer(t *testing.T) (*Server, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	certPEM, keyPEM, err := pki.GenerateRoot("Shift-My Lab Root")
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	authority, err := pki.Load(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureInstallation("token-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTarget(40.758, -73.9855, "Times Square"); err != nil {
		t.Fatal(err)
	}
	policy, err := netpolicy.NewControlled("lab.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return New(authority, store, policy), store
}

func TestCertificateForHostRejectsOutsideControlledPolicy(t *testing.T) {
	srv, _ := testServer(t)
	if _, err := srv.CertificateForHost("example.com"); err == nil {
		t.Fatal("expected non-controlled hostname rejection")
	}
}

func TestCertificateForHostMintsSingleHostSAN(t *testing.T) {
	srv, _ := testServer(t)
	cert, err := srv.CertificateForHost("loc-a.lab.example.test")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "loc-a.lab.example.test" {
		t.Fatalf("SANs=%v", leaf.DNSNames)
	}
}

func TestControlledLocationEndpointReturnsStoredTargetAndMarksSeen(t *testing.T) {
	srv, store := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "https://loc-a.lab.example.test/v1/location", nil)
	req.Host = "loc-a.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Label     string  `json:"label"`
		Revision  int64   `json:"revision"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Latitude != 40.758 || got.Longitude != -73.9855 || got.Label != "Times Square" || got.Revision != 1 {
		t.Fatalf("response=%+v", got)
	}
	inst, err := store.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if inst.ProxySeenAt == nil {
		t.Fatal("expected proxy seen timestamp")
	}
}

func TestControlledLocationEndpointCanRecordLabRequestAndResponse(t *testing.T) {
	srv, _ := testServer(t)
	spy := &recordingSpy{}
	srv.recorder = spy
	req := httptest.NewRequest(http.MethodGet, "https://device-loc.lab.example.test/v1/location", nil)
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if spy.requests != 1 || spy.responses != 1 {
		t.Fatalf("requests=%d responses=%d", spy.requests, spy.responses)
	}
	if spy.lastHost != "device-loc.lab.example.test" {
		t.Fatalf("host=%q", spy.lastHost)
	}
}

func TestControlledLocationEndpointRejectsWrongHost(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/v1/location", nil)
	req.Host = "example.com"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusMisdirectedRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestControlledWLOCEmulatorReturnsSelectedTarget(t *testing.T) {
	srv, store := testServer(t)
	body, err := hex.DecodeString(capturedLegacyWLOCRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/clls/wloc", bytes.NewReader(body))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content-type=%q", got)
	}
	if got := rr.Header().Get("X-Shift-My-Lab"); got != "controlled-wloc-emulator" {
		t.Fatalf("lab marker=%q", got)
	}
	version, functionID, devices, err := wloc.ParseResponse(rr.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || functionID != 1 || len(devices) != 1 {
		t.Fatalf("version=%d function=%d devices=%+v", version, functionID, devices)
	}
	if devices[0].BSSID != "34:DB:FD:43:E3:A1" {
		t.Fatalf("bssid=%q", devices[0].BSSID)
	}
	if devices[0].LatitudeE8 != int64(math.Round(40.758*1e8)) || devices[0].LongitudeE8 != int64(math.Round(-73.9855*1e8)) {
		t.Fatalf("location=%+v", devices[0])
	}
	inst, err := store.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if inst.ProxySeenAt == nil {
		t.Fatal("expected proxy seen timestamp")
	}
}

func TestControlledWLOCEmulatorRejectsMalformedBody(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/clls/wloc", bytes.NewReader(nil))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "Bad Request" {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestControlledWLOCEmulatorUsesExactPath(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/clls/wloc/", bytes.NewReader(nil))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}
