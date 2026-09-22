package wloc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const maxStringLength = 1 << 16

// Request is the subset of the WLOC request envelope used by the controlled
// lab emulator. Unknown protobuf fields are intentionally ignored.
type Request struct {
	Version       uint16
	Locale        string
	AppIdentifier string
	OSVersion     string
	FunctionID    uint32
	BSSIDs        []string
}

// DeviceLocation is the subset of a WLOC response that the controlled lab
// emulator needs for tests and diagnostics.
type DeviceLocation struct {
	BSSID       string
	LatitudeE8  int64
	LongitudeE8 int64
}

// ParseRequest decodes the observed ARPC-style request envelope and extracts
// repeated WifiDevice BSSIDs from the embedded protobuf payload.
func ParseRequest(data []byte) (Request, error) {
	if len(data) < 2 {
		return Request{}, errors.New("request is too short")
	}
	pos := 0
	req := Request{Version: binary.BigEndian.Uint16(data[pos : pos+2])}
	pos += 2

	var err error
	if req.Locale, err = readBEString(data, &pos); err != nil {
		return Request{}, fmt.Errorf("locale: %w", err)
	}
	if req.AppIdentifier, err = readBEString(data, &pos); err != nil {
		return Request{}, fmt.Errorf("app identifier: %w", err)
	}
	if req.OSVersion, err = readBEString(data, &pos); err != nil {
		return Request{}, fmt.Errorf("os version: %w", err)
	}
	if len(data)-pos < 8 {
		return Request{}, errors.New("request envelope is truncated")
	}
	req.FunctionID = binary.BigEndian.Uint32(data[pos : pos+4])
	pos += 4
	payloadLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
	pos += 4
	if payloadLen < 0 || payloadLen != len(data)-pos {
		return Request{}, fmt.Errorf("payload length %d does not match %d remaining bytes", payloadLen, len(data)-pos)
	}

	req.BSSIDs, err = parseAppleWLocBSSIDs(data[pos:])
	if err != nil {
		return Request{}, err
	}
	if len(req.BSSIDs) == 0 {
		return Request{}, errors.New("request contains no wifi devices")
	}
	return req, nil
}

// BuildResponse returns the observed 10-byte WLOC response envelope followed by
// an AppleWLoc protobuf containing one target location for every requested BSSID.
func BuildResponse(req Request, latitude, longitude float64) ([]byte, error) {
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return nil, errors.New("coordinates out of range")
	}
	if len(req.BSSIDs) == 0 {
		return nil, errors.New("at least one BSSID is required")
	}

	latE8 := int64(math.Round(latitude * 1e8))
	lonE8 := int64(math.Round(longitude * 1e8))
	payload := make([]byte, 0, len(req.BSSIDs)*48)
	for _, bssid := range req.BSSIDs {
		if bssid == "" {
			return nil, errors.New("empty BSSID")
		}
		location := make([]byte, 0, 24)
		location = appendVarintField(location, 1, latE8)
		location = appendVarintField(location, 2, lonE8)

		wifi := make([]byte, 0, len(bssid)+len(location)+8)
		wifi = appendBytesField(wifi, 1, []byte(bssid))
		wifi = appendBytesField(wifi, 2, location)
		payload = appendBytesField(payload, 2, wifi)
	}

	frame := make([]byte, 10, 10+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], req.Version)
	binary.BigEndian.PutUint32(frame[2:6], req.FunctionID)
	binary.BigEndian.PutUint32(frame[6:10], uint32(len(payload)))
	frame = append(frame, payload...)
	return frame, nil
}

// ParseResponse extracts the response envelope and target coordinates from the
// limited protobuf subset emitted by BuildResponse and by the captured fixtures.
func ParseResponse(data []byte) (uint16, uint32, []DeviceLocation, error) {
	if len(data) < 10 {
		return 0, 0, nil, errors.New("response is too short")
	}
	version := binary.BigEndian.Uint16(data[0:2])
	functionID := binary.BigEndian.Uint32(data[2:6])
	payloadLen := int(binary.BigEndian.Uint32(data[6:10]))
	if payloadLen != len(data)-10 {
		return 0, 0, nil, fmt.Errorf("payload length %d does not match %d remaining bytes", payloadLen, len(data)-10)
	}
	devices, err := parseAppleWLocLocations(data[10:])
	if err != nil {
		return 0, 0, nil, err
	}
	return version, functionID, devices, nil
}

func readBEString(data []byte, pos *int) (string, error) {
	if len(data)-*pos < 2 {
		return "", errors.New("length is truncated")
	}
	n := int(binary.BigEndian.Uint16(data[*pos : *pos+2]))
	*pos += 2
	if n > maxStringLength || n > len(data)-*pos {
		return "", errors.New("value is truncated")
	}
	value := string(data[*pos : *pos+n])
	*pos += n
	return value, nil
}

