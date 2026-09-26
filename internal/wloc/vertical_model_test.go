package wloc

import (
	"math"
	"testing"
)

func TestEstimateWifiVerticalPositionCapsAtTenAndFiltersAccuracy(t *testing.T) {
	scan := make([]ScanObservation, 0, 14)
	response := make([]DeviceLocation, 0, 14)

	for i := 0; i < 14; i++ {
		bssid := syntheticTestBSSID(i)
		rssi := -35 - i*3
		altitude := int64(100 + i)
		verticalAccuracy := int64(4)

		// Two otherwise-strong entries fail the observed <=4m quality gate.
		if i == 1 || i == 3 {
			verticalAccuracy = 10
		}
		// Make the weakest two qualified entries extreme outliers; top-10
		// selection must keep them out when more than ten qualify.
		if i >= 12 {
			altitude = 1000
		}

		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: rssi})
		response = append(response, DeviceLocation{
			BSSID:            bssid,
			Altitude:         altitude,
			VerticalAccuracy: verticalAccuracy,
		})
	}

	got, err := EstimateWifiVerticalPosition(scan, response, DefaultWifiVerticalMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if got.MatchedAPs != 14 {
		t.Fatalf("matched=%d want=14", got.MatchedAPs)
	}
	if got.QualifiedAPs != 12 {
		t.Fatalf("qualified=%d want=12", got.QualifiedAPs)
	}
	if got.UsedAPs != 10 {
		t.Fatalf("used=%d want=10", got.UsedAPs)
	}
	if got.Method != "empirical-vacc4-top10-rssi-weighted" {
		t.Fatalf("method=%q", got.Method)
	}
}

func TestEstimateWifiVerticalPositionAnonymizedCaptureA(t *testing.T) {
	// Relative altitude/RSSI/vertical-accuracy pattern translated away from the
	// source capture. The expected result preserves only the observed solver
	// geometry, not the original physical altitude.
	fixtures := []struct {
		alt, vacc float64
		rssi      int
	}{
		{1000.8, 4, -79},
		{1000.3, 4, -33},
		{1000.2, 4, -33},
		{999.6, 4, -61},
		{999.7, 4, -61},
		{1001.9, 4, -93},
		{1000.3, 4, -58},
		{1000.3, 4, -58},
		{1000.3, 4, -92},
	}
	scan := make([]ScanObservation, 0, len(fixtures))
	response := make([]DeviceLocation, 0, len(fixtures))
	for i, f := range fixtures {
		bssid := syntheticTestBSSID(i)
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: f.rssi})
		response = append(response, DeviceLocation{
			BSSID:            bssid,
			Altitude:         int64(math.Round(f.alt * 10)),
			VerticalAccuracy: int64(math.Round(f.vacc)),
		})
	}
	// DeviceLocation altitude is integer meters on wire; use an integer-shifted
	// fixture for a stable regression of selection/weighting behavior.
	for i := range response {
		response[i].Altitude = int64(math.Round(fixtures[i].alt))
	}

	got, err := EstimateWifiVerticalPosition(scan, response, DefaultWifiVerticalMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if got.UsedAPs != 9 {
		t.Fatalf("used=%d want=9", got.UsedAPs)
	}
	if got.Altitude < 999.5 || got.Altitude > 1001.0 {
		t.Fatalf("altitude=%f outside expected translated range", got.Altitude)
	}
}

func TestEstimateWifiVerticalPositionNoQualifiedOverlap(t *testing.T) {
	_, err := EstimateWifiVerticalPosition(
		[]ScanObservation{{BSSID: "02:00:00:00:00:01", RSSI: -40}},
		[]DeviceLocation{{
			BSSID:            "02:00:00:00:00:01",
			Altitude:         100,
			VerticalAccuracy: 10,
		}},
		DefaultWifiVerticalMaxAPs,
	)
	if err == nil {
		t.Fatal("expected no high-quality vertical overlap error")
	}
}
