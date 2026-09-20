package storage

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestUpdateTargetPersistsAndIncrementsRevision(t *testing.T) {
	path := t.TempDir() + "/state.db"
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureInstallation("token-1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateTarget(40.7580, -73.9855, "Times Square")
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationRevision != 1 {
		t.Fatalf("revision=%d", got.LocationRevision)
	}
	if got.SelectedLabel != "Times Square" {
		t.Fatalf("label=%q", got.SelectedLabel)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got2, err := reopened.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if got2.LocationRevision != 1 || got2.SelectedLabel != "Times Square" {
		t.Fatalf("reopened=%+v", got2)
	}
}

func TestEnrollmentMigrationPreservesExistingLocationAndRotatesLegacyProfileTokenOnce(t *testing.T) {
	path := t.TempDir() + "/state.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE installation (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	profile_token TEXT NOT NULL,
	selected_latitude REAL,
	selected_longitude REAL,
	selected_label TEXT NOT NULL DEFAULT '',
	location_revision INTEGER NOT NULL DEFAULT 0,
	doh_seen_at TEXT,
	proxy_seen_at TEXT
);
INSERT INTO installation (
	id, profile_token, selected_latitude, selected_longitude, selected_label, location_revision
) VALUES (1, 'old-exposed-token', 40.758, -73.9855, 'Times Square', 7);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureEnrollmentCredentials("new-doh", "stage2-one", "client-one"); err != nil {
		t.Fatal(err)
	}
	first, err := s.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if first.ProfileToken != "new-doh" || first.Stage2Token != "stage2-one" || first.ClientIdentifier != "client-one" {
		t.Fatalf("credentials=%+v", first)
	}
	if first.SelectedLabel != "Times Square" || first.LocationRevision != 7 {
		t.Fatalf("location state changed: %+v", first)
	}

	if err := s.EnsureEnrollmentCredentials("unexpected-rotation", "stage2-two", "client-two"); err != nil {
		t.Fatal(err)
	}
	second, err := s.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if second.ProfileToken != "new-doh" || second.Stage2Token != "stage2-one" || second.ClientIdentifier != "client-one" {
		t.Fatalf("credentials rotated on restart: %+v", second)
	}
}

func TestStage2AndIdentityStatusPersist(t *testing.T) {
	s, err := Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureInstallation("token-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureEnrollmentCredentials("token-2", "stage2-token", "client-id"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC)
	if err := s.MarkStage2Delivered(now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkStage2Delivered(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIdentityEnrolled(now.Add(time.Minute), "sha256:abc123"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage2DeliveredAt == nil || got.Stage2DeliveryCount != 2 {
		t.Fatalf("stage2=%+v", got)
	}
	if got.IdentityEnrolledAt == nil || got.IdentityCertFingerprint != "sha256:abc123" {
		t.Fatalf("identity=%+v", got)
	}
}

func TestUpdateTargetRejectsOutOfRangeCoordinates(t *testing.T) {
	s, err := Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureInstallation("token-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTarget(91, 0, "bad"); err == nil {
		t.Fatal("expected latitude validation error")
	}
	if _, err := s.UpdateTarget(0, 181, "bad"); err == nil {
		t.Fatal("expected longitude validation error")
	}
}

func TestMarkSeenTimestampsPersist(t *testing.T) {
	s, err := Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureInstallation("token-1"); err != nil {
		t.Fatal(err)
	}
	doh := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	proxy := doh.Add(time.Minute)
	if err := s.MarkDoHSeen(doh); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkProxySeen(proxy); err != nil {
		t.Fatal(err)
	}
	got, err := s.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if got.DoHSeenAt == nil || !got.DoHSeenAt.Equal(doh) {
		t.Fatalf("doh=%v", got.DoHSeenAt)
	}
	if got.ProxySeenAt == nil || !got.ProxySeenAt.Equal(proxy) {
		t.Fatalf("proxy=%v", got.ProxySeenAt)
	}
}

