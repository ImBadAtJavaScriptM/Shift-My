package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"howett.net/plist"
)

type Config struct {
	DisplayName      string
	PublicHost       string
	PublicIP         net.IP
	Token            string
	Stage2Token      string
	ClientIdentifier string
	RootCertDER      []byte
	MatchDomains     []string
}

// LabDomains returns only project-controlled hostnames. Production Apple/iCloud
// domains are deliberately rejected by validation below.
func LabDomains(publicHost string) []string {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publicHost)), ".")
	return []string{
		"loc-a." + base,
		"loc-b." + base,
		"device-loc." + base,
	}
}

// Generate is kept as the dashboard's primary profile generator. It now emits
// Stage 1: declarations + hardware-bound ACME identity + controlled DoH.
func Generate(cfg Config) ([]byte, error) {
	return GenerateStage1(cfg)
}

func GenerateStage1(cfg Config) ([]byte, error) {
	base, ip, matchDomains, err := validateBase(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Stage2Token == "" {
		return nil, fmt.Errorf("stage 2 token is required")
	}
	if cfg.ClientIdentifier == "" {
		return nil, fmt.Errorf("client identifier is required")
	}

	declarationsUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	dnsUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	acmeUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	profileUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	legacyToken, err := newServerToken()
	if err != nil {
		return nil, err
	}
	activationToken, err := newServerToken()
	if err != nil {
		return nil, err
	}

	legacyIdentifier := "io.shiftmytest.location.legacy.profile"
	legacyDeclaration, err := json.Marshal(map[string]any{
		"Identifier": legacyIdentifier,
		"Payload": map[string]any{
			"ProfileURL": "https://" + base + "/api/profile/standard.mobileconfig?p=" + url.QueryEscape(cfg.Stage2Token),
		},
		"ServerToken": legacyToken,
		"Type":        "com.apple.configuration.legacy",
	})
	if err != nil {
		return nil, fmt.Errorf("encode legacy declaration: %w", err)
	}
	activationDeclaration, err := json.Marshal(map[string]any{
		"Identifier": "io.shiftmytest.location.activation",
		"Payload": map[string]any{
			"StandardConfigurations": []string{legacyIdentifier},
		},
		"ServerToken": activationToken,
		"Type":        "com.apple.activation.simple",
	})
	if err != nil {
		return nil, fmt.Errorf("encode activation declaration: %w", err)
	}

	declarationsPayload := map[string]any{
		"PayloadType":        "com.apple.declarations",
		"PayloadVersion":     1,
		"PayloadIdentifier":  "io.shiftmytest.location.declarations",
		"PayloadUUID":        declarationsUUID,
		"PayloadDisplayName": "Shift-My Test Declarations",
		"Declarations":       []any{legacyDeclaration, activationDeclaration},
	}
	dnsPayload := managedDNSPayload(base, ip, cfg.Token, matchDomains, dnsUUID, "io.shiftmytest.location.dns", "Shift-My Test DNS")
	acmePayload := map[string]any{
		"PayloadType":        "com.apple.security.acme",
		"PayloadVersion":     1,
		"PayloadIdentifier":  "io.shiftmytest.identity.acme",
		"PayloadUUID":        acmeUUID,
		"PayloadDisplayName": "Shift-My Test Device Identity",
		"DirectoryURL":       "https://" + base + "/acme/device/directory",
		"ClientIdentifier":   cfg.ClientIdentifier,
		"KeyType":            "ECSECPrimeRandom",
		"KeySize":            256,
		"HardwareBound":      true,
		"Attest":             true,
		"KeyIsExtractable":   false,
		"Subject": []any{
			[]any{[]any{"O", "Shift-My Test"}},
			[]any{[]any{"OU", "Device Identity"}},
			[]any{[]any{"CN", cfg.ClientIdentifier}},
		},
		"UsageFlags": 1,
		"ExtendedKeyUsage": []any{
			"1.3.6.1.5.5.7.3.2",
		},
	}

	displayName := strings.TrimSpace(cfg.DisplayName)
	if displayName == "" {
		displayName = "Shift-My Test"
	}
	root := map[string]any{
		"PayloadType":              "Configuration",
		"PayloadVersion":           1,
		"PayloadIdentifier":        "io.shiftmytest.location",
		"PayloadUUID":              profileUUID,
		"PayloadDisplayName":       displayName,
		"PayloadRemovalDisallowed": false,
		"PayloadContent":           []any{declarationsPayload, dnsPayload, acmePayload},
	}
	data, err := plist.Marshal(root, plist.XMLFormat)
	if err != nil {
		return nil, fmt.Errorf("encode stage 1 mobileconfig: %w", err)
	}
	return data, nil
}

func GenerateStage2(cfg Config) ([]byte, error) {
	base, ip, matchDomains, err := validateBase(cfg)
	if err != nil {
		return nil, err
	}
	if len(cfg.RootCertDER) == 0 {
		return nil, fmt.Errorf("root certificate DER is required")
	}

	rootUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	dnsUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	profileUUID, err := newUUID()
	if err != nil {
		return nil, err
	}

	caPayload := map[string]any{
		"PayloadType":        "com.apple.security.root",
		"PayloadVersion":     1,
		"PayloadIdentifier":  "io.shiftmytest.location.standard.ca",
		"PayloadUUID":        rootUUID,
		"PayloadDisplayName": "Shift-My Test CA",
		"PayloadContent":     append([]byte(nil), cfg.RootCertDER...),
	}
	dnsPayload := managedDNSPayload(base, ip, cfg.Token, matchDomains, dnsUUID, "io.shiftmytest.location.standard.dns", "Shift-My Test DNS")

	root := map[string]any{
		"PayloadType":              "Configuration",
		"PayloadVersion":           1,
		"PayloadIdentifier":        "io.shiftmytest.location.standard",
		"PayloadUUID":              profileUUID,
		"PayloadDisplayName":       "Shift-My Test Stage 2",
		"PayloadRemovalDisallowed": false,
		"PayloadContent":           []any{caPayload, dnsPayload},
	}
	data, err := plist.Marshal(root, plist.XMLFormat)
	if err != nil {
		return nil, fmt.Errorf("encode stage 2 mobileconfig: %w", err)
	}
	return data, nil
}

func validateBase(cfg Config) (string, net.IP, []string, error) {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cfg.PublicHost)), ".")
	if base == "" || strings.ContainsAny(base, "/: ") {
		return "", nil, nil, fmt.Errorf("valid public hostname is required")
	}
	ip := cfg.PublicIP
	if ip == nil || ip.To16() == nil {
		return "", nil, nil, fmt.Errorf("valid public IP is required")
	}
	if cfg.Token == "" {
		return "", nil, nil, fmt.Errorf("profile token is required")
	}
	if len(cfg.MatchDomains) == 0 {
		return "", nil, nil, fmt.Errorf("at least one controlled match domain is required")
	}
	matchDomains := make([]string, 0, len(cfg.MatchDomains))
	for _, domain := range cfg.MatchDomains {
		normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if !strings.HasSuffix(normalized, "."+base) || normalized == base {
			return "", nil, nil, fmt.Errorf("match domain %q is not a controlled subdomain of %q", domain, base)
		}
		if isBlockedProductionDomain(normalized) {
			return "", nil, nil, fmt.Errorf("production service domain %q is not allowed", domain)
		}
		matchDomains = append(matchDomains, normalized)
	}
	return base, ip, matchDomains, nil
}

func managedDNSPayload(base string, ip net.IP, token string, matchDomains []string, uuid, identifier, displayName string) map[string]any {
	return map[string]any{
		"PayloadType":        "com.apple.dnsSettings.managed",
		"PayloadVersion":     1,
		"PayloadIdentifier":  identifier,
		"PayloadUUID":        uuid,
		"PayloadDisplayName": displayName,
		"DNSSettings": map[string]any{
			"DNSProtocol":              "HTTPS",
			"ServerURL":                "https://" + base + "/dns-query/" + token,
			"ServerAddresses":          []string{ip.String()},
			"SupplementalMatchDomains": matchDomains,
			"AllowFailover":            false,
		},
	}
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate payload UUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

func newServerToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate declaration server token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func isBlockedProductionDomain(host string) bool {
	blocked := []string{"apple.com", "icloud.com"}
	for _, suffix := range blocked {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
