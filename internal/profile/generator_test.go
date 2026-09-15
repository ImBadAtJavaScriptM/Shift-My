package profile

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"howett.net/plist"
)

func TestGenerateBuildsRemovableControlledDomainProfileWithIPv6(t *testing.T) {
	certPEM, _, err := pki.GenerateRoot("Shift-My Test Root")
	if err != nil { t.Fatal(err) }
	block, _ := pem.Decode(certPEM)
	if block == nil { t.Fatal("root PEM missing") }
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil { t.Fatal(err) }

	matchDomains := []string{
		"loc-a.lab.example.test",
		"loc-b.lab.example.test",
		"device-loc.lab.example.test",
	}
	data, err := Generate(Config{
		DisplayName:  "Shift-My Test",
		PublicHost:   "lab.example.test",
		PublicIP:     net.ParseIP("2001:db8::10"),
		Token:        "token-1",
		RootCertDER:  cert.Raw,
		MatchDomains: matchDomains,
	})
	if err != nil { t.Fatal(err) }

	var root map[string]any
	if _, err := plist.Unmarshal(data, &root); err != nil { t.Fatal(err) }
	if got, ok := root["PayloadRemovalDisallowed"].(bool); !ok || got {
		t.Fatalf("PayloadRemovalDisallowed=%v", root["PayloadRemovalDisallowed"])
	}
	payloads, ok := root["PayloadContent"].([]any)
	if !ok || len(payloads) != 2 {
		t.Fatalf("PayloadContent=%T %#v", root["PayloadContent"], root["PayloadContent"])
	}

	ca := payloadByType(t, payloads, "com.apple.security.root")
	if got, ok := ca["PayloadContent"].([]byte); !ok || string(got) != string(cert.Raw) {
		t.Fatal("root certificate DER mismatch")
	}

	dnsPayload := payloadByType(t, payloads, "com.apple.dnsSettings.managed")
	dnsSettings, ok := dnsPayload["DNSSettings"].(map[string]any)
	if !ok { t.Fatalf("DNSSettings=%T", dnsPayload["DNSSettings"]) }
	if dnsSettings["DNSProtocol"] != "HTTPS" {
		t.Fatalf("DNSProtocol=%v", dnsSettings["DNSProtocol"])
	}
	if dnsSettings["ServerURL"] != "https://lab.example.test/dns-query/token-1" {
		t.Fatalf("ServerURL=%v", dnsSettings["ServerURL"])
	}
	if failover, ok := dnsSettings["AllowFailover"].(bool); !ok || failover {
		t.Fatalf("AllowFailover=%v", dnsSettings["AllowFailover"])
	}
	assertStringArray(t, dnsSettings["ServerAddresses"], []string{"2001:db8::10"})
	assertStringArray(t, dnsSettings["SupplementalMatchDomains"], matchDomains)
}

func TestGenerateStillAcceptsIPv4(t *testing.T) {
	data, err := Generate(Config{
		DisplayName:  "Shift-My Test",
		PublicHost:   "lab.example.test",
		PublicIP:     net.ParseIP("203.0.113.10"),
		Token:        "token-1",
		RootCertDER:  []byte{1},
		MatchDomains: []string{"loc-a.lab.example.test"},
	})
	if err != nil { t.Fatal(err) }
	if len(data) == 0 { t.Fatal("expected profile bytes") }
}

func TestGenerateRejectsProductionAppleDomain(t *testing.T) {
	_, err := Generate(Config{
		DisplayName:  "Shift-My Test",
		PublicHost:   "apple.com",
		PublicIP:     net.ParseIP("2001:db8::10"),
		Token:        "token-1",
		RootCertDER:  []byte{1},
		MatchDomains: []string{"gs-loc.apple.com"},
	})
	if err == nil {
		t.Fatal("expected production Apple domain to be rejected")
	}
}

func payloadByType(t *testing.T, payloads []any, want string) map[string]any {
	t.Helper()
	for _, raw := range payloads {
		payload, ok := raw.(map[string]any)
		if ok && payload["PayloadType"] == want {
			return payload
		}
	}
	t.Fatalf("payload %q not found", want)
	return nil
}

func assertStringArray(t *testing.T, raw any, want []string) {
	t.Helper()
	values, ok := raw.([]any)
	if !ok { t.Fatalf("array=%T", raw) }
	if len(values) != len(want) { t.Fatalf("array=%v want=%v", values, want) }
	for i, value := range values {
		if value != want[i] { t.Fatalf("array[%d]=%v want=%v", i, value, want[i]) }
	}
}
