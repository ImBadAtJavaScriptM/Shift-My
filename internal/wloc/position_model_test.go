package wloc

import (
	"math"
	"testing"
)

func TestEstimateWifiPositionPatchRichIntegration(t *testing.T) {
	requestBytes, err := BuildSyntheticStructuredRequestFixture(DefaultRealisticRequestWifiRecords)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(requestBytes)
	if err != nil {
		t.Fatal(err)
	}

	fixture, err := BuildRichResponseFixture(req, DefaultRichFixtureWifiRecords)
	if err != nil {
		t.Fatal(err)
	}
	patched, _, err := PatchResponseCoordinatesOnly(fixture, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	_, _, devices, err := ParseResponse(patched)
	if err != nil {
		t.Fatal(err)
	}

	scan := make([]ScanObservation, 0, len(req.BSSIDs))
	for i, bssid := range req.BSSIDs {
		scan = append(scan, ScanObservation{
			BSSID: bssid,
			RSSI:  -35 - i*2,
		})
	}

	estimate, err := EstimateWifiPosition(scan, devices, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.MatchedAPs != 22 {
		t.Fatalf("matched=%d want=22", estimate.MatchedAPs)
	}
	if estimate.UsedAPs != DefaultWifiPositionMaxAPs {
		t.Fatalf("used=%d want=%d", estimate.UsedAPs, DefaultWifiPositionMaxAPs)
	}
	if math.Abs(estimate.Latitude-34.0094) > 1e-8 ||
		math.Abs(estimate.Longitude-(-118.4973)) > 1e-8 {
		t.Fatalf("estimate=(%.8f, %.8f)", estimate.Latitude, estimate.Longitude)
	}
	if estimate.SpreadMeters > 0.001 {
		t.Fatalf("spread=%f want approximately zero", estimate.SpreadMeters)
	}
	if estimate.Method != "empirical-top18-rssi-accuracy-weighted" {
		t.Fatalf("method=%q", estimate.Method)
	}
}

func TestEstimateWifiPositionUsesStrongestDuplicateAndIgnoresMissing(t *testing.T) {
	scan := []ScanObservation{
		{BSSID: "02:00:00:00:00:01", RSSI: -80},
		{BSSID: "02:00:00:00:00:01", RSSI: -40},
		{BSSID: "02:00:00:00:00:02", RSSI: -50},
		{BSSID: "02:00:00:00:00:03", RSSI: -30},
	}
	response := []DeviceLocation{
		{BSSID: "02:00:00:00:00:01", LatitudeE8: 1000000000, LongitudeE8: 2000000000, HorizontalAccuracy: 20},
		{BSSID: "02:00:00:00:00:02", LatitudeE8: 3000000000, LongitudeE8: 4000000000, HorizontalAccuracy: 20},
		{BSSID: "02:00:00:00:00:03"}, // no Location submessage
	}

	estimate, err := EstimateWifiPosition(scan, response, 1)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.MatchedAPs != 2 || estimate.UsedAPs != 1 {
		t.Fatalf("matched=%d used=%d", estimate.MatchedAPs, estimate.UsedAPs)
	}
	if estimate.Latitude != 10 || estimate.Longitude != 20 {
		t.Fatalf("estimate=(%f,%f)", estimate.Latitude, estimate.Longitude)
	}
	if estimate.StrongestRSSI != -40 || estimate.WeakestUsedRSSI != -40 {
		t.Fatalf("RSSI strongest=%d weakest=%d", estimate.StrongestRSSI, estimate.WeakestUsedRSSI)
	}
}

func TestEstimateWifiPositionNoOverlap(t *testing.T) {
	_, err := EstimateWifiPosition(
		[]ScanObservation{{BSSID: "02:00:00:00:00:01", RSSI: -40}},
		[]DeviceLocation{{BSSID: "02:00:00:00:00:02", LatitudeE8: 1000000000, LongitudeE8: 2000000000, HorizontalAccuracy: 20}},
		DefaultWifiPositionMaxAPs,
	)
	if err == nil {
		t.Fatal("expected no-overlap error")
	}
}
