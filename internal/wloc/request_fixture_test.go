package wloc

import (
	"strings"
	"testing"
)

func TestBuildSyntheticStructuredRequestFixture22(t *testing.T) {
	data, err := BuildSyntheticStructuredRequestFixture(DefaultRealisticRequestWifiRecords)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != 1 || req.FunctionID != 1 || req.Envelope != "structured-arpc" {
		t.Fatalf("version=%d function=%d envelope=%q", req.Version, req.FunctionID, req.Envelope)
	}
	if req.Locale != "en-001_001" || req.AppIdentifier != "com.apple.locationd" || req.OSVersion != "26.6.2.23G90" {
		t.Fatalf("envelope metadata=%+v", req)
	}
	if len(req.BSSIDs) != DefaultRealisticRequestWifiRecords {
		t.Fatalf("bssids=%d want=%d", len(req.BSSIDs), DefaultRealisticRequestWifiRecords)
	}
	if req.BSSIDs[0] != "02:54:4d:00:00:00" || req.BSSIDs[21] != "02:54:4d:00:00:15" {
		t.Fatalf("first=%q last=%q", req.BSSIDs[0], req.BSSIDs[21])
	}
	for _, bssid := range req.BSSIDs {
		if !strings.HasPrefix(strings.ToLower(bssid), "02:54:4d:") {
			t.Fatalf("non-synthetic bssid=%q", bssid)
		}
	}
}
