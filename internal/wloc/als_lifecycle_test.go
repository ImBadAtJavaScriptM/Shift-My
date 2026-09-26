package wloc

import (
	"math"
	"testing"
)

func TestEvaluateALSLifecycleFixAfterCompletedRequester(t *testing.T) {
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
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: -35 - i})
	}

	outcome, err := EvaluateALSLifecycle(scan, devices, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Event != "Network::AlsFinished" || outcome.Resolution != "fix" {
		t.Fatalf("outcome=%+v", outcome)
	}
	if outcome.MatchedAPs != 22 || outcome.Estimate == nil {
		t.Fatalf("outcome=%+v", outcome)
	}
	if math.Abs(outcome.Estimate.Latitude-34.0094) > 1e-8 || math.Abs(outcome.Estimate.Longitude-(-118.4973)) > 1e-8 {
		t.Fatalf("estimate=%+v", outcome.Estimate)
	}
}

func TestEvaluateALSLifecycleAllUnknownAfterCompletedRequester(t *testing.T) {
	scan := []ScanObservation{{BSSID: "02:00:00:00:00:01", RSSI: -40}}
	response := []DeviceLocation{{
		BSSID:              "02:00:00:00:00:02",
		LatitudeE8:         3400940000,
		LongitudeE8:        -11849730000,
		HorizontalAccuracy: 20,
	}}

	outcome, err := EvaluateALSLifecycle(scan, response, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Event != "Network::AlsFinished" ||
		outcome.Resolution != "Network::AlsAllUnknown" ||
		outcome.MatchedAPs != 0 ||
		outcome.Estimate != nil {
		t.Fatalf("outcome=%+v", outcome)
	}
}
