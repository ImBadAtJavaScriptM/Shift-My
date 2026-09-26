package wloc

import "testing"

func TestEstimateWifiPositionCapturedTwentyFiveToEighteen(t *testing.T) {
	scan := []ScanObservation{
		{BSSID: "a0:8a:06:b1:cd:49", RSSI: -91},
		{BSSID: "16:0c:6b:fa:00:96", RSSI: -82},
		{BSSID: "3c:5c:f1:b5:65:04", RSSI: -50},
		{BSSID: "84:eb:3f:3f:ab:7b", RSSI: -48},
		{BSSID: "3c:5c:f1:6d:d7:86", RSSI: -41},
		{BSSID: "3c:5c:f1:6d:d7:88", RSSI: -41},
		{BSSID: "b6:2f:03:90:48:9a", RSSI: -62},
		{BSSID: "98:ed:7e:08:59:87", RSSI: -51},
		{BSSID: "3c:5c:f1:b5:65:06", RSSI: -50},
		{BSSID: "d0:fc:d0:f7:20:74", RSSI: -60},
		{BSSID: "98:ed:7e:0d:87:a4", RSSI: -60},
		{BSSID: "50:6f:9a:01:00:00", RSSI: -60},
		{BSSID: "74:b6:b6:c5:2d:a5", RSSI: -91},
		{BSSID: "98:ed:7e:08:59:88", RSSI: -57},
		{BSSID: "98:ed:7e:0d:87:a7", RSSI: -74},
		{BSSID: "74:b6:b6:c5:2d:ab", RSSI: -91},
		{BSSID: "98:ed:7e:0d:87:aa", RSSI: -74},
		{BSSID: "98:ed:7e:08:59:84", RSSI: -60},
		{BSSID: "74:b6:b6:c5:2d:a3", RSSI: -60},
		{BSSID: "98:ed:7e:08:59:8a", RSSI: -51},
		{BSSID: "3c:5c:f1:6d:d7:84", RSSI: -60},
		{BSSID: "10:0c:6b:fa:00:96", RSSI: -82},
		{BSSID: "3c:5c:f1:b5:65:02", RSSI: -60},
		{BSSID: "22:48:6c:5c:c6:f5", RSSI: -86},
		{BSSID: "a0:04:60:94:5e:64", RSSI: -91},
	}
	response := []DeviceLocation{
		{BSSID: "10:0c:6b:fa:00:96", LatitudeE8: 3419876861, LongitudeE8: -11831657409, HorizontalAccuracy: 66},
		{BSSID: "16:0c:6b:fa:00:96", LatitudeE8: 3419878387, LongitudeE8: -11831659698, HorizontalAccuracy: 69},
		{BSSID: "22:48:6c:5c:c6:f5", LatitudeE8: 3419853973, LongitudeE8: -11831680297, HorizontalAccuracy: 37},
		{BSSID: "3c:5c:f1:6d:d7:86", LatitudeE8: 3419874572, LongitudeE8: -11831685638, HorizontalAccuracy: 21},
		{BSSID: "3c:5c:f1:6d:d7:88", LatitudeE8: 3419873809, LongitudeE8: -11831684875, HorizontalAccuracy: 22},
		{BSSID: "3c:5c:f1:b5:65:04", LatitudeE8: 3419870758, LongitudeE8: -11831676483, HorizontalAccuracy: 29},
		{BSSID: "3c:5c:f1:b5:65:06", LatitudeE8: 3419871520, LongitudeE8: -11831675720, HorizontalAccuracy: 28},
		{BSSID: "74:b6:b6:c5:2d:a5", LatitudeE8: 3419881439, LongitudeE8: -11831655120, HorizontalAccuracy: 35},
		{BSSID: "74:b6:b6:c5:2d:ab", LatitudeE8: 3419881057, LongitudeE8: -11831655883, HorizontalAccuracy: 35},
		{BSSID: "84:eb:3f:3f:ab:7b", LatitudeE8: 3419876861, LongitudeE8: -11831682586, HorizontalAccuracy: 24},
		{BSSID: "98:ed:7e:08:59:87", LatitudeE8: 3419878387, LongitudeE8: -11831684875, HorizontalAccuracy: 23},
		{BSSID: "98:ed:7e:08:59:88", LatitudeE8: 3419877243, LongitudeE8: -11831685638, HorizontalAccuracy: 23},
		{BSSID: "98:ed:7e:08:59:8a", LatitudeE8: 3419878768, LongitudeE8: -11831684112, HorizontalAccuracy: 24},
		{BSSID: "98:ed:7e:0d:87:a7", LatitudeE8: 3419878005, LongitudeE8: -11831698608, HorizontalAccuracy: 20},
		{BSSID: "98:ed:7e:0d:87:aa", LatitudeE8: 3419878005, LongitudeE8: -11831698608, HorizontalAccuracy: 20},
		{BSSID: "a0:04:60:94:5e:64", LatitudeE8: 3419866561, LongitudeE8: -11831635284, HorizontalAccuracy: 46},
		{BSSID: "a0:8a:06:b1:cd:49", LatitudeE8: 3419884109, LongitudeE8: -11831662750, HorizontalAccuracy: 50},
		{BSSID: "b6:2f:03:90:48:9a", LatitudeE8: 3419881439, LongitudeE8: -11831678771, HorizontalAccuracy: 27},
	}

	got, err := EstimateWifiPosition(scan, response, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if got.MatchedAPs != 18 || got.UsedAPs != 16 {
		t.Fatalf("matched=%d used=%d", got.MatchedAPs, got.UsedAPs)
	}
	const wantLat = 34.19875503
	const wantLon = -118.31677865
	errMeters := approximateDistanceMeters(got.Latitude, got.Longitude, wantLat, wantLon)
	if errMeters > 0.3 {
		t.Fatalf("estimate=(%.8f,%.8f) real=(%.8f,%.8f) error=%.3fm", got.Latitude, got.Longitude, wantLat, wantLon, errMeters)
	}
	t.Logf("estimate=(%.8f,%.8f) real=(%.8f,%.8f) error=%.3fm", got.Latitude, got.Longitude, wantLat, wantLon, errMeters)
}
