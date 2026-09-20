package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"testing"
	"time"
)

func TestSignClientCSRAddsPermanentIdentifierSANAndClientOnlyEKU(t *testing.T) {
	certPEM, keyPEM, err := GenerateRoot("Identity CA")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := dir + "/ca.pem"
	keyPath := dir + "/ca-key.pem"
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	ca, err := Load(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "ignored"},
	}, key)
	if err != nil {
		t.Fatal(err)
	}

	const clientID = "single-iphone-client-id"
	issued, err := ca.SignClientCSR(csrDER, clientID, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	leaf := issued.Leaf
	if leaf.Subject.CommonName != clientID {
		t.Fatalf("CN=%q", leaf.Subject.CommonName)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatalf("ExtKeyUsage=%v", leaf.ExtKeyUsage)
	}
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			t.Fatal("server-auth EKU must not be present")
		}
	}

	var san []byte
	for _, ext := range leaf.Extensions {
		if ext.Id.Equal(oidSubjectAltName) {
			san = ext.Value
			break
		}
	}
	if len(san) == 0 {
		t.Fatal("subjectAltName extension missing")
	}
	if !bytes.Contains(san, []byte(clientID)) {
		t.Fatalf("subjectAltName does not contain client identifier: %x", san)
	}

	var generalNames asn1.RawValue
	rest, err := asn1.Unmarshal(san, &generalNames)
	if err != nil || len(rest) != 0 || generalNames.Tag != 16 {
		t.Fatalf("invalid GeneralNames DER: %v", err)
	}
	var gn asn1.RawValue
	rest, err = asn1.Unmarshal(generalNames.Bytes, &gn)
	if err != nil || len(rest) != 0 || gn.Class != 2 || gn.Tag != 0 {
		t.Fatalf("invalid otherName GeneralName: class=%d tag=%d err=%v", gn.Class, gn.Tag, err)
	}
	seqDER, err := asn1.Marshal(asn1.RawValue{
		Class:      0,
		Tag:        16,
		IsCompound: true,
		Bytes:      gn.Bytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	var on otherName
	if rest, err := asn1.Unmarshal(seqDER, &on); err != nil || len(rest) != 0 {
		t.Fatalf("decode otherName: %v", err)
	}
	if !on.TypeID.Equal(oidPermanentIdentifier) {
		t.Fatalf("otherName OID=%v", on.TypeID)
	}
	var pi permanentIdentifier
	if rest, err := asn1.Unmarshal(on.Value.Bytes, &pi); err != nil || len(rest) != 0 {
		t.Fatalf("decode permanent identifier: %v", err)
	}
	if pi.IdentifierValue != clientID {
		t.Fatalf("identifier=%q", pi.IdentifierValue)
	}

	block, _ := pem.Decode(issued.ChainPEM)
	if block == nil {
		t.Fatal("issued PEM chain missing leaf")
	}
}