func parseAppleWLocBSSIDs(payload []byte) ([]string, error) {
	var bssids []string
	for pos := 0; pos < len(payload); {
		field, wire, value, next, err := nextField(payload, pos)
		if err != nil {
			return nil, fmt.Errorf("apple wloc protobuf: %w", err)
		}
		pos = next
		if field != 2 || wire != 2 {
			continue
		}
		bssid, err := parseWifiDeviceBSSID(value)
		if err != nil {
			return nil, err
		}
		if bssid != "" {
			bssids = append(bssids, bssid)
		}
	}
	return bssids, nil
}

func parseWifiDeviceBSSID(data []byte) (string, error) {
	for pos := 0; pos < len(data); {
		field, wire, value, next, err := nextField(data, pos)
		if err != nil {
			return "", fmt.Errorf("wifi device protobuf: %w", err)
		}
		pos = next
		if field == 1 && wire == 2 {
			return string(value), nil
		}
	}
	return "", nil
}

func parseAppleWLocLocations(payload []byte) ([]DeviceLocation, error) {
	var devices []DeviceLocation
	for pos := 0; pos < len(payload); {
		field, wire, value, next, err := nextField(payload, pos)
		if err != nil {
			return nil, fmt.Errorf("apple wloc protobuf: %w", err)
		}
		pos = next
		if field != 2 || wire != 2 {
			continue
		}
		device, err := parseWifiDeviceLocation(value)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func parseWifiDeviceLocation(data []byte) (DeviceLocation, error) {
	var device DeviceLocation
	for pos := 0; pos < len(data); {
		field, wire, value, next, err := nextField(data, pos)
		if err != nil {
			return DeviceLocation{}, fmt.Errorf("wifi device protobuf: %w", err)
		}
		pos = next
		switch {
		case field == 1 && wire == 2:
			device.BSSID = string(value)
		case field == 2 && wire == 2:
			lat, lon, err := parseLocation(value)
			if err != nil {
				return DeviceLocation{}, err
			}
			device.LatitudeE8, device.LongitudeE8 = lat, lon
		}
	}
	return device, nil
}

func parseLocation(data []byte) (int64, int64, error) {
	var lat, lon int64
	for pos := 0; pos < len(data); {
		field, wire, value, next, err := nextField(data, pos)
		if err != nil {
			return 0, 0, fmt.Errorf("location protobuf: %w", err)
		}
		pos = next
		if wire != 0 || (field != 1 && field != 2) {
			continue
		}
		u, n := binary.Uvarint(value)
		if n <= 0 {
			return 0, 0, errors.New("location varint is invalid")
		}
		if field == 1 {
			lat = int64(u)
		} else {
			lon = int64(u)
		}
	}
	return lat, lon, nil
}

// nextField returns the field number, wire type and encoded field value. For
// wire type 0 the returned value contains the varint bytes themselves; for wire
// type 2 it contains only the length-delimited payload.
func nextField(data []byte, pos int) (int, int, []byte, int, error) {
	key, n := binary.Uvarint(data[pos:])
	if n <= 0 {
		return 0, 0, nil, pos, errors.New("field key varint is invalid")
	}
	pos += n
	field := int(key >> 3)
	wire := int(key & 7)
	if field == 0 {
		return 0, 0, nil, pos, errors.New("field number 0 is invalid")
	}

	switch wire {
	case 0:
		start := pos
		_, n := binary.Uvarint(data[pos:])
		if n <= 0 {
			return 0, 0, nil, pos, errors.New("varint field is invalid")
		}
		pos += n
		return field, wire, data[start:pos], pos, nil
	case 1:
		if len(data)-pos < 8 {
			return 0, 0, nil, pos, errors.New("fixed64 field is truncated")
		}
		return field, wire, data[pos : pos+8], pos + 8, nil
	case 2:
		n64, n := binary.Uvarint(data[pos:])
		if n <= 0 {
			return 0, 0, nil, pos, errors.New("length-delimited size is invalid")
		}
		pos += n
		if n64 > uint64(len(data)-pos) {
			return 0, 0, nil, pos, errors.New("length-delimited field is truncated")
		}
		end := pos + int(n64)
		return field, wire, data[pos:end], end, nil
	case 5:
		if len(data)-pos < 4 {
			return 0, 0, nil, pos, errors.New("fixed32 field is truncated")
		}
		return field, wire, data[pos : pos+4], pos + 4, nil
	default:
		return 0, 0, nil, pos, fmt.Errorf("unsupported protobuf wire type %d", wire)
	}
}

func appendBytesField(dst []byte, field int, value []byte) []byte {
	dst = appendUvarint(dst, uint64(field<<3|2))
	dst = appendUvarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendVarintField(dst []byte, field int, value int64) []byte {
	dst = appendUvarint(dst, uint64(field<<3))
	return appendUvarint(dst, uint64(value))
}

func appendUvarint(dst []byte, value uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], value)
	return append(dst, buf[:n]...)
}
