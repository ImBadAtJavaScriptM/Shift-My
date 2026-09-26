package proxy

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/capture"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/netpolicy"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/wloc"
)

const maxWLOCBody = 1 << 20

// Server is the controlled TLS lab service. It accepts only hostnames owned by
// this project and never forwards requests to third-party services.
type Server struct {
	authority *pki.Authority
	store     *storage.Store
	policy    netpolicy.Policy
	recorder  capture.Recorder
}

func New(authority *pki.Authority, store *storage.Store, policy netpolicy.Policy) *Server {
	return &Server{authority: authority, store: store, policy: policy}
}

func NewWithRecorder(authority *pki.Authority, store *storage.Store, policy netpolicy.Policy, recorder capture.Recorder) *Server {
	return &Server{authority: authority, store: store, policy: policy, recorder: recorder}
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
	if r.Method == http.MethodPost && (r.URL.Path == "/clls/wloc" || r.URL.Path == "/w") {
		s.serveWLOC(w, r, host)
		return
	}
	if r.Method != http.MethodGet || r.URL.Path != "/v1/location" {
		http.NotFound(w, r)
		return
	}

	requestMeta := capture.Metadata{
		Host: host, Method: r.Method, Path: r.URL.Path, ContentType: r.Header.Get("Content-Type"),
		Protocol: r.Proto, BodyLength: r.ContentLength, Headers: r.Header.Clone(),
	}
	if s.recorder != nil {
		if err := s.recorder.RecordRequest(requestMeta, nil); err != nil {
			http.Error(w, "record lab request", http.StatusInternalServerError)
			return
		}
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

	payload, err := json.Marshal(struct {
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
	if err != nil {
		http.Error(w, "encode lab response", http.StatusInternalServerError)
		return
	}
	payload = append(payload, '\n')
	responseHeaders := make(http.Header)
	responseHeaders.Set("Content-Type", "application/json")
	responseHeaders.Set("Cache-Control", "no-store")
	if s.recorder != nil {
		if err := s.recorder.RecordResponse(capture.Metadata{
			Host: host, Method: r.Method, Path: r.URL.Path, ContentType: "application/json",
			Protocol: r.Proto, Status: http.StatusOK, BodyLength: int64(len(payload)), Headers: responseHeaders.Clone(),
		}, payload); err != nil {
			http.Error(w, "record lab response", http.StatusInternalServerError)
			return
		}
	}

	for key, values := range responseHeaders {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) serveWLOC(w http.ResponseWriter, r *http.Request, host string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWLOCBody+1))
	if err != nil {
		writeWLOCBadRequest(w)
		return
	}
	if len(body) > maxWLOCBody {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	requestMeta := capture.Metadata{
		Host: host, Method: r.Method, Path: r.URL.Path, ContentType: r.Header.Get("Content-Type"),
		Protocol: r.Proto, BodyLength: int64(len(body)), Headers: r.Header.Clone(),
	}
	if s.recorder != nil {
		if err := s.recorder.RecordRequest(requestMeta, body); err != nil {
			http.Error(w, "record lab request", http.StatusInternalServerError)
			return
		}
	}

	req, err := wloc.ParseRequest(body)
	if err != nil || (req.FunctionID != 1 && req.FunctionID != 2) {
		writeWLOCBadRequest(w)
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
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		if r.URL.Path == "/w" {
			mode = "coords-only"
		} else {
			mode = "preserve"
		}
	}

	var payload []byte
	patchStats := wloc.ResponsePatchStats{}
	switch mode {
	case "preserve":
		payload, err = wloc.BuildResponse(req, *inst.SelectedLatitude, *inst.SelectedLongitude)
		patchStats.Wifi = len(req.BSSIDs)
		patchStats.Locations = len(req.BSSIDs)
	case "clear-result-metadata":
		payload, err = wloc.BuildResponseClearingResultMetadata(req, *inst.SelectedLatitude, *inst.SelectedLongitude)
		patchStats.Wifi = len(req.BSSIDs)
		patchStats.Locations = len(req.BSSIDs)
	case "coords-only":
		payload, err = wloc.BuildResponseCoordinatesOnly(req, *inst.SelectedLatitude, *inst.SelectedLongitude)
		patchStats.Wifi = len(req.BSSIDs)
		patchStats.Locations = len(req.BSSIDs)
	case "patch-rich":
		var fixture []byte
		fixture, err = wloc.BuildRichResponseFixture(req, wloc.DefaultRichFixtureWifiRecords)
		if err == nil {
			payload, patchStats, err = wloc.PatchResponseCoordinatesOnly(fixture, *inst.SelectedLatitude, *inst.SelectedLongitude)
		}
	default:
		http.Error(w, "unsupported wloc mode", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "encode controlled wloc response", http.StatusInternalServerError)
		return
	}
	if err := s.store.MarkProxySeen(time.Now().UTC()); err != nil {
		http.Error(w, "record lab TLS activity", http.StatusInternalServerError)
		return
	}

	estimate := wloc.WifiPositionEstimate{}
	if scan, ok := wloc.SyntheticScanObservations(req.BSSIDs); ok {
		if _, _, devices, parseErr := wloc.ParseResponse(payload); parseErr == nil {
			if modeled, modelErr := wloc.EstimateWifiPosition(scan, devices, wloc.DefaultWifiPositionMaxAPs); modelErr == nil {
				estimate = modeled
			}
		}
	}

	logWLOCEvent(wlocEvent{
		Timestamp:            time.Now().UTC().Format(time.RFC3339Nano),
		Host:                 host,
		Path:                 r.URL.Path,
		Mode:                 mode,
		Envelope:             req.Envelope,
		FunctionID:           req.FunctionID,
		RequestBytes:         len(body),
		RequestBSSIDs:        len(req.BSSIDs),
		ResponseBytes:        len(payload),
		PatchedWifi:          patchStats.Wifi,
		PatchedCell:          patchStats.Cell,
		PatchedLocations:     patchStats.Locations,
		TargetRevision:       inst.LocationRevision,
		EstimateModel:        estimate.Method,
		EstimateLatitude:     estimate.Latitude,
		EstimateLongitude:    estimate.Longitude,
		EstimateMatched:      estimate.MatchedAPs,
		EstimateUsed:         estimate.UsedAPs,
		EstimateUnresolved:   estimateUnresolved(len(req.BSSIDs), estimate),
		EstimateSpreadMeters: estimate.SpreadMeters,
	})

	responseHeaders := make(http.Header)
	responseHeaders.Set("Content-Type", "application/octet-stream")
	responseHeaders.Set("Cache-Control", "no-store")
	responseHeaders.Set("X-Shift-My-Lab", "controlled-wloc-emulator")
	responseHeaders.Set("X-Shift-My-WLOC-Mode", mode)
	if s.recorder != nil {
		if err := s.recorder.RecordResponse(capture.Metadata{
			Host: host, Method: r.Method, Path: r.URL.Path, ContentType: "application/octet-stream",
			Protocol: r.Proto, Status: http.StatusOK, BodyLength: int64(len(payload)), Headers: responseHeaders.Clone(),
		}, payload); err != nil {
			http.Error(w, "record lab response", http.StatusInternalServerError)
			return
		}
	}
	for key, values := range responseHeaders {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func writeWLOCBadRequest(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Shift-My-Lab", "controlled-wloc-emulator")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte("Bad Request"))
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

type wlocEvent struct {
	Timestamp            string  `json:"timestamp"`
	Host                 string  `json:"host"`
	Path                 string  `json:"path"`
	Mode                 string  `json:"mode"`
	Envelope             string  `json:"envelope"`
	FunctionID           uint32  `json:"function_id"`
	RequestBytes         int     `json:"request_bytes"`
	RequestBSSIDs        int     `json:"request_bssids"`
	ResponseBytes        int     `json:"response_bytes"`
	PatchedWifi          int     `json:"patched_wifi"`
	PatchedCell          int     `json:"patched_cell"`
	PatchedLocations     int     `json:"patched_locations"`
	TargetRevision       int64   `json:"target_revision"`
	EstimateModel        string  `json:"estimate_model,omitempty"`
	EstimateLatitude     float64 `json:"estimate_latitude,omitempty"`
	EstimateLongitude    float64 `json:"estimate_longitude,omitempty"`
	EstimateMatched      int     `json:"estimate_matched,omitempty"`
	EstimateUsed         int     `json:"estimate_used,omitempty"`
	EstimateUnresolved   int     `json:"estimate_unresolved,omitempty"`
	EstimateSpreadMeters float64 `json:"estimate_spread_meters,omitempty"`
}

func estimateUnresolved(total int, estimate wloc.WifiPositionEstimate) int {
	if estimate.Method == "" {
		return 0
	}
	unresolved := total - estimate.MatchedAPs
	if unresolved < 0 {
		return 0
	}
	return unresolved
}

func logWLOCEvent(event wlocEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("wloc_event marshal_error=%v", err)
		return
	}
	log.Printf("wloc_event=%s", payload)
}
