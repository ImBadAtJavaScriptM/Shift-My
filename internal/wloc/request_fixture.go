package wloc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const DefaultRealisticRequestWifiRecords = 22

// BuildSyntheticStructuredRequestFixture creates a structured ARPC-style WLOC
// request using deterministic locally-administered BSSIDs. It is intended only
// for controlled lab traffic to project-owned hosts.
func BuildSyntheticStructuredRequestFixture(totalWifi int) ([]byte, error) {
	if totalWifi <= 0 {
		totalWifi = DefaultRealisticRequestWifiRecords
	}
	if totalWifi > 4096 {
		return nil, errors.New("synthetic request wifi record count is too large")
	}

	bssids := make([]string, 0, totalWifi)
	for i := 0; i < totalWifi; i++ {
		bssids = append(bssids, fmt.Sprintf("02:54:4d:%02x:%02x:%02x", byte(i>>16), byte(i>>8), byte(i)))
	}
	return BuildStructuredRequestFixture(bssids)
}

// BuildStructuredRequestFixture emits the same structured envelope accepted by
// ParseRequest: version + three big-endian length-prefixed strings + function
// ID + protobuf payload length + WifiDevice entries.
func BuildStructuredRequestFixture(bssids []string) ([]byte, error) {
	if len(bssids) == 0 {
		return nil, errors.New("at least one BSSID is required")
	}
	for _, bssid := range bssids {
		if !looksLikeBSSID(bssid) {
			return nil, fmt.Errorf("invalid BSSID %q", bssid)
		}
	}

	var payload []byte
	for _, bssid := range bssids {
		wifi := appendBytesField(nil, 1, []byte(bssid))
		wifi = appendVarintField(wifi, 3, 0)
		wifi = appendVarintField(wifi, 4, 1)
		payload = appendBytesField(payload, 2, wifi)
	}

	out := make([]byte, 0, 2+2+10+2+19+2+13+8+len(payload))
	var u16 [2]byte
	binary.BigEndian.PutUint16(u16[:], 1)
	out = append(out, u16[:]...)
	out = appendBEString(out, "en-001_001")
	out = appendBEString(out, "com.apple.locationd")
	out = appendBEString(out, "26.6.2.23G90")

	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], 1)
	out = append(out, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], uint32(len(payload)))
	out = append(out, u32[:]...)
	out = append(out, payload...)
	return out, nil
}

func appendBEString(dst []byte, value string) []byte {
	var size [2]byte
	binary.BigEndian.PutUint16(size[:], uint16(len(value)))
	dst = append(dst, size[:]...)
	return append(dst, value...)
}
