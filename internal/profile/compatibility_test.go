package profile

import (
	"encoding/json"
	"testing"

	"howett.net/plist"
)

// This locks the generated profiles to the payload shape observed in the
// reference ShiftMy profiles, while deliberately substituting only
// project-controlled DNS names and URLs.
func TestReferenceProfileShapeCompatibility(t *testing.T) {
	cfg := testConfig(t)

	stage1, err := GenerateStage1(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	if _, err := plist.Unmarshal(stage1, &first); err != nil {
		t.Fatal(err)
	}
	firstPayloads := payloadList(t, first)
	wantStage1Types := []string{
		"com.apple.declarations",
		"com.apple.dnsSettings.managed",
		"com.apple.security.acme",
	}
	if len(firstPayloads) != len(wantStage1Types) {
		t.Fatalf("stage 1 payload count=%d", len(firstPayloads))
	}
	for i, want := range wantStage1Types {
		payload, ok := firstPayloads[i].(map[string]any)
		if !ok || payload["PayloadType"] != want {
			t.Fatalf("stage 1 payload %d type=%v want=%q", i, payload["PayloadType"], want)
		}
	}

	declPayload := firstPayloads[0].(map[string]any)
	decls := declPayload["Declarations"].([]any)
	seenLegacy := false
	seenActivation := false
	for _, raw := range decls {
		var d map[string]any
		if err := json.Unmarshal(raw.([]byte), &d); err != nil {
			t.Fatal(err)
		}
		switch d["Type"] {
		case "com.apple.configuration.legacy":
			seenLegacy = true
			if d["ServerToken"] == "" {
				t.Fatal("legacy declaration ServerToken missing")
			}
		case "com.apple.activation.simple":
			seenActivation = true
			if d["ServerToken"] == "" {
				t.Fatal("activation declaration ServerToken missing")
			}
		}
	}
	if !seenLegacy || !seenActivation {
		t.Fatalf("legacy=%v activation=%v", seenLegacy, seenActivation)
	}

	acme := firstPayloads[2].(map[string]any)
	if acme["KeyType"] != "ECSECPrimeRandom" || acme["KeySize"] != uint64(256) && acme["KeySize"] != int64(256) && acme["KeySize"] != 256 {
		t.Fatalf("ACME key configuration=%v/%v", acme["KeyType"], acme["KeySize"])
	}
	if acme["UsageFlags"] != uint64(1) && acme["UsageFlags"] != int64(1) && acme["UsageFlags"] != 1 {
		t.Fatalf("UsageFlags=%v", acme["UsageFlags"])
	}
	eku, ok := acme["ExtendedKeyUsage"].([]any)
	if !ok || len(eku) != 1 || eku[0] != "1.3.6.1.5.5.7.3.2" {
		t.Fatalf("ExtendedKeyUsage=%#v", acme["ExtendedKeyUsage"])
	}

	stage2, err := GenerateStage2(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var second map[string]any
	if _, err := plist.Unmarshal(stage2, &second); err != nil {
		t.Fatal(err)
	}
	secondPayloads := payloadList(t, second)
	wantStage2Types := []string{
		"com.apple.security.root",
		"com.apple.dnsSettings.managed",
	}
	if len(secondPayloads) != len(wantStage2Types) {
		t.Fatalf("stage 2 payload count=%d", len(secondPayloads))
	}
	for i, want := range wantStage2Types {
		payload, ok := secondPayloads[i].(map[string]any)
		if !ok || payload["PayloadType"] != want {
			t.Fatalf("stage 2 payload %d type=%v want=%q", i, payload["PayloadType"], want)
		}
	}
}
