package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"
)

type IssuedClientCertificate struct {
	Leaf        *x509.Certificate
	ChainPEM    []byte
	Fingerprint string
}

// SignClientCSR issues a short-lived client-auth-only certificate from this
// authority. The requested subject and extensions are deliberately ignored;
// the server binds the identity to its own single-device client identifier.
func (a *Authority) SignClientCSR(csrDER []byte, clientIdentifier string, validFor time.Duration) (IssuedClientCertificate, error) {
	if a == nil || a.cert == nil || a.key == nil {
		return IssuedClientCertificate{}, fmt.Errorf("identity authority is not loaded")
	}
	if clientIdentifier == "" {
		return IssuedClientCertificate{}, fmt.Errorf("client identifier is required")
	}
	if validFor <= 0 {
		return IssuedClientCertificate{}, fmt.Errorf("positive certificate lifetime is required")
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return IssuedClientCertificate{}, fmt.Errorf("parse client CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return IssuedClientCertificate{}, fmt.Errorf("verify client CSR signature: %w", err)
	}
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve.Params().Name != "P-256" {
		return IssuedClientCertificate{}, fmt.Errorf("client CSR must use an ECDSA P-256 key")
	}

	serial, err := randomSerial()
	if err != nil {
		return IssuedClientCertificate{}, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   clientIdentifier,
			Organization: []string{"Shift-My Test"},
			OrganizationalUnit: []string{"Device Identity"},
		},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:         false,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, csr.PublicKey, a.key)
	if err != nil {
		return IssuedClientCertificate{}, fmt.Errorf("issue client certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return IssuedClientCertificate{}, fmt.Errorf("parse issued client certificate: %w", err)
	}
	chain := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.cert.Raw})...,
	)
	sum := sha256.Sum256(der)
	return IssuedClientCertificate{
		Leaf:        leaf,
		ChainPEM:    chain,
		Fingerprint: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

