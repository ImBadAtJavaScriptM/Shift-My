package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"strings"
	"time"
)

func (a *Authority) MintServerCertificate(host string) (tls.Certificate, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" || strings.ContainsAny(host, "/: ") {
		return tls.Certificate{}, fmt.Errorf("invalid DNS hostname")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if cert, ok := a.cache[host]; ok {
		return cert, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate leaf key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"Shift-My Test Lab"},
		},
		DNSNames:     []string{host},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create leaf certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse generated leaf: %w", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der, a.cert.Raw},
		PrivateKey:  key,
		Leaf:        leaf,
	}
	a.cache[host] = cert
	return cert, nil
}
