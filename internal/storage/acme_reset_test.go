package storage

import (
	"testing"
	"time"
)

func TestResetEnrollmentPreservesLocationAndClearsEnrollmentState(t *testing.T) {
	s, err := Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureInstallation("old-profile"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureEnrollmentCredentials("old-profile", "old-stage2", "old-client"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTarget(40.758, -73.9855, "Times Square"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.MarkDoHSeen(now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkProxySeen(now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkStage2Delivered(now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIdentityEnrolled(now, "sha256:old"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnsureACMEAccount("thumb", `{"kty":"EC"}`, now); err != nil {
		t.Fatal(err)
	}
	if err := s.PutACMENonce("nonce", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := s.ResetEnrollment("new-profile", "new-stage2", "new-client")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProfileToken != "new-profile" || got.Stage2Token != "new-stage2" || got.ClientIdentifier != "new-client" {
		t.Fatalf("credentials=%+v", got)
	}
	if got.SelectedLabel != "Times Square" || got.LocationRevision != 1 {
		t.Fatalf("location state changed: %+v", got)
	}
	if got.DoHSeenAt != nil || got.ProxySeenAt != nil || got.Stage2DeliveredAt != nil || got.IdentityEnrolledAt != nil {
		t.Fatalf("enrollment markers were not cleared: %+v", got)
	}
	if got.Stage2DeliveryCount != 0 || got.IdentityCertFingerprint != "" {
		t.Fatalf("enrollment counters were not cleared: %+v", got)
	}
	if _, err := s.ACMEAccount(); err != ErrACMENotFound {
		t.Fatalf("ACME account survived reset: %v", err)
	}
	ok, err := s.ConsumeACMENonce("nonce", now)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("old ACME nonce survived reset")
	}
}
