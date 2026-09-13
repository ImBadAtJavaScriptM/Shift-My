package location

import (
    "testing"

    "github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func newTestService(t *testing.T) *Service {
    t.Helper()
    store, err := storage.Open(t.TempDir() + "/state.db")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = store.Close() })
    if err := store.EnsureInstallation("token-1"); err != nil {
        t.Fatal(err)
    }
    return New(store)
}

func TestSetTargetRejectsInvalidCoordinates(t *testing.T) {
    svc := newTestService(t)
    if _, err := svc.SetTarget(91, 0, "bad"); err == nil {
        t.Fatal("expected latitude validation error")
    }
    if _, err := svc.SetTarget(0, -181, "bad"); err == nil {
        t.Fatal("expected longitude validation error")
    }
}

func TestSetTargetPersistsTrimmedLabelAndOneRevision(t *testing.T) {
    svc := newTestService(t)
    got, err := svc.SetTarget(40.758, -73.9855, "  Times Square  ")
    if err != nil {
        t.Fatal(err)
    }
    if got.LocationRevision != 1 {
        t.Fatalf("revision=%d", got.LocationRevision)
    }
    if got.SelectedLabel != "Times Square" {
        t.Fatalf("label=%q", got.SelectedLabel)
    }
}
