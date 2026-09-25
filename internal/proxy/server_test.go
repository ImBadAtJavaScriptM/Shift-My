package proxy

import (
	"bytes"
	"crypto/x509"
	"encoding/binary"
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

func TestControlledWLOCModeQuerySelectsResponseVariant(t *testing.T) {
	srv, _ := testServer(t)

	payload := appendBytesFieldForProxyTest(nil, 2, appendBytesFieldForProxyTest(
		appendBytesFieldForProxyTest(nil, 1, []byte("aa:bb:cc:dd:ee:01")),
		2,
		appendVarintFieldForProxyTest(nil, 1, 1),
	))
	payload = appendVarintFieldForProxyTest(payload, 3, 7)
	payload = appendVarintFieldForProxyTest(payload, 4, -1)
	payload = appendBytesFieldForProxyTest(payload, 33, appendBytesFieldForProxyTest(nil, 1, []byte("N104AP")))

	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], 1)
	binary.BigEndian.PutUint32(frame[2:6], 1)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)

	for _, tc := range []struct {
		name       string
		query      string
		wantMode   string
		wantLength int
	}{
		{name: "preserve default", query: "", wantMode: "preserve", wantLength: 1},
		{name: "clear selected", query: "?mode=clear-result-metadata", wantMode: "clear-result-metadata", wantLength: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/clls/wloc"+tc.query, bytes.NewReader(frame))
			req.Host = "device-loc.lab.example.test"
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("X-Shift-My-WLOC-Mode"); got != tc.wantMode {
				t.Fatalf("mode=%q", got)
			}
			body := rr.Body.Bytes()
			if len(body) < 10 {
				t.Fatalf("short response: %x", body)
			}
			gotPayload := body[10:]
			field3 := []byte{0x18, 0x07}
			if tc.wantLength == 1 && !bytes.Contains(gotPayload, field3) {
				t.Fatalf("preserve mode lost field 3: %x", gotPayload)
			}
			if tc.wantLength == 0 && bytes.Contains(gotPayload, field3) {
				t.Fatalf("clear mode retained field 3: %x", gotPayload)
			}
		})
	}
}

func TestControlledWLOCRejectsUnknownMode(t *testing.T) {
	srv, _ := testServer(t)
	body, err := hex.DecodeString(capturedLegacyWLOCRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/clls/wloc?mode=bogus", bytes.NewReader(body))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func appendBytesFieldForProxyTest(dst []byte, field int, value []byte) []byte {
	dst = appendUvarintForProxyTest(dst, uint64(field<<3|2))
	dst = appendUvarintForProxyTest(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendVarintFieldForProxyTest(dst []byte, field int, value int64) []byte {
	dst = appendUvarintForProxyTest(dst, uint64(field<<3))
	return appendUvarintForProxyTest(dst, uint64(value))
}

func appendUvarintForProxyTest(dst []byte, value uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], value)
	return append(dst, buf[:n]...)
}

func TestControlledWLOCCoordinatesOnlyModePreservesExistingMetadata(t *testing.T) {
	srv, _ := testServer(t)

	location := appendVarintFieldForProxyTest(nil, 1, 111)
	location = appendVarintFieldForProxyTest(location, 2, 222)
	location = appendVarintFieldForProxyTest(location, 3, 88)
	location = appendVarintFieldForProxyTest(location, 5, 777)
	wifi := appendBytesFieldForProxyTest(nil, 1, []byte("aa:bb:cc:dd:ee:01"))
	wifi = appendBytesFieldForProxyTest(wifi, 2, location)
	payload := appendBytesFieldForProxyTest(nil, 2, wifi)

	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], 1)
	binary.BigEndian.PutUint32(frame[2:6], 1)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)

	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/clls/wloc?mode=coords-only", bytes.NewReader(frame))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Shift-My-WLOC-Mode"); got != "coords-only" {
		t.Fatalf("mode=%q", got)
	}
	_, _, devices, err := wloc.ParseResponse(rr.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.HorizontalAccuracy != 88 || got.Altitude != 777 {
		t.Fatalf("coords-only mode changed existing metadata: %+v", got)
	}
	if got.UnknownValue4 != 0 || got.VerticalAccuracy != 0 || got.MotionActivityType != 0 || got.MotionActivityConfidence != 0 {
		t.Fatalf("coords-only mode injected unrelated metadata: %+v", got)
	}
}

