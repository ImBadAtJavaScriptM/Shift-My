package dashboard

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/location"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/profile"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	webassets "github.com/ImBadAtJavaScriptM/Shift-My/web"
)

const (
	maxLocationBody = 16 << 10
	maxResetBody    = 4 << 10
)

type Option func(*handler)

type handler struct {
	store      *storage.Store
	loc        *location.Service
	tmpl       *template.Template
	profileCfg *profile.Config
}

type statusResponse struct {
	SelectedLatitude  *float64 `json:"selected_latitude,omitempty"`
	SelectedLongitude *float64 `json:"selected_longitude,omitempty"`
	SelectedLabel     string   `json:"selected_label"`
	LocationRevision int64    `json:"location_revision"`
	DoHSeen          bool     `json:"doh_seen"`
	ProxySeen        bool     `json:"proxy_seen"`
	ProfileStatus    string   `json:"profile_status"`
	Stage2Delivered bool     `json:"stage2_delivered"`
	IdentityEnrolled bool    `json:"identity_enrolled"`
}

type locationRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Label     string  `json:"label"`
}

type resetEnrollmentRequest struct {
	Confirm string `json:"confirm"`
}

func WithProfileConfig(cfg profile.Config) Option {
	return func(h *handler) {
		copy := cfg
		copy.RootCertDER = append([]byte(nil), cfg.RootCertDER...)
		copy.MatchDomains = append([]string(nil), cfg.MatchDomains...)
		h.profileCfg = &copy
	}
}

func New(store *storage.Store, loc *location.Service, options ...Option) http.Handler {
	h := &handler{
		store: store,
		loc:   loc,
		tmpl:  template.Must(template.ParseFS(webassets.Assets, "templates/index.html")),
	}
	for _, option := range options {
		option(h)
	}

	staticRoot, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticRoot)))
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		staticHandler.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/api/status", h.status)
	mux.HandleFunc("/api/location", h.setLocation)
	mux.HandleFunc("/api/location/history", h.locationHistory)
	mux.HandleFunc("/api/location/presets", h.locationPresets)
	mux.HandleFunc("/api/enrollment/reset", h.resetEnrollment)
	mux.HandleFunc("/profile.mobileconfig", h.mobileconfig)
	mux.HandleFunc("/api/profile/standard.mobileconfig", h.stage2Mobileconfig)
	mux.HandleFunc("/", h.index)
	return mux
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, "render dashboard", http.StatusInternalServerError)
	}
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	inst, err := h.store.Installation()
	if err != nil {
		http.Error(w, "read status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, safeStatus(inst))
}

func (h *handler) setLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLocationBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req locationRequest
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := ensureSingleJSONValue(dec); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	inst, err := h.loc.SetTarget(req.Latitude, req.Longitude, req.Label)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, safeStatus(inst))
}


func (h *handler) locationHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := h.store.LocationHistory(12)
	if err != nil {
		http.Error(w, "read location history", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) locationPresets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.store.LocationPresets()
		if err != nil {
			http.Error(w, "read location presets", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxLocationBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var req locationRequest
		if err := dec.Decode(&req); err != nil || ensureSingleJSONValue(dec) != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		item, err := h.store.SaveLocationPreset(req.Label, req.Latitude, req.Longitude)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "valid preset id required", http.StatusBadRequest)
			return
		}
		if err := h.store.DeleteLocationPreset(id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "preset not found", http.StatusNotFound)
				return
			}
			http.Error(w, "delete location preset", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handler) resetEnrollment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxResetBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req resetEnrollmentRequest
	if err := dec.Decode(&req); err != nil || ensureSingleJSONValue(dec) != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Confirm != "RESET" {
		http.Error(w, "reset confirmation required", http.StatusBadRequest)
		return
	}

	profileToken, err := secureToken(32)
	if err != nil {
		http.Error(w, "generate profile credential", http.StatusInternalServerError)
		return
	}
	stage2Token, err := secureToken(32)
	if err != nil {
		http.Error(w, "generate stage 2 credential", http.StatusInternalServerError)
		return
	}
	clientIdentifier, err := secureToken(32)
	if err != nil {
		http.Error(w, "generate client identifier", http.StatusInternalServerError)
		return
	}
	inst, err := h.store.ResetEnrollment(profileToken, stage2Token, clientIdentifier)
	if err != nil {
		http.Error(w, "reset enrollment", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, safeStatus(inst))
}

func secureToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (h *handler) mobileconfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.profileCfg == nil {
		http.Error(w, "profile generation is not configured", http.StatusServiceUnavailable)
		return
	}
	inst, err := h.store.Installation()
	if err != nil {
		http.Error(w, "read installation", http.StatusInternalServerError)
		return
	}
	cfg := *h.profileCfg
	cfg.Token = inst.ProfileToken
	cfg.Stage2Token = inst.Stage2Token
	cfg.ClientIdentifier = inst.ClientIdentifier
	data, err := profile.GenerateStage1(cfg)
	if err != nil {
		http.Error(w, "generate profile", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="shift-my-test.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *handler) stage2Mobileconfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.profileCfg == nil {
		http.Error(w, "profile generation is not configured", http.StatusServiceUnavailable)
		return
	}
	inst, err := h.store.Installation()
	if err != nil {
		http.Error(w, "read installation", http.StatusInternalServerError)
		return
	}
	provided := r.URL.Query().Get("p")
	if provided == "" || !constantTimeTokenEqual(provided, inst.Stage2Token) {
		http.Error(w, "invalid stage 2 token", http.StatusUnauthorized)
		return
	}
	cfg := *h.profileCfg
	cfg.Token = inst.ProfileToken
	data, err := profile.GenerateStage2(cfg)
	if err != nil {
		http.Error(w, "generate stage 2 profile", http.StatusInternalServerError)
		return
	}
	if err := h.store.MarkStage2Delivered(time.Now()); err != nil {
		http.Error(w, "record stage 2 delivery", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="shift-my-test-standard.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func constantTimeTokenEqual(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func ensureSingleJSONValue(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func safeStatus(inst storage.Installation) statusResponse {
	profileStatus := "generated"
	if inst.DoHSeenAt != nil || inst.ProxySeenAt != nil {
		profileStatus = "traffic_seen"
	}
	return statusResponse{
		SelectedLatitude:  inst.SelectedLatitude,
		SelectedLongitude: inst.SelectedLongitude,
		SelectedLabel:     inst.SelectedLabel,
		LocationRevision: inst.LocationRevision,
		DoHSeen:          inst.DoHSeenAt != nil,
		ProxySeen:        inst.ProxySeenAt != nil,
		ProfileStatus:    profileStatus,
		Stage2Delivered: inst.Stage2DeliveredAt != nil,
		IdentityEnrolled: inst.IdentityEnrolledAt != nil,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
