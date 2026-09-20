package dashboard

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/ImBadAtJavaScriptM/Shift-My/internal/location"
    "github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func newTestHandler(t *testing.T) http.Handler {
    t.Helper()
    h, _ := newTestHandlerAndStore(t)
    return h
}

func newTestHandlerAndStore(t *testing.T) (http.Handler, *storage.Store) {
    t.Helper()
    store, err := storage.Open(t.TempDir() + "/state.db")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = store.Close() })
    if err := store.EnsureInstallation("token-1"); err != nil {
        t.Fatal(err)
    }
    if err := store.EnsureEnrollmentCredentials("doh-token-1", "stage2-token-1", "client-id-1"); err != nil {
        t.Fatal(err)
    }
    return New(store, location.New(store)), store
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
    for _, sensitive := range []string{"profile_token", "stage2_token", "client_identifier", "identity_cert_fingerprint"} {
        if _, exists := got[sensitive]; exists {
            t.Fatalf("status must not expose %s", sensitive)
        }
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


func TestResetEnrollmentPreservesTargetAndClearsLifecycleStatus(t *testing.T) {
    h, store := newTestHandlerAndStore(t)
    if _, err := store.UpdateTarget(40.758, -73.9855, "Times Square"); err != nil {
        t.Fatal(err)
    }
    now := time.Date(2026, 9, 20, 17, 0, 0, 0, time.UTC)
    if err := store.MarkDoHSeen(now); err != nil {
        t.Fatal(err)
    }
    if err := store.MarkProxySeen(now); err != nil {
        t.Fatal(err)
    }
    if err := store.MarkStage2Delivered(now); err != nil {
        t.Fatal(err)
    }
    if err := store.MarkIdentityEnrolled(now, "sha256:test"); err != nil {
        t.Fatal(err)
    }
    before, err := store.Installation()
    if err != nil {
        t.Fatal(err)
    }

    req := httptest.NewRequest(http.MethodPost, "/api/enrollment/reset", bytes.NewBufferString(`{"confirm":"RESET"}`))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }

    after, err := store.Installation()
    if err != nil {
        t.Fatal(err)
    }
    if after.SelectedLabel != "Times Square" || after.LocationRevision != 1 {
        t.Fatalf("target changed: %+v", after)
    }
    if after.ProfileToken == before.ProfileToken || after.Stage2Token == before.Stage2Token || after.ClientIdentifier == before.ClientIdentifier {
        t.Fatal("enrollment credentials were not rotated")
    }
    if after.DoHSeenAt != nil || after.ProxySeenAt != nil || after.Stage2DeliveredAt != nil || after.IdentityEnrolledAt != nil {
        t.Fatalf("lifecycle state not cleared: %+v", after)
    }

    var got map[string]any
    if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
        t.Fatal(err)
    }
    if got["selected_label"] != "Times Square" || got["location_revision"] != float64(1) {
        t.Fatalf("safe status lost target: %#v", got)
    }
    for _, sensitive := range []string{"profile_token", "stage2_token", "client_identifier", "identity_cert_fingerprint"} {
        if _, exists := got[sensitive]; exists {
            t.Fatalf("reset response exposed %s", sensitive)
        }
    }
}

func TestResetEnrollmentRequiresExplicitJSONConfirmation(t *testing.T) {
    h := newTestHandler(t)

    req := httptest.NewRequest(http.MethodPost, "/api/enrollment/reset", bytes.NewBufferString(`{"confirm":"no"}`))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusBadRequest {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }

    req = httptest.NewRequest(http.MethodGet, "/api/enrollment/reset", nil)
    rr = httptest.NewRecorder()
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusMethodNotAllowed {
        t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
    }
}
