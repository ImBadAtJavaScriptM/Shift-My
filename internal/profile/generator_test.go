package profile

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"strings"
	"testing"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"howett.net/plist"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	certPEM, _, err := pki.GenerateRoot("Shift-My Test Root")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("root PEM missing")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		DisplayName:      "Shift-My Test",
		PublicHost:       "lab.example.test",
		PublicIP:         net.ParseIP("2001:db8::10"),
		Token:            "doh-token-1",
		Stage2Token:      "stage2-token-1",
		ClientIdentifier: "client-identifier-1",
		RootCertDER:      cert.Raw,
		MatchDomains: []string{
			"loc-a.lab.example.test",
			"loc-b.lab.example.test",
			"device-loc.lab.example.test",
		},
	}
}

func TestGenerateStage1BuildsDeclarationsAttestedIdentityAndControlledDNS(t *testing.T) {
	cfg := testConfig(t)
	data, err := GenerateStage1(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]any
	if _, err := plist.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if got, ok := root["PayloadRemovalDisallowed"].(bool); !ok || got {
		t.Fatalf("PayloadRemovalDisallowed=%v", root["PayloadRemovalDisallowed"])
	}
	payloads := payloadList(t, root)
	if len(payloads) != 3 {
		t.Fatalf("PayloadContent len=%d", len(payloads))
	}
	if hasPayloadType(payloads, "com.apple.security.root") {
		t.Fatal("stage 1 must not contain the trusted root CA")
	}

	declarations := payloadByType(t, payloads, "com.apple.declarations")
	rawDeclarations, ok := declarations["Declarations"].([]any)
	if !ok || len(rawDeclarations) != 2 {
		t.Fatalf("Declarations=%T %#v", declarations["Declarations"], declarations["Declarations"])
	}
	foundLegacy := false
	foundActivation := false
	for _, raw := range rawDeclarations {
		b, ok := raw.([]byte)
		if !ok {
			t.Fatalf("declaration=%T", raw)
		}
		var decl map[string]any
		if err := json.Unmarshal(b, &decl); err != nil {
			t.Fatal(err)
		}
		switch decl["Type"] {
		case "com.apple.configuration.legacy":
			foundLegacy = true
			payload := decl["Payload"].(map[string]any)
			got := payload["ProfileURL"].(string)
			if got != "https://lab.example.test/api/profile/standard.mobileconfig?p=stage2-token-1" {
				t.Fatalf("ProfileURL=%q", got)
			}
		case "com.apple.activation.simple":
			foundActivation = true
		}
	}
	if !foundLegacy || !foundActivation {
		t.Fatalf("legacy=%v activation=%v", foundLegacy, foundActivation)
	}

	acme := payloadByType(t, payloads, "com.apple.security.acme")
	if acme["DirectoryURL"] != "https://lab.example.test/acme/device/directory" {
		t.Fatalf("DirectoryURL=%v", acme["DirectoryURL"])
	}
	if acme["ClientIdentifier"] != "client-identifier-1" {
		t.Fatalf("ClientIdentifier=%v", acme["ClientIdentifier"])
	}
	if got, ok := acme["HardwareBound"].(bool); !ok || !got {
		t.Fatalf("HardwareBound=%v", acme["HardwareBound"])
	}
	if got, ok := acme["Attest"].(bool); !ok || !got {
		t.Fatalf("Attest=%v", acme["Attest"])
	}
	if got, ok := acme["KeyIsExtractable"].(bool); !ok || got {
		t.Fatalf("KeyIsExtractable=%v", acme["KeyIsExtractable"])
	}

	assertControlledDNS(t, payloadByType(t, payloads, "com.apple.dnsSettings.managed"), cfg.MatchDomains)
}

