package pki

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func parseCertificatePEM(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("certificate PEM missing")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestGenerateRootCreatesSigningCA(t *testing.T) {
	certPEM, keyPEM, err := GenerateRoot("Shift-My Test Root")
	if err != nil {
		t.Fatal(err)
	}
	if len(keyPEM) == 0 {
		t.Fatal("key PEM is empty")
	}
	cert := parseCertificatePEM(t, certPEM)
	if !cert.IsCA {
		t.Fatal("root must be a CA")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Fatal("root must sign certificates")
	}
}

func TestAuthorityMintsExactControlledHostSANAndCaches(t *testing.T) {
	certPEM, keyPEM, err := GenerateRoot("Shift-My Test Root")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "root.pem")
	keyPath := filepath.Join(dir, "root-key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil { t.Fatal(err) }
	auth, err := Load(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	const host = "loc-a.lab.example.test"
	first, err := auth.MintServerCertificate(host)
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.MintServerCertificate(host)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Certificate) == 0 || len(second.Certificate) == 0 {
		t.Fatal("leaf certificate missing")
	}
	if string(first.Certificate[0]) != string(second.Certificate[0]) {
		t.Fatal("expected process-lifetime cached certificate")
	}
	leaf, err := x509.ParseCertificate(first.Certificate[0])
	if err != nil { t.Fatal(err) }
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != host {
		t.Fatalf("DNSNames=%v", leaf.DNSNames)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("ExtKeyUsage=%v", leaf.ExtKeyUsage)
	}
}
