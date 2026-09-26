package wloc

import (
	"math"
	"testing"
)

// These regression fixtures preserve relative AP geometry, RSSI, and horizontal
// accuracy from two independent successful captures while translating the
// coordinates and replacing all BSSIDs with deterministic synthetic values.
func TestEstimateWifiPositionAnonymizedCaptureNoCap(t *testing.T) {
	scan := make([]ScanObservation, 0, 18)
	response := make([]DeviceLocation, 0, 18)
	fixtures := []struct {
		lat, lon, acc float64
		rssi          int
	}{
		{12.34567800, -67.89012300, 66.0, -82},
		{12.34569326, -67.89014589, 69.0, -82},
		{12.34544912, -67.89035188, 37.0, -86},
		{12.34565511, -67.89040529, 21.0, -41},
		{12.34564748, -67.89039766, 22.0, -41},
		{12.34561697, -67.89031374, 29.0, -50},
		{12.34562459, -67.89030611, 28.0, -50},
		{12.34572378, -67.89010011, 35.0, -91},
		{12.34571996, -67.89010774, 35.0, -91},
		{12.34567800, -67.89037477, 24.0, -48},
		{12.34569326, -67.89039766, 23.0, -51},
		{12.34568182, -67.89040529, 23.0, -57},
		{12.34569707, -67.89039003, 24.0, -51},
		{12.34568944, -67.89053499, 20.0, -74},
		{12.34568944, -67.89053499, 20.0, -74},
		{12.34557500, -67.88990175, 46.0, -91},
		{12.34575048, -67.89017641, 50.0, -91},
		{12.34572378, -67.89033662, 27.0, -62},
	}
	for i, f := range fixtures {
		bssid := syntheticTestBSSID(i)
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: f.rssi})
		response = append(response, DeviceLocation{BSSID: bssid, LatitudeE8: int64(math.Round(f.lat * 1e8)), LongitudeE8: int64(math.Round(f.lon * 1e8)), HorizontalAccuracy: int64(math.Round(f.acc))})
	}
	got, err := EstimateWifiPosition(scan, response, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	wantLat, wantLon := 12.34566442, -67.89032756
	errMeters := approximateDistanceMeters(wantLat, wantLon, got.Latitude, got.Longitude)
	if errMeters > 1.0 {
		t.Fatalf("error=%.3fm estimate=(%.8f, %.8f) want=(%.8f, %.8f)", errMeters, got.Latitude, got.Longitude, wantLat, wantLon)
	}
	if got.UsedAPs != 18 {
		t.Fatalf("used=%d", got.UsedAPs)
	}
}

func TestEstimateWifiPositionAnonymizedCaptureTop18(t *testing.T) {
	scan := make([]ScanObservation, 0, 20)
	response := make([]DeviceLocation, 0, 20)
	fixtures := []struct {
		lat, lon, acc float64
		rssi          int
	}{
		{-23.45678900, 45.67890100, 22.0, -84},
		{-23.45651053, 45.67900782, 37.0, -82},
		{-23.45621680, 45.67929010, 67.0, -79},
		{-23.45630454, 45.67895441, 21.0, -33},
		{-23.45631217, 45.67896204, 22.0, -33},
		{-23.45634268, 45.67904596, 29.0, -61},
		{-23.45633506, 45.67905359, 28.0, -61},
		{-23.45649909, 45.67898493, 29.0, -89},
		{-23.45628165, 45.67898493, 24.0, -60},
		{-23.45608329, 45.67954187, 38.0, -93},
		{-23.45626258, 45.67885523, 20.0, -55},
		{-23.45625876, 45.67885523, 20.0, -54},
		{-23.45626639, 45.67896204, 23.0, -58},
		{-23.45627783, 45.67895441, 23.0, -57},
		{-23.45626258, 45.67896967, 24.0, -58},
		{-23.45627021, 45.67882471, 20.0, -69},
		{-23.45627021, 45.67882471, 20.0, -69},
		{-23.45638465, 45.67945795, 46.0, -92},
		{-23.45623587, 45.67902308, 27.0, -63},
		{-23.45607566, 45.67906122, 36.0, -93},
	}
	for i, f := range fixtures {
		bssid := syntheticTestBSSID(i)
		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: f.rssi})
		response = append(response, DeviceLocation{BSSID: bssid, LatitudeE8: int64(math.Round(f.lat * 1e8)), LongitudeE8: int64(math.Round(f.lon * 1e8)), HorizontalAccuracy: int64(math.Round(f.acc))})
	}
	got, err := EstimateWifiPosition(scan, response, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	wantLat, wantLon := -23.45632882, 45.67897750
	errMeters := approximateDistanceMeters(wantLat, wantLon, got.Latitude, got.Longitude)
	if errMeters > 1.0 {
		t.Fatalf("error=%.3fm estimate=(%.8f, %.8f) want=(%.8f, %.8f)", errMeters, got.Latitude, got.Longitude, wantLat, wantLon)
	}
	if got.UsedAPs != 18 {
		t.Fatalf("used=%d", got.UsedAPs)
	}
}