func TestShortControlledWLOCPathDefaultsToCoordsOnly(t *testing.T) {
	srv, _ := testServer(t)
	body, err := hex.DecodeString(capturedLegacyWLOCRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/w", bytes.NewReader(body))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Shift-My-WLOC-Mode"); got != "coords-only" {
		t.Fatalf("mode=%q", got)
	}
}

func TestControlledWLOCAcceptsFunction2(t *testing.T) {
	srv, _ := testServer(t)

	payload := appendBytesFieldForProxyTest(nil, 2,
		appendBytesFieldForProxyTest(nil, 1, []byte("02:00:00:00:00:01")))
	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], 1)
	binary.BigEndian.PutUint32(frame[2:6], 2)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)

	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/w", bytes.NewReader(frame))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	_, functionID, devices, err := wloc.ParseResponse(rr.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if functionID != 2 || len(devices) != 1 {
		t.Fatalf("function=%d devices=%+v", functionID, devices)
	}
}

func TestControlledWLOCRejectsUnsupportedFunctionID(t *testing.T) {
	srv, _ := testServer(t)

	payload := appendBytesFieldForProxyTest(nil, 2,
		appendBytesFieldForProxyTest(nil, 1, []byte("02:00:00:00:00:01")))
	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], 1)
	binary.BigEndian.PutUint32(frame[2:6], 3)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)

	req := httptest.NewRequest(http.MethodPost, "https://device-loc.lab.example.test/w", bytes.NewReader(frame))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestControlledWLOCPatchRichModeReturnsRealisticNeighborhood(t *testing.T) {
	srv, _ := testServer(t)
	body, err := hex.DecodeString(capturedLegacyWLOCRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"https://device-loc.lab.example.test/clls/wloc?mode=patch-rich",
		bytes.NewReader(body))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Shift-My-WLOC-Mode"); got != "patch-rich" {
		t.Fatalf("mode=%q", got)
	}
	_, functionID, devices, err := wloc.ParseResponse(rr.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if functionID != 1 || len(devices) != wloc.DefaultRichFixtureWifiRecords {
		t.Fatalf("function=%d devices=%d", functionID, len(devices))
	}
	wantLat := int64(math.Trunc(40.758 * 1e8))
	wantLon := int64(math.Trunc(-73.9855 * 1e8))
	wantPatched := wloc.DefaultRichFixtureWifiRecords - wloc.DefaultRichFixtureWifiRecords/8
	if devices[0].BSSID != "34:DB:FD:43:E3:A1" {
		t.Fatalf("first bssid=%q", devices[0].BSSID)
	}
	for i := 0; i < wantPatched; i++ {
		if devices[i].LatitudeE8 != wantLat || devices[i].LongitudeE8 != wantLon {
			t.Fatalf("device[%d]=%+v", i, devices[i])
		}
	}
	for i := wantPatched; i < len(devices); i++ {
		if devices[i].LatitudeE8 != 0 || devices[i].LongitudeE8 != 0 {
			t.Fatalf("missing-location device[%d] was synthesized: %+v", i, devices[i])
		}
	}
}

func TestControlledWLOCPatchRichWithSynthetic22Request(t *testing.T) {
	srv, _ := testServer(t)
	body, err := wloc.BuildSyntheticStructuredRequestFixture(wloc.DefaultRealisticRequestWifiRecords)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := wloc.ParseRequest(body)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"https://device-loc.lab.example.test/clls/wloc?mode=patch-rich",
		bytes.NewReader(body))
	req.Host = "device-loc.lab.example.test"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	_, functionID, devices, err := wloc.ParseResponse(rr.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if functionID != 1 || len(devices) != wloc.DefaultRichFixtureWifiRecords {
		t.Fatalf("function=%d devices=%d", functionID, len(devices))
	}
	for i, bssid := range parsed.BSSIDs {
		if devices[i].BSSID != bssid {
			t.Fatalf("device[%d].BSSID=%q want=%q", i, devices[i].BSSID, bssid)
		}
	}
	wantPatched := wloc.DefaultRichFixtureWifiRecords - wloc.DefaultRichFixtureWifiRecords/8
	located := 0
	for _, device := range devices {
		if device.LatitudeE8 != 0 || device.LongitudeE8 != 0 {
			located++
		}
	}
	if located != wantPatched {
		t.Fatalf("located=%d want=%d", located, wantPatched)
	}
}
