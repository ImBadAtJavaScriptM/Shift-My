package wloc

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"io"
	"math"
	"testing"
)

func TestPatchResponseCoordinatesOnlyFindsPrefixedFrame(t *testing.T) {
	base, err := hex.DecodeString(capturedNotFoundResponseHex)
	if err != nil {
		t.Fatal(err)
	}
	prefix := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	body := append(append([]byte(nil), prefix...), base...)

	patched, stats, err := PatchResponseCoordinatesOnly(body, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Wifi != 1 || stats.Locations != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if !bytes.Equal(patched[:len(prefix)], prefix) {
		t.Fatalf("prefix changed: %x", patched[:len(prefix)])
	}
	_, _, devices, err := ParseResponse(patched[len(prefix):])
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	if devices[0].LatitudeE8 != int64(math.Trunc(34.0094*1e8)) ||
		devices[0].LongitudeE8 != int64(math.Trunc(-118.4973*1e8)) {
		t.Fatalf("location=%+v", devices[0])
	}
}

func TestPatchResponseCoordinatesOnlyHandlesGzip(t *testing.T) {
	base, err := hex.DecodeString(capturedNotFoundResponseHex)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(base); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	patched, stats, err := PatchResponseCoordinatesOnly(compressed.Bytes(), 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Wifi != 1 || stats.Locations != 1 {
		t.Fatalf("stats=%+v", stats)
	}

	zr, err := gzip.NewReader(bytes.NewReader(patched))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, devices, err := ParseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices=%+v", devices)
	}
	if devices[0].LatitudeE8 != int64(math.Trunc(34.0094*1e8)) ||
		devices[0].LongitudeE8 != int64(math.Trunc(-118.4973*1e8)) {
		t.Fatalf("location=%+v", devices[0])
	}
}

func TestBuildRichResponseFixtureThenPatch(t *testing.T) {
	req := Request{
		Version:    1,
		FunctionID: 1,
		BSSIDs: []string{
			"98:ed:7e:08:59:88",
			"a0:04:60:94:5e:64",
			"3c:5c:f1:6d:d7:84",
		},
	}
	fixture, err := BuildRichResponseFixture(req, DefaultRichFixtureWifiRecords)
	if err != nil {
		t.Fatal(err)
	}

	_, _, before, err := ParseResponse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != DefaultRichFixtureWifiRecords {
		t.Fatalf("fixture records=%d want=%d", len(before), DefaultRichFixtureWifiRecords)
	}

	patched, stats, err := PatchResponseCoordinatesOnly(fixture, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	wantPatched := DefaultRichFixtureWifiRecords - DefaultRichFixtureWifiRecords/8
	if stats.Wifi != wantPatched || stats.Locations != wantPatched {
		t.Fatalf("stats=%+v want patched=%d", stats, wantPatched)
	}
	beforeMasked, err := maskLocationCoordinatesForTest(fixture[10:])
	if err != nil {
		t.Fatal(err)
	}
	afterMasked, err := maskLocationCoordinatesForTest(patched[10:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeMasked, afterMasked) {
		t.Fatal("non-coordinate rich-response bytes changed")
	}

	_, _, after, err := ParseResponse(patched)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != DefaultRichFixtureWifiRecords {
		t.Fatalf("patched records=%d want=%d", len(after), DefaultRichFixtureWifiRecords)
	}
	if after[0].BSSID != req.BSSIDs[0] {
		t.Fatalf("first BSSID=%q want=%q", after[0].BSSID, req.BSSIDs[0])
	}
	for i := 0; i < wantPatched; i++ {
		if after[i].LatitudeE8 != int64(math.Trunc(34.0094*1e8)) ||
			after[i].LongitudeE8 != int64(math.Trunc(-118.4973*1e8)) {
			t.Fatalf("record %d location=%+v", i, after[i])
		}
	}
	for i := wantPatched; i < len(after); i++ {
		if after[i].LatitudeE8 != 0 || after[i].LongitudeE8 != 0 {
			t.Fatalf("missing-location record %d was synthesized: %+v", i, after[i])
		}
	}
}

func TestPatchResponseCoordinatesOnlyHandlesRawProtobufPayload(t *testing.T) {
	req := Request{
		Version:    1,
		FunctionID: 1,
		BSSIDs:     []string{"98:ed:7e:08:59:88"},
	}
	fixture, err := BuildRichResponseFixture(req, 16)
	if err != nil {
		t.Fatal(err)
	}
	rawPayload := fixture[10:]

	patched, stats, err := PatchResponseCoordinatesOnly(rawPayload, 34.0094, -118.4973)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Wifi == 0 || stats.Locations == 0 {
		t.Fatalf("stats=%+v", stats)
	}
	beforeMasked, err := maskLocationCoordinatesForTest(rawPayload)
	if err != nil {
		t.Fatal(err)
	}
	afterMasked, err := maskLocationCoordinatesForTest(patched)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeMasked, afterMasked) {
		t.Fatal("raw protobuf fallback changed non-coordinate bytes")
	}
}
