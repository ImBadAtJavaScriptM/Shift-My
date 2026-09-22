package wloc

import (
	"encoding/hex"
	"math"
	"testing"
)

const capturedLegacyRequestHex = "00010005656e5f55530013636f6d2e6170706c652e6c6f636174696f6e64000a382e312e313242343131000000010000001912130a1133343a44423a46443a34333a45333a413118002001"
const compactResponseStyleFixtureHex = "000100000001000000190a1134323a37353a63333a66393a61313a3339f80101800202"
const modernStructuredRequestHex = "0001000a656e2d3030315f3030310013636f6d2e6170706c652e6c6f636174696f6e64000d31382e362e322e323247313030000000010000001912130a1134323a44423a46443a34333a45333a413118002001"
const capturedNotFoundResponseHex = "0001000000010000004312410a1133343a64623a66643a34333a65333a6131122c088098f7f8bcffffffff01108098f7f8bcffffffff0118ffffffffffffffffff0128ffffffffffffffffff01"

func TestParseCapturedLegacyRequest(t *testing.T) {
	data, err := hex.DecodeString(capturedLegacyRequestHex)
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
	if req.Locale != "en_US" || req.AppIdentifier != "com.apple.locationd" || req.OSVersion != "8.1.12B411" {
		t.Fatalf("envelope=%+v", req)
	}
	if len(req.BSSIDs) != 1 || req.BSSIDs[0] != "34:DB:FD:43:E3:A1" {
		t.Fatalf("bssids=%v", req.BSSIDs)
	}
}

func TestParseCompactResponseStyleFixtureAsLabInput(t *testing.T) {
	data, err := hex.DecodeString(compactResponseStyleFixtureHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != 1 || req.FunctionID != 1 || req.Envelope != "compact-response-style" {
		t.Fatalf("version=%d function=%d envelope=%q", req.Version, req.FunctionID, req.Envelope)
	}
	if len(req.BSSIDs) != 1 || req.BSSIDs[0] != "42:75:c3:f9:a1:39" {
		t.Fatalf("bssids=%v", req.BSSIDs)
	}
}

func TestParseModernStructuredARPCRequest(t *testing.T) {
	data, err := hex.DecodeString(modernStructuredRequestHex)
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
	if req.Locale != "en-001_001" || req.AppIdentifier != "com.apple.locationd" || req.OSVersion != "18.6.2.22G100" {
		t.Fatalf("envelope=%+v", req)
	}
	if len(req.BSSIDs) != 1 || req.BSSIDs[0] != "42:DB:FD:43:E3:A1" {
		t.Fatalf("bssids=%v", req.BSSIDs)
	}
}

func TestParseCapturedNotFoundResponse(t *testing.T) {
	data, err := hex.DecodeString(capturedNotFoundResponseHex)
	if err != nil {
		t.Fatal(err)
	}
	version, functionID, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || functionID != 1 || len(devices) != 1 {
		t.Fatalf("version=%d function=%d devices=%+v", version, functionID, devices)
	}
	if devices[0].BSSID != "34:db:fd:43:e3:a1" {
		t.Fatalf("bssid=%q", devices[0].BSSID)
	}
	if devices[0].LatitudeE8 != -18000000000 || devices[0].LongitudeE8 != -18000000000 {
		t.Fatalf("location=%+v", devices[0])
	}
}

func TestBuildResponseUsesSelectedTargetForEveryRequestedBSSID(t *testing.T) {
	req := Request{Version: 1, FunctionID: 1, BSSIDs: []string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02"}}
	data, err := BuildResponse(req, 34.146941, -118.144516)
	if err != nil {
		t.Fatal(err)
	}
	version, functionID, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || functionID != 1 || len(devices) != 2 {
		t.Fatalf("version=%d function=%d devices=%+v", version, functionID, devices)
	}
	wantLat := int64(math.Round(34.146941 * 1e8))
	wantLon := int64(math.Round(-118.144516 * 1e8))
	for i, device := range devices {
		if device.BSSID != req.BSSIDs[i] || device.LatitudeE8 != wantLat || device.LongitudeE8 != wantLon {
			t.Fatalf("device[%d]=%+v", i, device)
		}
		if device.HorizontalAccuracy != defaultHorizontalAccuracy ||
			device.UnknownValue4 != defaultUnknownValue4 ||
			device.Altitude != defaultAltitude ||
			device.VerticalAccuracy != defaultVerticalAccuracy ||
			device.MotionActivityType != defaultMotionActivityType ||
			device.MotionActivityConfidence != defaultMotionActivityConfidence {
			t.Fatalf("device[%d] metadata=%+v", i, device)
		}
	}
}

func TestParseRequestRejectsTruncatedPayload(t *testing.T) {
	data, err := hex.DecodeString(capturedLegacyRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(data[:len(data)-1]); err == nil {
		t.Fatal("expected truncated request rejection")
	}

	data, err = hex.DecodeString(compactResponseStyleFixtureHex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(data[:len(data)-1]); err == nil {
		t.Fatal("expected truncated compact request rejection")
	}
}

func TestRichResponseContainsReferenceMetadataFields(t *testing.T) {
	req := Request{Version: 1, FunctionID: 1, BSSIDs: []string{"34:DB:FD:43:E3:A1"}}
	data, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	_, _, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.HorizontalAccuracy != 39 ||
		got.UnknownValue4 != 3 ||
		got.Altitude != 530 ||
		got.VerticalAccuracy != 1000 ||
		got.MotionActivityType != 63 ||
		got.MotionActivityConfidence != 467 {
		t.Fatalf("metadata=%+v", got)
	}
}
