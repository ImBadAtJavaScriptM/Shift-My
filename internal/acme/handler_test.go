package acme

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func newTestHandler(t *testing.T) (*Handler, *storage.Store) {
	t.Helper()
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureInstallation("token-1"); err != nil {
		t.Fatal(err)
	}
	h := New(store, "lab.example.test").(*Handler)
	h.now = func() time.Time {
		return time.Date(2026, 9, 20, 17, 0, 0, 0, time.UTC)
	}
	return h, store
}

func TestDirectoryAdvertisesDeviceACMEEndpoints(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test/acme/device/directory", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["newNonce"] != "https://lab.example.test/acme/device/new-nonce" {
		t.Fatalf("newNonce=%q", got["newNonce"])
	}
	if got["newAccount"] != "https://lab.example.test/acme/device/new-account" {
		t.Fatalf("newAccount=%q", got["newAccount"])
	}
	if got["newOrder"] != "https://lab.example.test/acme/device/new-order" {
		t.Fatalf("newOrder=%q", got["newOrder"])
	}
}

func TestNewNonceHeadIsNoStoreAndSingleUse(t *testing.T) {
	h, store := newTestHandler(t)
	req := httptest.NewRequest(http.MethodHead, "https://lab.example.test/acme/device/new-nonce", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	nonce := rr.Header().Get("Replay-Nonce")
	if nonce == "" {
		t.Fatal("missing Replay-Nonce")
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", rr.Header().Get("Cache-Control"))
	}
	wantLink := "<https://lab.example.test/acme/device/directory>;rel=\"index\""
	if rr.Header().Get("Link") != wantLink {
		t.Fatalf("Link=%q", rr.Header().Get("Link"))
	}
	ok, err := store.ConsumeACMENonce(nonce, h.now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("issued nonce was not persisted")
	}
	ok, err = store.ConsumeACMENonce(nonce, h.now().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("nonce replay must fail")
	}
}

func TestNewNonceGetReturnsNoContentWithNonce(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "https://lab.example.test/acme/device/new-nonce", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Replay-Nonce") == "" {
		t.Fatal("missing Replay-Nonce")
	}
}
