package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
)

var oidAppleFreshness = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 11, 1}

const appleEnterpriseAttestationRootPEM = `-----BEGIN CERTIFICATE-----
MIICJDCCAamgAwIBAgIUQsDCuyxyfFxeq/bxpm8frF15hzcwCgYIKoZIzj0EAwMw
UTEtMCsGA1UEAwwkQXBwbGUgRW50ZXJwcmlzZSBBdHRlc3RhdGlvbiBSb290IENB
MRMwEQYDVQQKDApBcHBsZSBJbmMuMQswCQYDVQQGEwJVUzAeFw0yMjAyMTYxOTAx
MjRaFw00NzAyMjAwMDAwMDBaMFExLTArBgNVBAMMJEFwcGxlIEVudGVycHJpc2Ug
QXR0ZXN0YXRpb24gUm9vdCBDQTETMBEGA1UECgwKQXBwbGUgSW5jLjELMAkGA1UE
BhMCVVMwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAAT6Jigq+Ps9Q4CoT8t8q+UnOe2p
oT9nRaUfGhBTbgvqSGXPjVkbYlIWYO+1zPk2Sz9hQ5ozzmLrPmTBgEWRcHjA2/y7
7GEicps9wn2tj+G89l3INNDKETdxSPPIZpPj8VmjQjBAMA8GA1UdEwEB/wQFMAMB
Af8wHQYDVR0OBBYEFPNqTQGd8muBpV5du+UIbVbi+d66MA4GA1UdDwEB/wQEAwIB
BjAKBggqhkjOPQQDAwNpADBmAjEA1xpWmTLSpr1VH4f8Ypk8f3jMUKYz4QPG8mL5
8m9sX/b2+eXpTv2pH4RZgJjucnbcAjEA4ZSB6S45FlPuS/u4pTnzoz632rA+xW/T
ZwFEh9bhKjJ+5VQ9/Do1os0u3LEkgN/r
-----END CERTIFICATE-----`

type appleAttestationObject struct {
	Format  string                    `cbor:"fmt"`
	AttStmt appleAttestationStatement `cbor:"attStmt"`
}

type appleAttestationStatement struct {
	X5C [][]byte `cbor:"x5c"`
}

type attestationVerifier struct {
	roots []*x509.Certificate
	now   func() time.Time
}

func newAttestationVerifier() (*attestationVerifier, error) {
	block, _ := pem.Decode([]byte(appleEnterpriseAttestationRootPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("parse Apple Enterprise Attestation Root CA PEM")
	}
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse Apple Enterprise Attestation Root CA: %w", err)
	}
	return &attestationVerifier{roots: []*x509.Certificate{root}, now: time.Now}, nil
}

func (v *attestationVerifier) verify(raw []byte, challengeToken string) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("attestation object is empty")
	}
	var obj appleAttestationObject
	if err := cbor.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("decode attestation object: %w", err)
	}
	if obj.Format != "apple" {
		return "", fmt.Errorf("unsupported attestation format %q", obj.Format)
	}
	if len(obj.AttStmt.X5C) == 0 || len(obj.AttStmt.X5C) > 10 {
		return "", fmt.Errorf("attestation certificate chain length %d is invalid", len(obj.AttStmt.X5C))
	}

	chain := make([]*x509.Certificate, 0, len(obj.AttStmt.X5C))
	for i, der := range obj.AttStmt.X5C {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return "", fmt.Errorf("parse attestation certificate %d: %w", i, err)
		}
		chain = append(chain, cert)
	}
	leaf := chain[0]

	roots := x509.NewCertPool()
	for _, root := range v.roots {
		roots.AddCert(root)
	}
	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		intermediates.AddCert(cert)
	}
	now := time.Now()
	if v.now != nil {
		now = v.now()
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return "", fmt.Errorf("attestation chain does not verify: %w", err)
	}

	expectedFreshness := sha256.Sum256([]byte(challengeToken))
	var freshness []byte
	for _, ext := range leaf.Extensions {
		if ext.Id.Equal(oidAppleFreshness) {
			freshness = ext.Value
			break
		}
	}
	if subtle.ConstantTimeCompare(freshness, expectedFreshness[:]) != 1 {
		return "", errors.New("attestation freshness code does not match challenge")
	}

	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return "", errors.New("attested identity key is not ECDSA P-256")
	}
	spki, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal attested public key: %w", err)
	}
	sum := sha256.Sum256(spki)
	return hex.EncodeToString(sum[:]), nil
}

func spkiSHA256(publicKey any) (string, error) {
	spki, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(spki)
	return hex.EncodeToString(sum[:]), nil
}
