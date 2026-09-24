package wloc

import (
	"bytes"
	"encoding/hex"
	"math"
	"slices"
	"testing"
)

const capturedLegacyRequestHex = "00010005656e5f55530013636f6d2e6170706c652e6c6f636174696f6e64000a382e312e313242343131000000010000001912130a1133343a44423a46443a34333a45333a413118002001"
const compactResponseStyleFixtureHex = "000100000001000000190a1134323a37353a63333a66393a61313a3339f80101800202"
const modernStructuredRequestHex = "0001000a656e2d3030315f3030310013636f6d2e6170706c652e6c6f636174696f6e64000d31382e362e322e323247313030000000010000001912130a1134323a44423a46443a34333a45333a413118002001"
const realisticMultiBSSIDRequestHex = "000100000001000000be12350a1161613a64353a39643a35383a65393a396412200880d4dae60e10d8c7c3f6d8ffffffff011827200328920430e807583f60d30312350a1133633a37633a33663a65343a37323a343812200880d4dae60e10d8c7c3f6d8ffffffff011827200328920430e807583f60d30312350a1138613a64353a39643a35383a65393a396412200880d4dae60e10d8c7c3f6d8ffffffff011827200328920430e807583f60d3030a1137613a64353a39643a35383a65393a3964f80101800202"
const modernFunction2BSSIDOnlyRequestHex = "0001000a656e2d3030315f3030310013636f6d2e6170706c652e6c6f636174696f6e64000a32362e322e3233433535000000020000004512130a1130323a30303a30303a30303a30303a303112130a1130323a30303a30303a30303a30303a303212130a1130323a30303a30303a30303a30303a3033f80101800202"
const capturedNotFoundResponseHex = "0001000000010000004312410a1133343a64623a66643a34333a65333a6131122c088098f7f8bcffffffff01108098f7f8bcffffffff0118ffffffffffffffffff0128ffffffffffffffffff01"

