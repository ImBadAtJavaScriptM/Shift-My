package dashboard

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/ImBadAtJavaScriptM/Shift-My/internal/location"
    "github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func newTestHandler(t *testing.T) http.Handler {
    t.Helper()
    store, err := storage.Open(t.TempDir() + "/state.db")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = store.Close() })
    if err := store.EnsureInstallation("token-1"); err != nil {
        t.Fatal(err)
    }
    return New(store, location.New(store))
}

func TestStatusReturnsCurrentRevisionWithoutProfileToken(t *testing.T) {
    h := newTestHandler(t)
    req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
    var got map[string]any
    if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
        t.Fatal(err)
    }
    if got["location_revision"] != float64(0) {
        t.Fatalf("revision=%v", got["location_revision"])
    }
    if _, exists := got["profile_token"]; exists {
        t.Fatal("status must not expose profile token")
    }
}

func TestPostLocationPersistsTargetAndRevision(t *testing.T) {
    h := newTestHandler(t)
    body := []byte(`{"latitude":40.758,"longitude":-73.9855,"label":"Times Square"}`)
    req := httptest.NewRequest(http.MethodPost, "/api/location", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
    var got map[string]any
    if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
        t.Fatal(err)
    }
    if got["location_revision"] != float64(1) {
        t.Fatalf("revision=%v", got["location_revision"])
    }
    if got["selected_label"] != "Times Square" {
        t.Fatalf("label=%v", got["selected_label"])
    }
}

func TestPostLocationRejectsInvalidJSON(t *testing.T) {
    h := newTestHandler(t)
    req := httptest.NewRequest(http.MethodPost, "/api/location", bytes.NewBufferString("{"))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusBadRequest {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
}

func TestPostLocationRequiresJSONContentType(t *testing.T) {
    h := newTestHandler(t)
    req := httptest.NewRequest(http.MethodPost, "/api/location", bytes.NewBufferString(`{"latitude":1,"longitude":2}`))
    req.Header.Set("Content-Type", "text/plain")
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusUnsupportedMediaType {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
}
