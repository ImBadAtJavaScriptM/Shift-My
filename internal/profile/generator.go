package profile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"howett.net/plist"
)

type Config struct {
	DisplayName  string
	PublicHost   string
	PublicIPv4   net.IP
	Token        string
	RootCertDER  []byte
	MatchDomains []string
}

func LabDomains(publicHost string) []string {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publicHost)), ".")
	return []string{
		"loc-a." + base,
		"loc-b." + base,
		"device-loc." + base,
	}
}

func Generate(cfg Config) ([]byte, error) {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cfg.PublicHost)), ".")
	if base == "" || strings.ContainsAny(base, "/: ") {
		return nil, fmt.Errorf("valid public hostname is required")
	}
	ip := cfg.PublicIPv4.To4()
	if ip == nil {
		return nil, fmt.Errorf("valid public IPv4 is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("profile token is required")
	}
	if len(cfg.RootCertDER) == 0 {
		return nil, fmt.Errorf("root certificate DER is required")
	}
	if len(cfg.MatchDomains) == 0 {
		return nil, fmt.Errorf("at least one controlled match domain is required")
	}
	matchDomains := make([]string, 0, len(cfg.MatchDomains))
	for _, domain := range cfg.MatchDomains {
		normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if !strings.HasSuffix(normalized, "."+base) || normalized == base {
			return nil, fmt.Errorf("match domain %q is not a controlled subdomain of %q", domain, base)
		}
		if isBlockedProductionDomain(normalized) {
			return nil, fmt.Errorf("production service domain %q is not allowed", domain)
		}
		matchDomains = append(matchDomains, normalized)
	}

	rootUUID, err := newUUID()
	if err != nil { return nil, err }
	dnsUUID, err := newUUID()
	if err != nil { return nil, err }
	profileUUID, err := newUUID()
	if err != nil { return nil, err }

	displayName := strings.TrimSpace(cfg.DisplayName)
	if displayName == "" {
		displayName = "Shift-My Test"
	}
	caPayload := map[string]any{
		"PayloadType":        "com.apple.security.root",
		"PayloadVersion":     1,
		"PayloadIdentifier":  "io.shiftmytest.location.ca",
		"PayloadUUID":        rootUUID,
		"PayloadDisplayName": "Shift-My Test CA",
		"PayloadContent":     append([]byte(nil), cfg.RootCertDER...),
	}
	dnsPayload := map[string]any{
		"PayloadType":        "com.apple.dnsSettings.managed",
		"PayloadVersion":     1,
		"PayloadIdentifier":  "io.shiftmytest.location.dns",
		"PayloadUUID":        dnsUUID,
		"PayloadDisplayName": "Shift-My Test DNS",
		"DNSSettings": map[string]any{
			"DNSProtocol":              "HTTPS",
			"ServerURL":                "https://" + base + "/dns-query/" + cfg.Token,
			"ServerAddresses":          []string{ip.String()},
			"SupplementalMatchDomains": matchDomains,
			"AllowFailover":            false,
		},
	}
	root := map[string]any{
		"PayloadType":              "Configuration",
		"PayloadVersion":           1,
		"PayloadIdentifier":        "io.shiftmytest.location",
		"PayloadUUID":              profileUUID,
		"PayloadDisplayName":       displayName,
		"PayloadRemovalDisallowed": false,
		"PayloadContent":           []any{caPayload, dnsPayload},
	}
	data, err := plist.Marshal(root, plist.XMLFormat)
	if err != nil {
		return nil, fmt.Errorf("encode mobileconfig: %w", err)
	}
	return data, nil
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

func isBlockedProductionDomain(host string) bool {
	blocked := []string{"apple.com", "icloud.com"}
	for _, suffix := range blocked {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