func TestParseCapturedLegacyRequest(t *testing.T) {
	data, err := hex.DecodeString(capturedLegacyRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != 1 || req.FunctionID != 1 || req.Envelope != "structured-arpc" {
		t.Fatalf("version=%d function=%d envelope=%q", req.Version, req.FunctionID, req.Envelope)
	}
	if req.Locale != "en_US" || req.AppIdentifier != "com.apple.locationd" || req.OSVersion != "8.1.12B411" {
		t.Fatalf("envelope=%+v", req)
	}
	if len(req.BSSIDs) != 1 || req.BSSIDs[0] != "34:DB:FD:43:E3:A1" {
		t.Fatalf("bssids=%v", req.BSSIDs)
	}
}

func TestParseCompactResponseStyleFixtureAsLabInput(t *testing.T) {
	data, err := hex.DecodeString(compactResponseStyleFixtureHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != 1 || req.FunctionID != 1 || req.Envelope != "compact-response-style" {
		t.Fatalf("version=%d function=%d envelope=%q", req.Version, req.FunctionID, req.Envelope)
	}
	if len(req.BSSIDs) != 1 || req.BSSIDs[0] != "42:75:c3:f9:a1:39" {
		t.Fatalf("bssids=%v", req.BSSIDs)
	}
}

func TestParseModernStructuredARPCRequest(t *testing.T) {
	data, err := hex.DecodeString(modernStructuredRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != 1 || req.FunctionID != 1 || req.Envelope != "structured-arpc" {
		t.Fatalf("version=%d function=%d envelope=%q", req.Version, req.FunctionID, req.Envelope)
	}
	if req.Locale != "en-001_001" || req.AppIdentifier != "com.apple.locationd" || req.OSVersion != "18.6.2.22G100" {
		t.Fatalf("envelope=%+v", req)
	}
	if len(req.BSSIDs) != 1 || req.BSSIDs[0] != "42:DB:FD:43:E3:A1" {
		t.Fatalf("bssids=%v", req.BSSIDs)
	}
}

func TestParseCapturedNotFoundResponse(t *testing.T) {
	data, err := hex.DecodeString(capturedNotFoundResponseHex)
	if err != nil {
		t.Fatal(err)
	}
	version, functionID, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || functionID != 1 || len(devices) != 1 {
		t.Fatalf("version=%d function=%d devices=%+v", version, functionID, devices)
	}
	if devices[0].BSSID != "34:db:fd:43:e3:a1" {
		t.Fatalf("bssid=%q", devices[0].BSSID)
	}
	if devices[0].LatitudeE8 != -18000000000 || devices[0].LongitudeE8 != -18000000000 {
		t.Fatalf("location=%+v", devices[0])
	}
}

func TestBuildResponseUsesSelectedTargetForEveryRequestedBSSID(t *testing.T) {
	req := Request{Version: 1, FunctionID: 1, BSSIDs: []string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02"}}
	data, err := BuildResponse(req, 34.146941, -118.144516)
	if err != nil {
		t.Fatal(err)
	}
	version, functionID, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || functionID != 1 || len(devices) != 2 {
		t.Fatalf("version=%d function=%d devices=%+v", version, functionID, devices)
	}
	wantLat := int64(math.Round(34.146941 * 1e8))
	wantLon := int64(math.Round(-118.144516 * 1e8))
	for i, device := range devices {
		if device.BSSID != req.BSSIDs[i] || device.LatitudeE8 != wantLat || device.LongitudeE8 != wantLon {
			t.Fatalf("device[%d]=%+v", i, device)
		}
		if device.HorizontalAccuracy != defaultHorizontalAccuracy ||
			device.UnknownValue4 != defaultUnknownValue4 ||
			device.Altitude != defaultAltitude ||
			device.VerticalAccuracy != defaultVerticalAccuracy ||
			device.MotionActivityType != defaultMotionActivityType ||
			device.MotionActivityConfidence != defaultMotionActivityConfidence {
			t.Fatalf("device[%d] metadata=%+v", i, device)
		}
	}
}

func TestParseRequestRejectsTruncatedPayload(t *testing.T) {
	data, err := hex.DecodeString(capturedLegacyRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(data[:len(data)-1]); err == nil {
		t.Fatal("expected truncated request rejection")
	}

	data, err = hex.DecodeString(compactResponseStyleFixtureHex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(data[:len(data)-1]); err == nil {
		t.Fatal("expected truncated compact request rejection")
	}
}

func TestRichResponseContainsReferenceMetadataFields(t *testing.T) {
	req := Request{Version: 1, FunctionID: 1, BSSIDs: []string{"34:DB:FD:43:E3:A1"}}
	data, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	_, _, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.HorizontalAccuracy != 39 ||
		got.UnknownValue4 != 3 ||
		got.Altitude != 530 ||
		got.VerticalAccuracy != 1000 ||
		got.MotionActivityType != 63 ||
		got.MotionActivityConfidence != 467 {
		t.Fatalf("metadata=%+v", got)
	}
}

func TestBuildResponsePreservesTopLevelAndWifiUnknownFields(t *testing.T) {
	oldLocation := appendVarintField(nil, 1, 123)
	oldLocation = appendVarintField(oldLocation, 2, -456)

	wifi := appendBytesField(nil, 1, []byte("aa:bb:cc:dd:ee:01"))
	wifi = appendBytesField(wifi, 2, oldLocation)
	wifiUnknown := appendVarintField(nil, 7, 9)
	wifi = append(wifi, wifiUnknown...)

	topBundle := appendBytesField(nil, 5, []byte("com.example.fixture"))
	topUnknown31 := appendVarintField(nil, 31, 1)
	topUnknown32 := appendVarintField(nil, 32, 2)

	payload := appendBytesField(nil, 2, wifi)
	payload = append(payload, topBundle...)
	payload = append(payload, topUnknown31...)
	payload = append(payload, topUnknown32...)

	req := Request{
		Version:    1,
		FunctionID: 1,
		BSSIDs:     []string{"aa:bb:cc:dd:ee:01"},
		Payload:    payload,
	}
	data, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	responsePayload := data[10:]

	for name, field := range map[string][]byte{
		"top bundle": topBundle,
		"top field 31": topUnknown31,
		"top field 32": topUnknown32,
		"wifi field 7": wifiUnknown,
	} {
		if !bytes.Contains(responsePayload, field) {
			t.Fatalf("%s was not preserved: %x", name, responsePayload)
		}
	}
	if bytes.Contains(responsePayload, oldLocation) {
		t.Fatalf("old location was preserved instead of replaced: %x", responsePayload)
	}

	_, _, devices, err := ParseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.LatitudeE8 != int64(math.Round(34.0094*1e8)) ||
		got.LongitudeE8 != int64(math.Round(-118.4973*1e8)) {
		t.Fatalf("location=%+v", got)
	}
}

func TestParsedRequestRetainsOriginalPayloadForRewrite(t *testing.T) {
	data, err := hex.DecodeString(capturedLegacyRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Payload) == 0 {
		t.Fatal("expected parsed request payload to be retained")
	}
	original := append([]byte(nil), req.Payload...)

	response, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(response[10:], original) {
		t.Fatal("expected wifi location block to be rewritten")
	}
	if !bytes.Contains(response[10:], []byte("34:DB:FD:43:E3:A1")) {
		t.Fatalf("BSSID was not preserved: %x", response[10:])
	}
}

func TestClearResultMetadataVariant(t *testing.T) {
	oldLocation := appendVarintField(nil, 1, 111)
	oldLocation = appendVarintField(oldLocation, 2, 222)

	wifi := appendBytesField(nil, 1, []byte("aa:bb:cc:dd:ee:01"))
	wifi = appendBytesField(wifi, 2, oldLocation)

	numCell := appendVarintField(nil, 3, 7)
	numWifi := appendVarintField(nil, 4, -1)
	appBundle := appendBytesField(nil, 5, []byte("com.example.fixture"))
	deviceType := appendBytesField(nil, 33, appendBytesField(nil, 1, []byte("N104AP")))
	unknown31 := appendVarintField(nil, 31, 1)

	payload := appendBytesField(nil, 2, wifi)
	payload = append(payload, numCell...)
	payload = append(payload, numWifi...)
	payload = append(payload, appBundle...)
	payload = append(payload, deviceType...)
	payload = append(payload, unknown31...)

	req := Request{
		Version:    1,
		FunctionID: 1,
		BSSIDs:     []string{"aa:bb:cc:dd:ee:01"},
		Payload:    payload,
	}

	preserve, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := BuildResponseClearingResultMetadata(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}

	for name, field := range map[string][]byte{
		"num cell results": numCell,
		"num wifi results": numWifi,
		"device type": deviceType,
	} {
		if !bytes.Contains(preserve[10:], field) {
			t.Fatalf("preserve variant lost %s: %x", name, preserve)
		}
		if bytes.Contains(cleared[10:], field) {
			t.Fatalf("clear variant retained %s: %x", name, cleared)
		}
	}
	for name, field := range map[string][]byte{
		"app bundle": appBundle,
		"unknown field 31": unknown31,
	} {
		if !bytes.Contains(preserve[10:], field) || !bytes.Contains(cleared[10:], field) {
			t.Fatalf("%s was not preserved in both variants", name)
		}
	}

	_, _, preserveDevices, err := ParseResponse(preserve)
	if err != nil {
		t.Fatal(err)
	}
	_, _, clearDevices, err := ParseResponse(cleared)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserveDevices) != 1 || len(clearDevices) != 1 {
		t.Fatalf("preserve=%+v clear=%+v", preserveDevices, clearDevices)
	}
	if preserveDevices[0].LatitudeE8 != clearDevices[0].LatitudeE8 ||
		preserveDevices[0].LongitudeE8 != clearDevices[0].LongitudeE8 {
		t.Fatalf("location changed between variants: preserve=%+v clear=%+v", preserveDevices[0], clearDevices[0])
	}

	t.Logf("preserve_bytes=%d preserve_hex=%x", len(preserve), preserve)
	t.Logf("cleared_bytes=%d cleared_hex=%x", len(cleared), cleared)
}

func TestRealisticMultiBSSIDFixtureModesAreIdenticalWhenResultMetadataAbsent(t *testing.T) {
	data, err := hex.DecodeString(realisticMultiBSSIDRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	preserve, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := BuildResponseClearingResultMetadata(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preserve, cleared) {
		t.Fatalf("expected identical responses when fields 3,4,33 are absent\npreserve=%x\ncleared=%x", preserve, cleared)
	}
	if len(preserve) != 200 {
		t.Fatalf("response length=%d want=200", len(preserve))
	}
	_, _, devices, err := ParseResponse(preserve)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 3 {
		t.Fatalf("wifi devices=%d want=3: %+v", len(devices), devices)
	}
	wantLat := int64(math.Round(34.0094 * 1e8))
	wantLon := int64(math.Round(-118.4973 * 1e8))
	for i, device := range devices {
		if device.LatitudeE8 != wantLat || device.LongitudeE8 != wantLon {
			t.Fatalf("device[%d]=%+v", i, device)
		}
	}
	for name, field := range map[string][]byte{
		"top-level field 1 BSSID-like value": []byte("7a:d5:9d:58:e9:9d"),
		"field 31": appendVarintField(nil, 31, 1),
		"field 32": appendVarintField(nil, 32, 2),
	} {
		if !bytes.Contains(preserve[10:], field) {
			t.Fatalf("%s was not preserved", name)
		}
	}
}

func TestRealisticMultiBSSIDRewriteChangesOnlyCoordinates(t *testing.T) {
	data, err := hex.DecodeString(realisticMultiBSSIDRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	response, err := BuildResponse(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if len(response) != len(data) {
		t.Fatalf("response length=%d request length=%d", len(response), len(data))
	}

	originalMasked, err := maskLocationCoordinatesForTest(data[10:])
	if err != nil {
		t.Fatal(err)
	}
	responseMasked, err := maskLocationCoordinatesForTest(response[10:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(originalMasked, responseMasked) {
		t.Fatalf("non-coordinate bytes changed\noriginal=%x\nresponse=%x", originalMasked, responseMasked)
	}

	_, _, devices, err := ParseResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	for i, device := range devices {
		if device.HorizontalAccuracy != 39 ||
			device.UnknownValue4 != 3 ||
			device.Altitude != 530 ||
			device.VerticalAccuracy != 1000 ||
			device.MotionActivityType != 63 ||
			device.MotionActivityConfidence != 467 {
			t.Fatalf("device[%d] metadata=%+v", i, device)
		}
	}
}

func TestRealisticCapturedLocationFieldSet(t *testing.T) {
	data, err := hex.DecodeString(realisticMultiBSSIDRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	payload := data[10:]
	wantFields := []int{1, 2, 3, 4, 5, 6, 11, 12}
	seenWifi := 0
	for pos := 0; pos < len(payload); {
		field, wire, value, next, err := nextField(payload, pos)
		if err != nil {
			t.Fatal(err)
		}
		pos = next
		if field != 2 || wire != 2 {
			continue
		}
		seenWifi++
		var location []byte
		for wpos := 0; wpos < len(value); {
			wf, ww, wv, wnext, err := nextField(value, wpos)
			if err != nil {
				t.Fatal(err)
			}
			wpos = wnext
			if wf == 2 && ww == 2 {
				location = wv
			}
		}
		if location == nil {
			t.Fatalf("wifi device %d missing location", seenWifi)
		}
		var got []int
		for lpos := 0; lpos < len(location); {
			lf, _, _, lnext, err := nextField(location, lpos)
			if err != nil {
				t.Fatal(err)
			}
			lpos = lnext
			got = append(got, lf)
		}
		if !slices.Equal(got, wantFields) {
			t.Fatalf("wifi device %d fields=%v want=%v", seenWifi, got, wantFields)
		}
	}
	if seenWifi != 3 {
		t.Fatalf("wifi devices=%d want=3", seenWifi)
	}
}

func maskLocationCoordinatesForTest(payload []byte) ([]byte, error) {
	out := make([]byte, 0, len(payload))
	for pos := 0; pos < len(payload); {
		start := pos
		field, wire, value, next, err := nextField(payload, pos)
		if err != nil {
			return nil, err
		}
		pos = next
		if field != 2 || wire != 2 {
			out = append(out, payload[start:next]...)
			continue
		}

		wifi := make([]byte, 0, len(value))
		for wpos := 0; wpos < len(value); {
			wstart := wpos
			wf, ww, wv, wnext, err := nextField(value, wpos)
			if err != nil {
				return nil, err
			}
			wpos = wnext
			if wf != 2 || ww != 2 {
				wifi = append(wifi, value[wstart:wnext]...)
				continue
			}

			location := make([]byte, 0, len(wv))
			for lpos := 0; lpos < len(wv); {
				lstart := lpos
				lf, lw, _, lnext, err := nextField(wv, lpos)
				if err != nil {
					return nil, err
				}
				lpos = lnext
				if (lf == 1 || lf == 2) && lw == 0 {
					location = appendVarintField(location, lf, 0)
				} else {
					location = append(location, wv[lstart:lnext]...)
				}
			}
			wifi = appendBytesField(wifi, 2, location)
		}
		out = appendBytesField(out, 2, wifi)
	}
	return out, nil
}

func TestRealisticMultiBSSIDSeparatesOpaqueTopLevelReference(t *testing.T) {
	data, err := hex.DecodeString(realisticMultiBSSIDRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.BSSIDs) != 3 {
		t.Fatalf("wifi device bssids=%v", req.BSSIDs)
	}
	if len(req.TopLevelBSSIDs) != 1 || req.TopLevelBSSIDs[0] != "7a:d5:9d:58:e9:9d" {
		t.Fatalf("top-level bssid-like values=%v", req.TopLevelBSSIDs)
	}
	for _, bssid := range req.BSSIDs {
		if bssid == req.TopLevelBSSIDs[0] {
			t.Fatalf("top-level opaque value was conflated with WifiDevice BSSID: %v", req)
		}
	}
}

func TestCoordinatesOnlyPreservesMetadataAndOpaqueTopLevelFields(t *testing.T) {
	location := appendVarintField(nil, 1, 111)
	location = appendVarintField(location, 2, 222)
	location = appendVarintField(location, 3, 88)
	location = appendVarintField(location, 4, 9)
	location = appendVarintField(location, 5, 777)
	location = appendVarintField(location, 6, 555)
	location = appendVarintField(location, 9, 123456789)
	location = appendVarintField(location, 11, 17)
	location = appendVarintField(location, 12, 23)
	location = appendVarintField(location, 29, 44)

	wifi := appendBytesField(nil, 1, []byte("aa:bb:cc:dd:ee:01"))
	wifi = appendBytesField(wifi, 2, location)
	topLevelReference := appendBytesField(nil, 1, []byte("7a:d5:9d:58:e9:9d"))
	field31 := appendVarintField(nil, 31, 1)
	field32 := appendVarintField(nil, 32, 2)

	payload := append([]byte(nil), topLevelReference...)
	payload = appendBytesField(payload, 2, wifi)
	payload = append(payload, field31...)
	payload = append(payload, field32...)

	req := Request{
		Version:        1,
		FunctionID:     1,
		BSSIDs:         []string{"aa:bb:cc:dd:ee:01"},
		TopLevelBSSIDs: []string{"7a:d5:9d:58:e9:9d"},
		Payload:        payload,
	}

	response, err := BuildResponseCoordinatesOnly(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	responsePayload := response[10:]
	for name, raw := range map[string][]byte{
		"top-level field 1": topLevelReference,
		"top-level field 31": field31,
		"top-level field 32": field32,
		"horizontal accuracy 88": appendVarintField(nil, 3, 88),
		"field 4 value 9": appendVarintField(nil, 4, 9),
		"altitude 777": appendVarintField(nil, 5, 777),
		"vertical accuracy 555": appendVarintField(nil, 6, 555),
		"timestamp 123456789": appendVarintField(nil, 9, 123456789),
		"motion type 17": appendVarintField(nil, 11, 17),
		"motion confidence 23": appendVarintField(nil, 12, 23),
		"location field 29 value 44": appendVarintField(nil, 29, 44),
	} {
		if !bytes.Contains(responsePayload, raw) {
			t.Fatalf("%s was not preserved: %x", name, responsePayload)
		}
	}

	_, _, devices, err := ParseResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.LatitudeE8 != int64(math.Trunc(34.0094*1e8)) ||
		got.LongitudeE8 != int64(math.Trunc(-118.4973*1e8)) {
		t.Fatalf("location=%+v", got)
	}
	if got.HorizontalAccuracy != 88 ||
		got.UnknownValue4 != 9 ||
		got.Altitude != 777 ||
		got.VerticalAccuracy != 555 ||
		got.MotionActivityType != 17 ||
		got.MotionActivityConfidence != 23 {
		t.Fatalf("metadata was changed: %+v", got)
	}
}

func TestCoordinatesOnlyAddsMinimalLocationWhenMissing(t *testing.T) {
	wifi := appendBytesField(nil, 1, []byte("aa:bb:cc:dd:ee:01"))
	wifiUnknown := appendVarintField(nil, 7, 9)
	wifi = append(wifi, wifiUnknown...)
	payload := appendBytesField(nil, 2, wifi)

	req := Request{
		Version:    1,
		FunctionID: 1,
		BSSIDs:     []string{"aa:bb:cc:dd:ee:01"},
		Payload:    payload,
	}
	response, err := BuildResponseCoordinatesOnly(req, 1.000000004, -2.000000004)
	if err != nil {
		t.Fatal(err)
	}
	_, _, devices, err := ParseResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.LatitudeE8 != 100000000 || got.LongitudeE8 != -200000000 {
		t.Fatalf("coordinates=%+v", got)
	}
	if got.HorizontalAccuracy != 0 || got.Altitude != 0 || got.MotionActivityType != 0 {
		t.Fatalf("minimal location unexpectedly injected metadata: %+v", got)
	}
	if !bytes.Contains(response[10:], wifiUnknown) {
		t.Fatalf("wifi unknown field was not preserved: %x", response[10:])
	}
}

func TestModernFunction2BSSIDOnlyRequestAndCoordsOnlyResponse(t *testing.T) {
	data, err := hex.DecodeString(modernFunction2BSSIDOnlyRequestHex)
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if req.Version != 1 || req.FunctionID != 2 || req.Envelope != "structured-arpc" {
		t.Fatalf("request=%+v", req)
	}
	if req.Locale != "en-001_001" || req.AppIdentifier != "com.apple.locationd" || req.OSVersion != "26.2.23C55" {
		t.Fatalf("envelope=%+v", req)
	}
	wantBSSIDs := []string{
		"02:00:00:00:00:01",
		"02:00:00:00:00:02",
		"02:00:00:00:00:03",
	}
	if !slices.Equal(req.BSSIDs, wantBSSIDs) {
		t.Fatalf("bssids=%v want=%v", req.BSSIDs, wantBSSIDs)
	}

	response, err := BuildResponseCoordinatesOnly(req, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	version, functionID, devices, err := ParseResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || functionID != 2 {
		t.Fatalf("version=%d function=%d", version, functionID)
	}
	if len(devices) != 3 {
		t.Fatalf("devices=%+v", devices)
	}
	wantLat := int64(math.Trunc(34.0094 * 1e8))
	wantLon := int64(math.Trunc(-118.4973 * 1e8))
	for i, device := range devices {
		if device.BSSID != wantBSSIDs[i] || device.LatitudeE8 != wantLat || device.LongitudeE8 != wantLon {
			t.Fatalf("device[%d]=%+v", i, device)
		}
		if device.HorizontalAccuracy != 0 || device.Altitude != 0 || device.MotionActivityType != 0 {
			t.Fatalf("coords-only injected metadata into request-only wifi device: %+v", device)
		}
	}
}

func TestPatchResponseCoordinatesOnlyPatchesCapturedNotFoundSentinel(t *testing.T) {
	data, err := hex.DecodeString(capturedNotFoundResponseHex)
	if err != nil {
		t.Fatal(err)
	}
	patched, stats, err := PatchResponseCoordinatesOnly(data, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Wifi != 1 || stats.Cell != 0 || stats.Locations != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	_, _, devices, err := ParseResponse(patched)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	got := devices[0]
	if got.LatitudeE8 != int64(math.Trunc(34.0094*1e8)) ||
		got.LongitudeE8 != int64(math.Trunc(-118.4973*1e8)) {
		t.Fatalf("location=%+v", got)
	}
	// The captured not-found response carries -1 metadata. A response-side
	// patch must preserve that rather than rebuilding the Location message.
	if got.HorizontalAccuracy != -1 || got.UnknownValue4 != 0 || got.Altitude != 0 {
		t.Fatalf("metadata changed unexpectedly: %+v", got)
	}
}

func TestPatchResponseCoordinatesOnlyDoesNotSynthesizeMissingLocation(t *testing.T) {
	wifi := appendBytesField(nil, 1, []byte("02:00:00:00:00:01"))
	payload := appendBytesField(nil, 2, wifi)
	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], 1)
	binary.BigEndian.PutUint32(frame[2:6], 2)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)

	patched, stats, err := PatchResponseCoordinatesOnly(frame, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Locations != 0 || stats.Wifi != 0 || stats.Cell != 0 {
		t.Fatalf("stats=%+v", stats)
	}
	if !bytes.Equal(patched, frame) {
		t.Fatalf("request-only response was modified\noriginal=%x\npatched=%x", frame, patched)
	}
}

func TestPatchResponseCoordinatesOnlyPatchesCellResponseAndPreservesFields(t *testing.T) {
	location := appendVarintField(nil, 1, 111)
	location = appendVarintField(location, 2, 222)
	location = appendVarintField(location, 3, 88)
	location = appendVarintField(location, 9, 123456789)

	cell := appendVarintField(nil, 1, 460)
	cell = appendVarintField(cell, 2, 11)
	cell = appendBytesField(cell, 5, location)
	cell = appendVarintField(cell, 7, 9)

	payload := appendBytesField(nil, 22, cell)
	payload = appendVarintField(payload, 31, 1)

	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], 1)
	binary.BigEndian.PutUint32(frame[2:6], 2)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)

	patched, stats, err := PatchResponseCoordinatesOnly(frame, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Cell != 1 || stats.Wifi != 0 || stats.Locations != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if !bytes.Contains(patched[10:], appendVarintField(nil, 3, 88)) {
		t.Fatal("horizontal accuracy was not preserved")
	}
	if !bytes.Contains(patched[10:], appendVarintField(nil, 9, 123456789)) {
		t.Fatal("timestamp-like field was not preserved")
	}
	if !bytes.Contains(patched[10:], appendVarintField(nil, 31, 1)) {
		t.Fatal("top-level opaque field was not preserved")
	}
	if bytes.Contains(patched[10:], appendVarintField(nil, 1, 111)) &&
		bytes.Contains(patched[10:], appendVarintField(nil, 2, 222)) {
		t.Fatal("old cell coordinates still present")
	}
}
