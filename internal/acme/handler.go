package acme

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

const nonceTTL = 10 * time.Minute

type Handler struct {
	store      *storage.Store
	publicHost string
	now        func() time.Time
}

func New(store *storage.Store, publicHost string) http.Handler {
	return &Handler{
		store:      store,
		publicHost: strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publicHost)), "."),
		now:        time.Now,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/acme/device/directory":
		h.directory(w, r)
	case "/acme/device/new-nonce":
		h.newNonce(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) directory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := "https://" + h.publicHost + "/acme/device"
	writeJSON(w, http.StatusOK, map[string]any{
		"newNonce":   base + "/new-nonce",
		"newAccount": base + "/new-account",
		"newOrder":   base + "/new-order",
	})
}

func (h *Handler) newNonce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		w.Header().Set("Allow", "HEAD, GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nonce, err := h.store.IssueACMENonce(h.now(), nonceTTL)
	if err != nil {
		http.Error(w, "issue nonce", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Replay-Nonce", nonce)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Link", "<https://"+h.publicHost+"/acme/device/directory>;rel="index"")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
