package acme

import (
	"crypto/x509"
	"strings"
	"testing"
)

func TestAppleAttestationVerifierRejectsWrongFreshnessToken(t *testing.T) {
	verifier, err := newAttestationVerifier()
	if err != nil {
		t.Fatal(err)
	}
	root, rootKey := testRoot(t, "Fake Apple Enterprise Attestation Root")
	verifier.roots = []*x509.Certificate{root}
	deviceKey := mustECDSAKey(t)

	raw := testAppleAttestationObject(t, root, rootKey, &deviceKey.PublicKey, "expected-token")
	if _, err := verifier.verify(raw, "different-token"); err == nil || !strings.Contains(err.Error(), "freshness") {
		t.Fatalf("expected freshness mismatch, got %v", err)
	}
}

func TestAppleAttestationVerifierAcceptsExpectedAppleFormatAndFreshness(t *testing.T) {
	verifier, err := newAttestationVerifier()
	if err != nil {
		t.Fatal(err)
	}
	root, rootKey := testRoot(t, "Fake Apple Enterprise Attestation Root")
	verifier.roots = []*x509.Certificate{root}
	deviceKey := mustECDSAKey(t)

	raw := testAppleAttestationObject(t, root, rootKey, &deviceKey.PublicKey, "challenge-token")
	got, err := verifier.verify(raw, "challenge-token")
	if err != nil {
		t.Fatal(err)
	}
	want, err := spkiSHA256(&deviceKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SPKI hash=%q want=%q", got, want)
	}
}

func TestEmbeddedAppleAttestationRootParsesAsExpectedRoot(t *testing.T) {
	verifier, err := newAttestationVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier.roots) != 1 {
		t.Fatalf("roots=%d", len(verifier.roots))
	}
	root := verifier.roots[0]
	if root.Subject.CommonName != "Apple Enterprise Attestation Root CA" {
		t.Fatalf("root CN=%q", root.Subject.CommonName)
	}
	if !root.IsCA {
		t.Fatal("embedded Apple attestation trust anchor is not a CA")
	}
}
