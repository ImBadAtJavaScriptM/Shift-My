package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"
)

var (
	oidSubjectAltName      = asn1.ObjectIdentifier{2, 5, 29, 17}
	oidPermanentIdentifier = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 8, 3}
)

type permanentIdentifier struct {
	IdentifierValue string `asn1:"utf8,optional"`
}

type otherName struct {
	TypeID asn1.ObjectIdentifier
	Value  asn1.RawValue
}

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
	sanDER, err := permanentIdentifierSAN(clientIdentifier)
	if err != nil {
		return IssuedClientCertificate{}, fmt.Errorf("encode permanent identifier SAN: %w", err)
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
		ExtraExtensions: []pkix.Extension{
			{Id: oidSubjectAltName, Value: sanDER},
		},
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



func permanentIdentifierSAN(clientIdentifier string) ([]byte, error) {
	if clientIdentifier == "" {
		return nil, fmt.Errorf("client identifier is required")
	}
	valueDER, err := asn1.Marshal(permanentIdentifier{IdentifierValue: clientIdentifier})
	if err != nil {
		return nil, err
	}
	// otherName ::= SEQUENCE { type-id OBJECT IDENTIFIER,
	//                          value [0] EXPLICIT ANY DEFINED BY type-id }
	encodedOtherName, err := asn1.Marshal(otherName{
		TypeID: oidPermanentIdentifier,
		Value: asn1.RawValue{
			Class:      2,
			Tag:        0,
			IsCompound: true,
			Bytes:      valueDER,
		},
	})
	if err != nil {
		return nil, err
	}
	var sequence asn1.RawValue
	rest, err := asn1.Unmarshal(encodedOtherName, &sequence)
	if err != nil || len(rest) != 0 || sequence.Class != 0 || sequence.Tag != 16 {
		return nil, fmt.Errorf("encode otherName sequence")
	}
	// GeneralName.otherName is [0] IMPLICIT OtherName, so replace the outer
	// SEQUENCE tag with a context-specific constructed tag 0 while retaining
	// the OtherName sequence contents.
	generalName := asn1.RawValue{
		Class:      2,
		Tag:        0,
		IsCompound: true,
		Bytes:      sequence.Bytes,
	}
	return asn1.Marshal([]asn1.RawValue{generalName})
}
