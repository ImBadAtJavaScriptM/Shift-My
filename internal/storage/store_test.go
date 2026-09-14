package storage

import (
    "testing"
    "time"
)

func TestUpdateTargetPersistsAndIncrementsRevision(t *testing.T) {
    path := t.TempDir()+"/state.db"
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    if err := s.EnsureInstallation("token-1"); err != nil { t.Fatal(err) }
    got, err := s.UpdateTarget(40.7580, -73.9855, "Times Square")
    if err != nil { t.Fatal(err) }
    if got.LocationRevision != 1 { t.Fatalf("revision=%d", got.LocationRevision) }
    if got.SelectedLabel != "Times Square" { t.Fatalf("label=%q", got.SelectedLabel) }
    if err := s.Close(); err != nil { t.Fatal(err) }

    reopened, err := Open(path)
    if err != nil { t.Fatal(err) }
    defer reopened.Close()
    got2, err := reopened.Installation()
    if err != nil { t.Fatal(err) }
    if got2.LocationRevision != 1 || got2.SelectedLabel != "Times Square" {
        t.Fatalf("reopened=%+v", got2)
    }
}

func TestUpdateTargetRejectsOutOfRangeCoordinates(t *testing.T) {
    s, err := Open(t.TempDir()+"/state.db")
    if err != nil { t.Fatal(err) }
    defer s.Close()
    if err := s.EnsureInstallation("token-1"); err != nil { t.Fatal(err) }
    if _, err := s.UpdateTarget(91, 0, "bad"); err == nil {
        t.Fatal("expected latitude validation error")
    }
    if _, err := s.UpdateTarget(0, 181, "bad"); err == nil {
        t.Fatal("expected longitude validation error")
    }
}

func TestMarkSeenTimestampsPersist(t *testing.T) {
    s, err := Open(t.TempDir()+"/state.db")
    if err != nil { t.Fatal(err) }
    defer s.Close()
    if err := s.EnsureInstallation("token-1"); err != nil { t.Fatal(err) }
    doh := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
    proxy := doh.Add(time.Minute)
    if err := s.MarkDoHSeen(doh); err != nil { t.Fatal(err) }
    if err := s.MarkProxySeen(proxy); err != nil { t.Fatal(err) }
    got, err := s.Installation()
    if err != nil { t.Fatal(err) }
    if got.DoHSeenAt == nil || !got.DoHSeenAt.Equal(doh) { t.Fatalf("doh=%v", got.DoHSeenAt) }
    if got.ProxySeenAt == nil || !got.ProxySeenAt.Equal(proxy) { t.Fatalf("proxy=%v", got.ProxySeenAt) }
}
