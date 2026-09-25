package wloc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// DefaultRichFixtureWifiRecords mirrors the scale observed in the controlled
// iPhone capture: a small scan can receive a much larger neighborhood response.
const DefaultRichFixtureWifiRecords = 114

// BuildRichResponseFixture creates a deterministic, lab-only WLOC response
// containing the request BSSIDs plus synthetic locally-administered neighbors.
// It is designed to exercise response patching at realistic scale without
// contacting any external location service.
func BuildRichResponseFixture(req Request, totalWifi int) ([]byte, error) {
	if len(req.BSSIDs) == 0 {
		return nil, errors.New("at least one BSSID is required")
	}
	if totalWifi <= 0 {
		totalWifi = DefaultRichFixtureWifiRecords
	}
	if totalWifi < len(req.BSSIDs) {
		totalWifi = len(req.BSSIDs)
	}
	if totalWifi > 4096 {
		return nil, errors.New("rich fixture wifi record count is too large")
	}

	bssids := make([]string, 0, totalWifi)
	seen := make(map[string]struct{}, totalWifi)
	for _, bssid := range req.BSSIDs {
		if !looksLikeBSSID(bssid) {
			return nil, fmt.Errorf("invalid BSSID %q", bssid)
		}
		key := strings.ToLower(bssid)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		bssids = append(bssids, bssid)
	}

	for i := 0; len(bssids) < totalWifi; i++ {
		bssid := fmt.Sprintf("02:53:4d:%02x:%02x:%02x", byte(i>>16), byte(i>>8), byte(i))
		key := strings.ToLower(bssid)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		bssids = append(bssids, bssid)
	}

	// Keep roughly the same found/missing mix as the controlled capture:
	// 114 total -> 100 coordinate-bearing records + 14 without Location.
	missingLocations := totalWifi / 8
	withLocations := totalWifi - missingLocations

	payload := make([]byte, 0, totalWifi*64)
	for i, bssid := range bssids {
		wifi := appendBytesField(nil, 1, []byte(bssid))
		if i < withLocations {
			// Neutral synthetic source coordinates. The patcher replaces only
			// fields 1/2; all other metadata remains intact.
			location := appendVarintField(nil, 1, int64(123456789+i*137))
			location = appendVarintField(location, 2, int64(234567890+i*211))
			location = appendVarintField(location, 3, int64(20+i%54))
			location = appendVarintField(location, 4, 3)
			location = appendVarintField(location, 5, int64(100+i%20))
			location = appendVarintField(location, 6, 1000)
			location = appendVarintField(location, 9, int64(811913740+i))
			location = appendVarintField(location, 11, 63)
			location = appendVarintField(location, 12, int64(100+i%300))
			location = appendVarintField(location, 29, int64(i+1))
			wifi = appendBytesField(wifi, 2, location)
		} else {
			// Preserve a realistic no-location entry for patcher coverage.
			wifi = appendVarintField(wifi, 7, int64(i))
		}
		wifi = appendVarintField(wifi, 15, int64(1000+i))
		payload = appendBytesField(payload, 2, wifi)
	}

	// Include representative top-level metadata/opaque fields so patch tests
	// verify they survive byte-for-byte.
	payload = appendVarintField(payload, 3, 0)
	payload = appendVarintField(payload, 4, int64(totalWifi))
	payload = appendVarintField(payload, 31, 1)
	payload = appendVarintField(payload, 32, 2)

	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], req.Version)
	binary.BigEndian.PutUint32(frame[2:6], req.FunctionID)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)
	return frame, nil
}
