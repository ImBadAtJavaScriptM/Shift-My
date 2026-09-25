package wloc

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const maxResponseFrameScan = 256

// PatchResponseCoordinatesOnly rewrites only existing latitude/longitude fields
// in a controlled WLOC response. It preserves the surrounding response shape,
// supports gzip-wrapped bodies, tolerates a short prefix before the WLOC frame,
// and falls back to scanning a raw protobuf payload when no standard envelope
// is present.
//
// It never synthesizes a missing Location message and never contacts an
// external service.
func PatchResponseCoordinatesOnly(data []byte, latitude, longitude float64) ([]byte, ResponsePatchStats, error) {
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return nil, ResponsePatchStats{}, errors.New("coordinates out of range")
	}

	if isGzipBody(data) {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, ResponsePatchStats{}, fmt.Errorf("open gzip WLOC response: %w", err)
		}
		header := reader.Header
		raw, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return nil, ResponsePatchStats{}, fmt.Errorf("read gzip WLOC response: %w", err)
		}
		if closeErr != nil {
			return nil, ResponsePatchStats{}, fmt.Errorf("close gzip WLOC response: %w", closeErr)
		}

		patched, stats, err := patchResponseCoordinatesOnlyUncompressed(raw, latitude, longitude)
		if err != nil {
			return nil, ResponsePatchStats{}, err
		}
		if stats.Locations == 0 {
			return append([]byte(nil), data...), stats, nil
		}

		var out bytes.Buffer
		writer := gzip.NewWriter(&out)
		writer.Header = header
		if _, err := writer.Write(patched); err != nil {
			return nil, ResponsePatchStats{}, fmt.Errorf("write gzip WLOC response: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, ResponsePatchStats{}, fmt.Errorf("close gzip WLOC response: %w", err)
		}
		return out.Bytes(), stats, nil
	}

	return patchResponseCoordinatesOnlyUncompressed(data, latitude, longitude)
}

func patchResponseCoordinatesOnlyUncompressed(data []byte, latitude, longitude float64) ([]byte, ResponsePatchStats, error) {
	// Keep the exact legacy behavior first for the common case where the frame
	// begins at byte zero and consumes the entire body.
	if patched, stats, err := patchResponseCoordinatesOnlyStrict(data, latitude, longitude); err == nil {
		return patched, stats, nil
	}

	maxOffset := len(data) - 1
	if maxOffset > maxResponseFrameScan {
		maxOffset = maxResponseFrameScan
	}

	// Some capture/rewrite stacks prepend a small wrapper before the WLOC
	// frame. Locate a complete compact or structured frame, patch only that
	// frame, and preserve prefix/suffix bytes exactly.
	for offset := 1; offset <= maxOffset; offset++ {
		for _, end := range candidateFrameEnds(data, offset) {
			frame := data[offset:end]
			patchedFrame, stats, err := patchResponseCoordinatesOnlyStrict(frame, latitude, longitude)
			if err != nil || stats.Locations == 0 {
				continue
			}
			out := make([]byte, 0, len(data)-len(frame)+len(patchedFrame))
			out = append(out, data[:offset]...)
			out = append(out, patchedFrame...)
			out = append(out, data[end:]...)
			return out, stats, nil
		}
	}

	// Raw protobuf fallback for controlled fixtures that omit the ARPC/compact
	// envelope entirely. Requiring at least one patchable Wi-Fi/cell Location
	// keeps the scan conservative.
	latE8 := int64(math.Trunc(latitude * 1e8))
	lonE8 := int64(math.Trunc(longitude * 1e8))
	for offset := 0; offset <= maxOffset; offset++ {
		patchedPayload, stats, err := patchExistingResponsePayload(data[offset:], latE8, lonE8)
		if err != nil || stats.Locations == 0 {
			continue
		}
		out := make([]byte, 0, offset+len(patchedPayload))
		out = append(out, data[:offset]...)
		out = append(out, patchedPayload...)
		return out, stats, nil
	}

	return nil, ResponsePatchStats{}, errors.New("unsupported WLOC response framing")
}

func candidateFrameEnds(data []byte, offset int) []int {
	var ends []int
	if end, ok := compactFrameEnd(data, offset); ok {
		ends = append(ends, end)
	}
	if end, ok := structuredFrameEnd(data, offset); ok {
		duplicate := false
		for _, existing := range ends {
			if existing == end {
				duplicate = true
				break
			}
		}
		if !duplicate {
			ends = append(ends, end)
		}
	}
	return ends
}

func compactFrameEnd(data []byte, offset int) (int, bool) {
	if offset < 0 || len(data)-offset < 10 {
		return 0, false
	}
	payloadLen := int(binary.BigEndian.Uint32(data[offset+6 : offset+10]))
	end := offset + 10 + payloadLen
	if payloadLen <= 0 || end > len(data) {
		return 0, false
	}
	return end, true
}

func structuredFrameEnd(data []byte, offset int) (int, bool) {
	if offset < 0 || len(data)-offset < 2 {
		return 0, false
	}
	pos := offset + 2
	for i := 0; i < 3; i++ {
		if _, err := readBEString(data, &pos); err != nil {
			return 0, false
		}
	}
	if len(data)-pos < 8 {
		return 0, false
	}
	payloadLen := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
	end := pos + 8 + payloadLen
	if payloadLen <= 0 || end > len(data) {
		return 0, false
	}
	return end, true
}

func isGzipBody(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}
