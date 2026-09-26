package wloc

import (
	"errors"
	"testing"
)

func TestALSRequesterSnapshotOutstandingHint(t *testing.T) {
	snapshot := ALSRequesterSnapshot{
		RequesterToken:  1279524,
		ProviderCode:    3636,
		IssuedSerial:    71,
		CompletedSerial: 61,
		Lane:            2,
	}
	if got := snapshot.OutstandingHint(); got != 10 {
		t.Fatalf("outstanding hint=%d want=10", got)
	}
}

func TestALSCompletionDispatcherSingleCompletion(t *testing.T) {
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
	_, _, response, err := ParseResponse(patched)
	if err != nil {
		t.Fatal(err)
	}

	scan := make([]ScanObservation, 0, len(req.BSSIDs))
	for i, bssid := range req.BSSIDs {
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: -35 - i})
	}

	dispatcher := NewALSCompletionDispatcher(nil)
	err = dispatcher.Complete(ALSCompletion{
		Requester: ALSRequesterSnapshot{
			RequesterToken:  1310720,
			ProviderCode:    3636,
			IssuedSerial:    72,
			CompletedSerial: 63,
			Lane:            2,
		},
		Response: response,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dispatcher.PendingCompletions() != 1 {
		t.Fatalf("pending=%d want=1", dispatcher.PendingCompletions())
	}

	result, err := dispatcher.Flush(scan, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Event != "Network::AlsFinished" ||
		result.Cause != "completed-requester" ||
		result.CoalescedCompletions != 1 ||
		len(result.Requesters) != 1 ||
		result.Requesters[0].RequesterToken != 1310720 {
		t.Fatalf("result=%+v", result)
	}
	if result.Outcome.Resolution != "fix" ||
		result.Outcome.CandidateWorkingSet != 18 ||
		result.Outcome.RejectedByWorkingSetCap != 4 {
		t.Fatalf("outcome=%+v", result.Outcome)
	}
	if dispatcher.PendingCompletions() != 0 {
		t.Fatalf("pending after flush=%d", dispatcher.PendingCompletions())
	}
}

func TestALSCompletionDispatcherCoalescesMultipleRequesters(t *testing.T) {
	service := NewALSAccessPointLocationService()
	dispatcher := NewALSCompletionDispatcher(service)

	first := DeviceLocation{
		BSSID:              "02:00:00:00:00:01",
		LatitudeE8:         3400940000,
		LongitudeE8:        -11849730000,
		HorizontalAccuracy: 20,
	}
	second := DeviceLocation{
		BSSID:              "02:00:00:00:00:02",
		LatitudeE8:         3400940000,
		LongitudeE8:        -11849730000,
		HorizontalAccuracy: 20,
	}
	if err := dispatcher.Complete(ALSCompletion{
		Requester: ALSRequesterSnapshot{
			RequesterToken:  1318244,
			ProviderCode:    3636,
			IssuedSerial:    72,
			CompletedSerial: 64,
			Lane:            2,
		},
		Response: []DeviceLocation{first},
	}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Complete(ALSCompletion{
		Requester: ALSRequesterSnapshot{
			RequesterToken:  1344980,
			ProviderCode:    3636,
			IssuedSerial:    72,
			CompletedSerial: 65,
			Lane:            2,
		},
		Response: []DeviceLocation{second},
	}); err != nil {
		t.Fatal(err)
	}

	if dispatcher.PendingCompletions() != 2 {
		t.Fatalf("pending=%d want=2", dispatcher.PendingCompletions())
	}

	result, err := dispatcher.Flush([]ScanObservation{
		{BSSID: first.BSSID, RSSI: -40},
		{BSSID: second.BSSID, RSSI: -45},
	}, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if result.CoalescedCompletions != 2 ||
		len(result.Requesters) != 2 ||
		result.CachedLocations != 2 {
		t.Fatalf("result=%+v", result)
	}
	if result.Requesters[0].RequesterToken != 1318244 ||
		result.Requesters[1].RequesterToken != 1344980 {
		t.Fatalf("requesters=%+v", result.Requesters)
	}
	if result.Outcome.Resolution != "fix" ||
		result.Outcome.ALSLocated != 2 ||
		result.Outcome.CandidateWorkingSet != 2 {
		t.Fatalf("outcome=%+v", result.Outcome)
	}
}

func TestALSAccessPointLocationServicePrefersUsableAndMoreAccurate(t *testing.T) {
	service := NewALSAccessPointLocationService()
	bssid := "02:00:00:00:00:01"

	service.MergeResponse([]DeviceLocation{{BSSID: bssid}})
	service.MergeResponse([]DeviceLocation{{
		BSSID:              bssid,
		LatitudeE8:         1000000000,
		LongitudeE8:        2000000000,
		HorizontalAccuracy: 40,
	}})
	service.MergeResponse([]DeviceLocation{{
		BSSID:              bssid,
		LatitudeE8:         1100000000,
		LongitudeE8:        2100000000,
		HorizontalAccuracy: 20,
	}})
	service.MergeResponse([]DeviceLocation{{
		BSSID:              bssid,
		LatitudeE8:         1200000000,
		LongitudeE8:        2200000000,
		HorizontalAccuracy: 80,
	}})

	snapshot := service.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot[0].LatitudeE8 != 1100000000 ||
		snapshot[0].LongitudeE8 != 2100000000 ||
		snapshot[0].HorizontalAccuracy != 20 {
		t.Fatalf("cached=%+v", snapshot[0])
	}
}

func TestALSCompletionDispatcherAllUnknownAfterFlush(t *testing.T) {
	dispatcher := NewALSCompletionDispatcher(nil)
	if err := dispatcher.Complete(ALSCompletion{
		Requester: ALSRequesterSnapshot{
			RequesterToken:  1644217,
			ProviderCode:    3798,
			IssuedSerial:    80,
			CompletedSerial: 77,
			Lane:            2,
		},
		Response: []DeviceLocation{{
			BSSID: "02:00:00:00:00:01",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := dispatcher.Flush([]ScanObservation{
		{BSSID: "02:00:00:00:00:01", RSSI: -40},
		{BSSID: "02:00:00:00:00:02", RSSI: -50},
	}, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Resolution != "Network::AlsAllUnknown" ||
		result.Outcome.HasUsableOverlap ||
		result.Outcome.Estimate != nil {
		t.Fatalf("result=%+v", result)
	}
}

func TestALSCompletionDispatcherRejectsFlushWithoutCompletion(t *testing.T) {
	dispatcher := NewALSCompletionDispatcher(nil)
	_, err := dispatcher.Flush(
		[]ScanObservation{{BSSID: "02:00:00:00:00:01", RSSI: -40}},
		DefaultWifiPositionMaxAPs,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, err) { // keep the assertion side-effect-free; message is checked below
		t.Fatal(err)
	}
	if err.Error() != "no completed ALS requester is pending" {
		t.Fatalf("error=%q", err)
	}
}

func TestALSCompletionDispatcherCachedReevaluation(t *testing.T) {
	service := NewALSAccessPointLocationService()
	dispatcher := NewALSCompletionDispatcher(service)
	device := DeviceLocation{
		BSSID:              "02:00:00:00:00:01",
		LatitudeE8:         3400940000,
		LongitudeE8:        -11849730000,
		HorizontalAccuracy: 20,
	}
	scan := []ScanObservation{{BSSID: device.BSSID, RSSI: -45}}

	if _, err := dispatcher.ReevaluateCached(scan, DefaultWifiPositionMaxAPs); err == nil {
		t.Fatal("expected cached re-evaluation to fail before initial dispatch")
	}
	if err := dispatcher.Complete(ALSCompletion{
		Requester: ALSRequesterSnapshot{
			RequesterToken:  2000001,
			ProviderCode:    3636,
			IssuedSerial:    81,
			CompletedSerial: 78,
			Lane:            2,
		},
		Response: []DeviceLocation{device},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.ReevaluateCached(scan, DefaultWifiPositionMaxAPs); err == nil {
		t.Fatal("expected cached re-evaluation to fail while a completion is pending")
	}
	first, err := dispatcher.Flush(scan, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cause != "completed-requester" || first.Outcome.Resolution != "fix" {
		t.Fatalf("first=%+v", first)
	}

	repeat, err := dispatcher.ReevaluateCached(scan, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Event != "Network::AlsFinished" ||
		repeat.Cause != "cached-reevaluation" ||
		repeat.CoalescedCompletions != 0 ||
		len(repeat.Requesters) != 0 ||
		repeat.Outcome.Resolution != "fix" {
		t.Fatalf("repeat=%+v", repeat)
	}

	unknown, err := dispatcher.ReevaluateCached(
		[]ScanObservation{{BSSID: "02:00:00:00:00:02", RSSI: -40}},
		DefaultWifiPositionMaxAPs,
	)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Outcome.Resolution != "Network::AlsAllUnknown" {
		t.Fatalf("unknown=%+v", unknown)
	}
}
