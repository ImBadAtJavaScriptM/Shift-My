package dashboard

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/location"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	webassets "github.com/ImBadAtJavaScriptM/Shift-My/web"
)

const maxLocationBody = 16 << 10

type handler struct {
	store *storage.Store
	loc   *location.Service
	tmpl  *template.Template
}

type statusResponse struct {
	SelectedLatitude  *float64 `json:"selected_latitude,omitempty"`
	SelectedLongitude *float64 `json:"selected_longitude,omitempty"`
	SelectedLabel     string   `json:"selected_label"`
	LocationRevision int64    `json:"location_revision"`
	DoHSeen          bool     `json:"doh_seen"`
	ProxySeen        bool     `json:"proxy_seen"`
	ProfileStatus    string   `json:"profile_status"`
}

type locationRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Label     string  `json:"label"`
}

func New(store *storage.Store, loc *location.Service) http.Handler {
	h := &handler{
		store: store,
		loc:   loc,
		tmpl:  template.Must(template.ParseFS(webassets.Assets, "templates/index.html")),
	}

	staticRoot, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticRoot))))
	mux.HandleFunc("/api/status", h.status)
	mux.HandleFunc("/api/location", h.setLocation)
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
	profile := "generated"
	if inst.DoHSeenAt != nil || inst.ProxySeenAt != nil {
		profile = "traffic_seen"
	}
	return statusResponse{
		SelectedLatitude:  inst.SelectedLatitude,
		SelectedLongitude: inst.SelectedLongitude,
		SelectedLabel:     inst.SelectedLabel,
		LocationRevision: inst.LocationRevision,
		DoHSeen:          inst.DoHSeenAt != nil,
		ProxySeen:        inst.ProxySeenAt != nil,
		ProfileStatus:    profile,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
