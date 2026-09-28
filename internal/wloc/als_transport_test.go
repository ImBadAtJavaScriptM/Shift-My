package wloc

import (
	"strings"
	"testing"
)

func transportTestResponse(t *testing.T) ([]byte, []ScanObservation) {
	t.Helper()

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

	scan := make([]ScanObservation, 0, len(req.BSSIDs))
	for i, bssid := range req.BSSIDs {
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: -35 - i})
	}
	return patched, scan
}

func TestALSTransportRegistryRegisteredTaskCompletion(t *testing.T) {
	response, scan := transportTestResponse(t)
	dispatcher := NewALSCompletionDispatcher(nil)
	registry := NewALSTransportRegistry(dispatcher)

	registration := ALSTransportRegistration{
		TaskID:               "BEA489C5-8877-460B-8148-1C371D37CEB0.1",
		ActivityID:           418065,
		IssuedSerial:         60,
		ParentRequesterToken: 1079203,
	}
	if err := registry.Register(registration); err != nil {
		t.Fatal(err)
	}

	got, err := registry.Complete(
		registration.TaskID,
		ALSRequesterSnapshot{
			RequesterToken:  1085649, // observed child-style token, intentionally different
			ProviderCode:    2619,
			IssuedSerial:    60,
			CompletedSerial: 51,
			Lane:            2,
		},
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResponseVersion != 1 ||
		got.ResponseFunctionID != 1 ||
		got.ResponseRecords != DefaultRichFixtureWifiRecords {
		t.Fatalf("completion=%+v", got)
	}
	if got.Registration.ParentRequesterToken == got.CompletionRequester.RequesterToken {
		t.Fatal("test must exercise parent/child requester fan-out")
	}
	if registry.PendingTasks() != 0 {
		t.Fatalf("pending tasks=%d want=0", registry.PendingTasks())
	}
	if dispatcher.PendingCompletions() != 1 {
		t.Fatalf("pending completions=%d want=1", dispatcher.PendingCompletions())
	}

	result, err := dispatcher.Flush(scan, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Resolution != "fix" ||
		result.Outcome.CandidateWorkingSet != DefaultWifiPositionMaxAPs {
		t.Fatalf("dispatch=%+v", result)
	}
}

func TestALSTransportRegistryFanoutSameIssuedSerial(t *testing.T) {
	response, scan := transportTestResponse(t)
	dispatcher := NewALSCompletionDispatcher(nil)
	registry := NewALSTransportRegistry(dispatcher)

	for _, reg := range []ALSTransportRegistration{
		{TaskID: "task-a", ActivityID: 415980, IssuedSerial: 72, ParentRequesterToken: 1310720},
		{TaskID: "task-b", ActivityID: 566745, IssuedSerial: 72, ParentRequesterToken: 1310720},
	} {
		if err := registry.Register(reg); err != nil {
			t.Fatal(err)
		}
	}

	for i, item := range []struct {
		task  string
		token uint64
		done  int
	}{
		{"task-a", 1318244, 64},
		{"task-b", 1344980, 65},
	} {
		_, err := registry.Complete(item.task, ALSRequesterSnapshot{
			RequesterToken:  item.token,
			ProviderCode:    3636,
			IssuedSerial:    72,
			CompletedSerial: item.done,
			Lane:            2,
		}, response)
		if err != nil {
			t.Fatalf("completion %d: %v", i, err)
		}
	}

	if registry.PendingTasks() != 0 || dispatcher.PendingCompletions() != 2 {
		t.Fatalf("tasks=%d completions=%d", registry.PendingTasks(), dispatcher.PendingCompletions())
	}
	result, err := dispatcher.Flush(scan, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if result.CoalescedCompletions != 2 {
		t.Fatalf("coalesced=%d want=2", result.CoalescedCompletions)
	}
}

func TestALSTransportRegistryRejectsUnregisteredResponse(t *testing.T) {
	response, _ := transportTestResponse(t)
	registry := NewALSTransportRegistry(nil)

	_, err := registry.Complete("not-registered", ALSRequesterSnapshot{
		RequesterToken:  1,
		IssuedSerial:    60,
		CompletedSerial: 51,
	}, response)
	if err == nil || !strings.Contains(err.Error(), "no registered transport task") {
		t.Fatalf("err=%v", err)
	}
}

func TestALSTransportRegistryRejectsIssuedSerialMismatch(t *testing.T) {
	response, _ := transportTestResponse(t)
	registry := NewALSTransportRegistry(nil)
	if err := registry.Register(ALSTransportRegistration{
		TaskID:               "task",
		IssuedSerial:         60,
		ParentRequesterToken: 100,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := registry.Complete("task", ALSRequesterSnapshot{
		RequesterToken:  101,
		IssuedSerial:    61,
		CompletedSerial: 51,
	}, response)
	if err == nil || !strings.Contains(err.Error(), "does not match registered serial") {
		t.Fatalf("err=%v", err)
	}
	if registry.PendingTasks() != 1 {
		t.Fatalf("mismatched response consumed task")
	}
}

func TestALSTransportRegistryRejectsReplay(t *testing.T) {
	response, _ := transportTestResponse(t)
	registry := NewALSTransportRegistry(nil)
	if err := registry.Register(ALSTransportRegistration{
		TaskID:               "task",
		IssuedSerial:         60,
		ParentRequesterToken: 100,
	}); err != nil {
		t.Fatal(err)
	}
	requester := ALSRequesterSnapshot{
		RequesterToken:  101,
		IssuedSerial:    60,
		CompletedSerial: 51,
	}
	if _, err := registry.Complete("task", requester, response); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Complete("task", requester, response); err == nil {
		t.Fatal("expected replay to be rejected after one-shot task completion")
	}
}
