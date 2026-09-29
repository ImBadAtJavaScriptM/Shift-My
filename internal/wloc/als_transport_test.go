package wloc

import (
	"strings"
	"testing"
)

func transportTestResponse(t *testing.T, count int) ([]byte, []ScanObservation) {
	t.Helper()

	requestBytes, err := BuildSyntheticStructuredRequestFixture(DefaultRealisticRequestWifiRecords)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(requestBytes)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := BuildRichResponseFixture(req, count)
	if err != nil {
		t.Fatal(err)
	}
	patched, _, err := PatchResponseCoordinatesOnly(fixture, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}

	scan := make([]ScanObservation, 0, len(req.BSSIDs))
	for i, bssid := range req.BSSIDs {
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: -35 - i})
	}
	return patched, scan
}

func liveOrigin(id int, reason string) ALSQueryOrigin {
	return ALSQueryOrigin{
		OriginID: id,
		Kind:     ALSQueryOriginLiveWifi,
		Reason:   reason,
	}
}

func backgroundOrigin(id int) ALSQueryOrigin {
	return ALSQueryOrigin{
		OriginID:            id,
		Kind:                ALSQueryOriginCoordinateNeighborhood,
		HasCoordinateCenter: true,
	}
}

func TestALSTransportRegistryLiveTaskCanAdvanceSerialBeforeCompletion(t *testing.T) {
	response, scan := transportTestResponse(t, DefaultRichFixtureWifiRecords)
	dispatcher := NewALSCompletionDispatcher(nil)
	registry := NewALSTransportRegistry(dispatcher)

	// Mirrors the clean iOS 26 Stage2 transaction: the task is created beside
	// issued serial 54, but the same CFNetwork task later completes under 60.
	registration := ALSTransportRegistration{
		TaskID:               "BEA489C5-8877-460B-8148-1C371D37CEB0.1",
		ActivityID:           418065,
		Origin:               liveOrigin(53, ObservedLookupReasonUnknownRatio),
		IssuedSerial:         54,
		ParentRequesterToken: 928939,
	}
	if err := registry.Register(registration); err != nil {
		t.Fatal(err)
	}

	got, err := registry.Complete(
		registration.TaskID,
		ALSRequesterSnapshot{
			RequesterToken:  1085649,
			ProviderCode:    2619,
			IssuedSerial:    60,
			CompletedSerial: 51,
			Lane:            2,
		},
		ALSResponseFamilySummary{
			OriginID:    53,
			RecordCount: DefaultRichFixtureWifiRecords,
			FamilyFlag:  ObservedALSResponseFamilyLive,
		},
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DeliveredToWifi ||
		got.Route.Consumer != "wifi-position-live" ||
		got.SerialAdvance != 6 ||
		got.ResponseRecords != DefaultRichFixtureWifiRecords {
		t.Fatalf("completion=%+v", got)
	}
	if registry.PendingTasks() != 0 || dispatcher.PendingCompletions() != 1 {
		t.Fatalf("tasks=%d completions=%d", registry.PendingTasks(), dispatcher.PendingCompletions())
	}

	result, err := dispatcher.Flush(scan, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Resolution != "fix" {
		t.Fatalf("dispatch=%+v", result)
	}
}

func TestALSTransportRegistryRoutesHeterogeneousChildren(t *testing.T) {
	liveResponse, scan := transportTestResponse(t, DefaultRichFixtureWifiRecords)
	backgroundResponse, _ := transportTestResponse(t, 400)
	dispatcher := NewALSCompletionDispatcher(nil)
	registry := NewALSTransportRegistry(dispatcher)

	// One later ALS serial can have heterogeneous child responses. The live
	// child belongs to WifiPosition; the coordinate-neighborhood child belongs
	// to background place/geofence work.
	for _, reg := range []ALSTransportRegistration{
		{
			TaskID:               "task-live",
			ActivityID:           418065,
			Origin:               liveOrigin(53, ObservedLookupReasonUnknownRatio),
			IssuedSerial:         54,
			ParentRequesterToken: 928939,
		},
		{
			TaskID:               "task-background",
			ActivityID:           415980,
			Origin:               backgroundOrigin(55),
			IssuedSerial:         56,
			ParentRequesterToken: 978498,
		},
	} {
		if err := registry.Register(reg); err != nil {
			t.Fatal(err)
		}
	}

	live, err := registry.Complete(
		"task-live",
		ALSRequesterSnapshot{
			RequesterToken:  1085649,
			ProviderCode:    2619,
			IssuedSerial:    60,
			CompletedSerial: 51,
			Lane:            2,
		},
		ALSResponseFamilySummary{
			OriginID: 53, RecordCount: DefaultRichFixtureWifiRecords, FamilyFlag: ObservedALSResponseFamilyLive,
		},
		liveResponse,
	)
	if err != nil {
		t.Fatal(err)
	}
	background, err := registry.Complete(
		"task-background",
		ALSRequesterSnapshot{
			RequesterToken:  1092913,
			ProviderCode:    2619,
			IssuedSerial:    60,
			CompletedSerial: 52,
			Lane:            2,
		},
		ALSResponseFamilySummary{
			OriginID: 55, RecordCount: 400, FamilyFlag: ObservedALSResponseFamilyBackground,
		},
		backgroundResponse,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !live.DeliveredToWifi || background.DeliveredToWifi {
		t.Fatalf("live=%+v background=%+v", live, background)
	}
	if background.Route.Consumer != "background-neighborhood" {
		t.Fatalf("background route=%+v", background.Route)
	}
	if dispatcher.PendingCompletions() != 1 {
		t.Fatalf("only live child should reach WifiPosition; pending=%d", dispatcher.PendingCompletions())
	}
	if _, err := dispatcher.Flush(scan, DefaultWifiPositionMaxAPs); err != nil {
		t.Fatal(err)
	}
}

func TestALSTransportRegistryOneRequesterSpansMultipleIssuedSerials(t *testing.T) {
	registry := NewALSTransportRegistry(nil)

	for i, reg := range []ALSTransportRegistration{
		{TaskID: "task-serial-a", ActivityID: 1, Origin: backgroundOrigin(100), IssuedSerial: 64, ParentRequesterToken: 7001},
		{TaskID: "task-serial-b", ActivityID: 1, Origin: backgroundOrigin(101), IssuedSerial: 65, ParentRequesterToken: 7001},
		{TaskID: "task-serial-c", ActivityID: 1, Origin: backgroundOrigin(102), IssuedSerial: 66, ParentRequesterToken: 7001},
	} {
		if err := registry.Register(reg); err != nil {
			t.Fatalf("registration %d: %v", i, err)
		}
	}

	if registry.PendingTasks() != 3 {
		t.Fatalf("pending tasks=%d want=3", registry.PendingTasks())
	}
}

func TestALSTransportRegistryRejectsUnregisteredResponse(t *testing.T) {
	response, _ := transportTestResponse(t, DefaultRichFixtureWifiRecords)
	registry := NewALSTransportRegistry(nil)

	_, err := registry.Complete(
		"not-registered",
		ALSRequesterSnapshot{RequesterToken: 1, IssuedSerial: 60, CompletedSerial: 51},
		ALSResponseFamilySummary{OriginID: 53, RecordCount: DefaultRichFixtureWifiRecords, FamilyFlag: ObservedALSResponseFamilyLive},
		response,
	)
	if err == nil || !strings.Contains(err.Error(), "no registered transport task") {
		t.Fatalf("err=%v", err)
	}
}

func TestALSTransportRegistryRejectsOriginMismatch(t *testing.T) {
	response, _ := transportTestResponse(t, DefaultRichFixtureWifiRecords)
	registry := NewALSTransportRegistry(nil)
	if err := registry.Register(ALSTransportRegistration{
		TaskID:               "task",
		Origin:               liveOrigin(53, ObservedLookupReasonUnknownRatio),
		IssuedSerial:         54,
		ParentRequesterToken: 100,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := registry.Complete(
		"task",
		ALSRequesterSnapshot{RequesterToken: 101, IssuedSerial: 60, CompletedSerial: 51},
		ALSResponseFamilySummary{OriginID: 54, RecordCount: DefaultRichFixtureWifiRecords, FamilyFlag: ObservedALSResponseFamilyLive},
		response,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match request origin") {
		t.Fatalf("err=%v", err)
	}
	if registry.PendingTasks() != 1 {
		t.Fatal("origin-mismatched response consumed task")
	}
}

func TestALSTransportRegistryRejectsSummaryCountMismatch(t *testing.T) {
	response, _ := transportTestResponse(t, DefaultRichFixtureWifiRecords)
	registry := NewALSTransportRegistry(nil)
	if err := registry.Register(ALSTransportRegistration{
		TaskID:               "task",
		Origin:               liveOrigin(53, ObservedLookupReasonUnknownRatio),
		IssuedSerial:         54,
		ParentRequesterToken: 100,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := registry.Complete(
		"task",
		ALSRequesterSnapshot{RequesterToken: 101, IssuedSerial: 60, CompletedSerial: 51},
		ALSResponseFamilySummary{OriginID: 53, RecordCount: 999, FamilyFlag: ObservedALSResponseFamilyLive},
		response,
	)
	if err == nil || !strings.Contains(err.Error(), "do not match decoded records") {
		t.Fatalf("err=%v", err)
	}
	if registry.PendingTasks() != 1 {
		t.Fatal("count-mismatched response consumed task")
	}
}

func TestALSTransportRegistryRejectsReplay(t *testing.T) {
	response, _ := transportTestResponse(t, DefaultRichFixtureWifiRecords)
	registry := NewALSTransportRegistry(nil)
	registration := ALSTransportRegistration{
		TaskID:               "task",
		Origin:               liveOrigin(53, ObservedLookupReasonUnknownRatio),
		IssuedSerial:         54,
		ParentRequesterToken: 100,
	}
	if err := registry.Register(registration); err != nil {
		t.Fatal(err)
	}
	requester := ALSRequesterSnapshot{RequesterToken: 101, IssuedSerial: 60, CompletedSerial: 51}
	summary := ALSResponseFamilySummary{OriginID: 53, RecordCount: DefaultRichFixtureWifiRecords, FamilyFlag: ObservedALSResponseFamilyLive}
	if _, err := registry.Complete("task", requester, summary, response); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Complete("task", requester, summary, response); err == nil {
		t.Fatal("expected replay to be rejected after one-shot task completion")
	}
}
