package storage

import (
	"testing"
	"time"
)

func TestACMEStateSurvivesStoreReopen(t *testing.T) {
	path := t.TempDir() + "/state.db"
	now := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureInstallation("token"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureEnrollmentCredentials("doh", "stage2", "client"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnsureACMEAccount("thumbprint", `{"kty":"EC","crv":"P-256","x":"x","y":"y"}`, now); err != nil {
		t.Fatal(err)
	}
	order := ACMEOrder{
		ID:                 "order-1",
		AccountID:          1,
		ClientIdentifier:   "client",
		Status:             "pending",
		ChallengeToken:     "challenge-token",
		ChallengeTokenHash: "challenge-hash",
		ChallengeStatus:    "pending",
		ExpiresAt:          now.Add(time.Hour),
		CreatedAt:          now,
	}
	if err := s.CreateACMEOrder(order); err != nil {
		t.Fatal(err)
	}
	if err := s.PutACMENonce("nonce-1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	account, err := reopened.ACMEAccount()
	if err != nil {
		t.Fatal(err)
	}
	if account.Thumbprint != "thumbprint" || account.Status != "valid" {
		t.Fatalf("account=%+v", account)
	}
	got, err := reopened.ACMEOrder("order-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ChallengeToken != "challenge-token" || got.Status != "pending" || got.ChallengeStatus != "pending" {
		t.Fatalf("order=%+v", got)
	}
	ok, err := reopened.ConsumeACMENonce("nonce-1", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("persisted nonce could not be consumed after reopen")
	}
}