func TestGenerateStage2BuildsRootAndControlledDNS(t *testing.T) {
	cfg := testConfig(t)
	data, err := GenerateStage2(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if _, err := plist.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	payloads := payloadList(t, root)
	if len(payloads) != 2 {
		t.Fatalf("PayloadContent len=%d", len(payloads))
	}
	ca := payloadByType(t, payloads, "com.apple.security.root")
	if got, ok := ca["PayloadContent"].([]byte); !ok || string(got) != string(cfg.RootCertDER) {
		t.Fatal("root certificate DER mismatch")
	}
	assertControlledDNS(t, payloadByType(t, payloads, "com.apple.dnsSettings.managed"), cfg.MatchDomains)
}

func TestGenerateStage1RequiresEnrollmentCredentials(t *testing.T) {
	cfg := testConfig(t)
	cfg.Stage2Token = ""
	if _, err := GenerateStage1(cfg); err == nil {
		t.Fatal("expected missing stage 2 token to fail")
	}
	cfg = testConfig(t)
	cfg.ClientIdentifier = ""
	if _, err := GenerateStage1(cfg); err == nil {
		t.Fatal("expected missing client identifier to fail")
	}
}

func TestGenerateStillRejectsProductionAppleDomain(t *testing.T) {
	cfg := testConfig(t)
	cfg.PublicHost = "apple.com"
	cfg.MatchDomains = []string{"gs-loc.apple.com"}
	if _, err := GenerateStage1(cfg); err == nil {
		t.Fatal("expected production Apple domain to be rejected")
	}
	if _, err := GenerateStage2(cfg); err == nil {
		t.Fatal("expected production Apple domain to be rejected")
	}
}

func TestGeneratedProfilesDoNotContainProductionAppleOrICloudHosts(t *testing.T) {
	cfg := testConfig(t)
	for _, generate := range []func(Config) ([]byte, error){GenerateStage1, GenerateStage2} {
		data, err := generate(cfg)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(string(data))
		for _, blocked := range []string{"gs-loc.apple.com", "icloud.com"} {
			if strings.Contains(text, blocked) {
				t.Fatalf("generated profile contains blocked production host %q", blocked)
			}
		}
	}
}

func assertControlledDNS(t *testing.T, dnsPayload map[string]any, matchDomains []string) {
	t.Helper()
	dnsSettings, ok := dnsPayload["DNSSettings"].(map[string]any)
	if !ok {
		t.Fatalf("DNSSettings=%T", dnsPayload["DNSSettings"])
	}
	if dnsSettings["DNSProtocol"] != "HTTPS" {
		t.Fatalf("DNSProtocol=%v", dnsSettings["DNSProtocol"])
	}
	if dnsSettings["ServerURL"] != "https://lab.example.test/dns-query/doh-token-1" {
		t.Fatalf("ServerURL=%v", dnsSettings["ServerURL"])
	}
	if failover, ok := dnsSettings["AllowFailover"].(bool); !ok || failover {
		t.Fatalf("AllowFailover=%v", dnsSettings["AllowFailover"])
	}
	assertStringArray(t, dnsSettings["ServerAddresses"], []string{"2001:db8::10"})
	assertStringArray(t, dnsSettings["SupplementalMatchDomains"], matchDomains)
}

func payloadList(t *testing.T, root map[string]any) []any {
	t.Helper()
	payloads, ok := root["PayloadContent"].([]any)
	if !ok {
		t.Fatalf("PayloadContent=%T %#v", root["PayloadContent"], root["PayloadContent"])
	}
	return payloads
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

func hasPayloadType(payloads []any, want string) bool {
	for _, raw := range payloads {
		payload, ok := raw.(map[string]any)
		if ok && payload["PayloadType"] == want {
			return true
		}
	}
	return false
}

func assertStringArray(t *testing.T, raw any, want []string) {
	t.Helper()
	values, ok := raw.([]any)
	if !ok {
		t.Fatalf("array=%T", raw)
	}
	if len(values) != len(want) {
		t.Fatalf("array=%v want=%v", values, want)
	}
	for i, value := range values {
		if value != want[i] {
			t.Fatalf("array[%d]=%v want=%v", i, value, want[i])
		}
	}
}
