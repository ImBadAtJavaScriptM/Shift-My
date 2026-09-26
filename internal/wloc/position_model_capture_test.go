package wloc

import (
	"math"
	"testing"
)

func TestEstimateWifiPositionCapsAtStrongestEighteen(t *testing.T) {
	scan := make([]ScanObservation, 0, 20)
	response := make([]DeviceLocation, 0, 20)

	for i := 0; i < 20; i++ {
		bssid := syntheticTestBSSID(i)
		rssi := -30 - i
		latitude := int64(3400000000)
		longitude := int64(-11800000000)

		// Make the two weakest APs extreme spatial outliers. If the estimator
		// fails to cap at the strongest 18, the result will move noticeably.
		if i >= 18 {
			latitude = 3500000000
			longitude = -11700000000
		}

		scan = append(scan, ScanObservation{BSSID: bssid, RSSI: rssi})
		response = append(response, DeviceLocation{
			BSSID:              bssid,
			LatitudeE8:         latitude,
			LongitudeE8:        longitude,
			HorizontalAccuracy: 20,
		})
	}

	got, err := EstimateWifiPosition(scan, response, DefaultWifiPositionMaxAPs)
	if err != nil {
		t.Fatal(err)
	}
	if got.MatchedAPs != 20 || got.UsedAPs != 18 {
		t.Fatalf("matched=%d used=%d", got.MatchedAPs, got.UsedAPs)
	}
	if math.Abs(got.Latitude-34) > 1e-10 || math.Abs(got.Longitude-(-118)) > 1e-10 {
		t.Fatalf("estimate=(%.8f, %.8f)", got.Latitude, got.Longitude)
	}
	if got.WeakestUsedRSSI != -47 {
		t.Fatalf("weakest used RSSI=%d want=-47", got.WeakestUsedRSSI)
	}
}

func syntheticTestBSSID(i int) string {
	const hex = "0123456789abcdef"
	return string([]byte{
		'0', '2', ':',
		'0', '0', ':',
		'0', '0', ':',
		'0', '0', ':',
		hex[(i>>4)&0xf], hex[i&0xf], ':',
		'0', '1',
	})
}
