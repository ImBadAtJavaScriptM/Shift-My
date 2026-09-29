package wloc

import "testing"

func TestCorrelateALSConsumerRouteStage2LiveDoesNotDependOnRecordCount(t *testing.T) {
	origin := ALSQueryOrigin{
		OriginID: 53,
		Kind:     ALSQueryOriginLiveWifi,
		Reason:   ObservedLookupReasonUnknownRatio,
	}
	for _, records := range []int{114, 118, 7} {
		got, err := CorrelateALSConsumerRoute(origin, ALSResponseFamilySummary{
			OriginID:    origin.OriginID,
			RecordCount: records,
			FamilyFlag:  ObservedALSResponseFamilyLive,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Consumer != "wifi-position-live" {
			t.Fatalf("records=%d route=%+v", records, got)
		}
	}
}

func TestCorrelateALSConsumerRouteWSBLive(t *testing.T) {
	got, err := CorrelateALSConsumerRoute(
		ALSQueryOrigin{
			OriginID: 79,
			Kind:     ALSQueryOriginLiveWifi,
			Reason:   ObservedLookupReasonWSBLive,
		},
		ALSResponseFamilySummary{
			OriginID:    79,
			RecordCount: 4,
			FamilyFlag:  ObservedALSResponseFamilyLive,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Consumer != "wifi-position-live" {
		t.Fatalf("route=%+v", got)
	}
}

func TestCorrelateALSConsumerRouteBackgroundNeighborhood(t *testing.T) {
	got, err := CorrelateALSConsumerRoute(
		ALSQueryOrigin{
			OriginID:            55,
			Kind:                ALSQueryOriginCoordinateNeighborhood,
			HasCoordinateCenter: true,
		},
		ALSResponseFamilySummary{
			OriginID:    55,
			RecordCount: 400,
			FamilyFlag:  ObservedALSResponseFamilyBackground,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Consumer != "background-neighborhood" {
		t.Fatalf("route=%+v", got)
	}
}

func TestCorrelateALSConsumerRouteRejectsMismatchedOrigin(t *testing.T) {
	_, err := CorrelateALSConsumerRoute(
		ALSQueryOrigin{OriginID: 53, Kind: ALSQueryOriginLiveWifi, Reason: ObservedLookupReasonUnknownRatio},
		ALSResponseFamilySummary{OriginID: 54, RecordCount: 114, FamilyFlag: ObservedALSResponseFamilyLive},
	)
	if err == nil {
		t.Fatal("expected origin mismatch")
	}
}

func TestCorrelateALSConsumerRouteRejectsFamilyMismatch(t *testing.T) {
	_, err := CorrelateALSConsumerRoute(
		ALSQueryOrigin{OriginID: 53, Kind: ALSQueryOriginLiveWifi, Reason: ObservedLookupReasonUnknownRatio},
		ALSResponseFamilySummary{OriginID: 53, RecordCount: 400, FamilyFlag: ObservedALSResponseFamilyBackground},
	)
	if err == nil {
		t.Fatal("expected family mismatch")
	}
}
