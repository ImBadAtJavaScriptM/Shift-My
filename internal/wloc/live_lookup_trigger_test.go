package wloc

import "testing"

func syntheticLookupScan(n int) []ScanObservation {
	scan := make([]ScanObservation, 0, n)
	for i := 0; i < n; i++ {
		scan = append(scan, ScanObservation{
			BSSID: syntheticTestBSSID(i),
			RSSI:  -35 - i,
		})
	}
	return scan
}

func TestLiveWifiLookupStage2Observed22And27(t *testing.T) {
	for _, n := range []int{22, 27} {
		state := &LiveWifiLookupTriggerState{}
		got, err := state.EvaluateStage2InitialFinal(syntheticLookupScan(n), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !got.ShouldQuery ||
			got.Reason != ObservedLookupReasonUnknownRatio ||
			got.TotalScan != n ||
			got.NotReturned != n ||
			got.NotReturnedPercent != 100 ||
			len(got.RequestBSSIDs) != n {
			t.Fatalf("n=%d decision=%+v", n, got)
		}
	}
}

func TestLiveWifiLookupStage2IsOneShotPerCycle(t *testing.T) {
	state := &LiveWifiLookupTriggerState{}
	scan := syntheticLookupScan(22)

	first, err := state.EvaluateStage2InitialFinal(scan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ShouldQuery {
		t.Fatalf("first=%+v", first)
	}

	repeat, err := state.EvaluateStage2InitialFinal(scan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.ShouldQuery || !repeat.OneShotSuppressed {
		t.Fatalf("repeat=%+v", repeat)
	}

	state.ResetPositioningCycle()
	next, err := state.EvaluateStage2InitialFinal(scan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !next.ShouldQuery || next.OneShotSuppressed {
		t.Fatalf("next=%+v", next)
	}
}

func TestLiveWifiLookupStage2ObservedPositiveCaseRequiresAllNotReturned(t *testing.T) {
	state := &LiveWifiLookupTriggerState{}
	scan := syntheticLookupScan(3)
	cached := []DeviceLocation{
		{
			BSSID:              scan[0].BSSID,
			LatitudeE8:         3400000000,
			LongitudeE8:        -11800000000,
			HorizontalAccuracy: 20,
		},
	}
	got, err := state.EvaluateStage2InitialFinal(scan, cached)
	if err != nil {
		t.Fatal(err)
	}
	if got.ShouldQuery || got.NotReturned != 2 || got.NotReturnedPercent != 66 {
		t.Fatalf("decision=%+v", got)
	}
}

func TestEvaluateWSBLiveObservedThreeAPPath(t *testing.T) {
	scan := syntheticLookupScan(3)
	got, err := EvaluateWSBLive(scan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ShouldQuery ||
		got.Reason != ObservedLookupReasonWSBLive ||
		got.TotalScan != 3 ||
		got.NotReturned != 3 ||
		got.NotReturnedPercent != 100 ||
		len(got.RequestBSSIDs) != 3 {
		t.Fatalf("decision=%+v", got)
	}
}

func TestEvaluateWSBLiveDoesNotClaimTriggerWithUsableOverlap(t *testing.T) {
	scan := syntheticLookupScan(3)
	cached := []DeviceLocation{{
		BSSID:              scan[0].BSSID,
		LatitudeE8:         3400000000,
		LongitudeE8:        -11800000000,
		HorizontalAccuracy: 20,
	}}
	got, err := EvaluateWSBLive(scan, cached)
	if err != nil {
		t.Fatal(err)
	}
	if got.ShouldQuery {
		t.Fatalf("decision=%+v", got)
	}
}

func TestLiveLookupRequestBSSIDOrderDeduplicates(t *testing.T) {
	scan := []ScanObservation{
		{BSSID: "02:00:00:00:00:01", RSSI: -80},
		{BSSID: "02:00:00:00:00:02", RSSI: -50},
		{BSSID: "02:00:00:00:00:01", RSSI: -40},
	}
	got, err := EvaluateWSBLive(scan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RequestBSSIDs) != 2 ||
		got.RequestBSSIDs[0] != "02:00:00:00:00:01" ||
		got.RequestBSSIDs[1] != "02:00:00:00:00:02" {
		t.Fatalf("request BSSIDs=%v", got.RequestBSSIDs)
	}
}
