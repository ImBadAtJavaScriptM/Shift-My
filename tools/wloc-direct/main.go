package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	legacyUserAgent = "locationd/1753.17 CFNetwork/889.9 Darwin/17.2.0"
	modernUserAgent = "locationd/2890.16.16 CFNetwork/1496.0.7 Darwin/23.5.0"
)

var (
	bssidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
	allowedEndpoints = map[string]struct{}{
		"https://gs-loc.apple.com/clls/wloc":          {},
		"https://gs-loc-cn.apple.com/clls/wloc":       {},
		"https://iphone-services.apple.com/clls/wloc": {},
	}
)

func main() {
	endpoint := flag.String("endpoint", "https://gs-loc.apple.com/clls/wloc", "Apple WLOC endpoint")
	mode := flag.String("mode", "legacy", "request shape: legacy or modern")
	bssid := flag.String("bssid", "34:DB:FD:43:E3:A1", "primary BSSID to query")
	out := flag.String("out", "apple-wloc-response.bin", "file for the raw Apple response")
	requestOut := flag.String("request-out", "", "optional file for the exact raw request body")
	timeout := flag.Duration("timeout", 20*time.Second, "HTTP timeout")
	flag.Parse()

	if _, ok := allowedEndpoints[*endpoint]; !ok {
		fmt.Fprintln(os.Stderr, "endpoint not allowed; use one of:")
		for candidate := range allowedEndpoints {
			fmt.Fprintln(os.Stderr, " ", candidate)
		}
		os.Exit(2)
	}
	if !bssidPattern.MatchString(*bssid) {
		fmt.Fprintln(os.Stderr, "invalid BSSID; expected XX:XX:XX:XX:XX:XX")
		os.Exit(2)
	}

	var requestBody []byte
	var userAgent string
	switch strings.ToLower(*mode) {
	case "legacy":
		requestBody = buildLegacyWLOCRequest(*bssid)
		userAgent = legacyUserAgent
	case "modern":
		// Keep the primary public example BSSID and add only synthetic,
		// locally-administered BSSIDs so this diagnostic does not disclose
		// nearby real access points.
		requestBody = buildModernWLOCRequest([]string{
			*bssid,
			"02:00:00:00:00:01",
			"02:00:00:00:00:02",
		})
		userAgent = modernUserAgent
	default:
		fmt.Fprintln(os.Stderr, "invalid -mode; use legacy or modern")
		os.Exit(2)
	}

	if *requestOut != "" {
		if err := os.WriteFile(*requestOut, requestBody, 0o600); err != nil {
			fatal(err)
		}
	}
	reqHash := sha256.Sum256(requestBody)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *endpoint, bytes.NewReader(requestBody))
	if err != nil {
		fatal(err)
	}
	req.Header.Set("User-Agent", userAgent)
	if strings.EqualFold(*mode, "modern") {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Charset", "utf-8")
		req.Header.Set("Accept-Language", "en-us")
	}

	client := &http.Client{Timeout: *timeout}
	resp, err := client.Do(req)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, body, 0o600); err != nil {
		fatal(err)
	}
	respHash := sha256.Sum256(body)

	fmt.Printf("endpoint: %s\n", *endpoint)
	fmt.Printf("mode: %s\n", strings.ToLower(*mode))
	fmt.Printf("bssid: %s\n", *bssid)
	fmt.Printf("request_bytes: %d\n", len(requestBody))
	fmt.Printf("request_sha256: %x\n", reqHash)
	if *requestOut != "" {
		fmt.Printf("request_saved: %s\n", *requestOut)
	}
	fmt.Printf("http_status: %s\n", resp.Status)
	fmt.Printf("content_type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("response_bytes: %d\n", len(body))
	fmt.Printf("response_sha256: %x\n", respHash)
	fmt.Printf("response_base64: %s\n", base64.StdEncoding.EncodeToString(body))
	fmt.Printf("saved: %s\n", *out)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		os.Exit(1)
	}
}

func buildLegacyWLOCRequest(bssid string) []byte {
	wifiDevice := append([]byte{0x0a, 0x11}, []byte(bssid)...)

	payload := make([]byte, 0, 2+len(wifiDevice)+4)
	payload = append(payload, 0x12, byte(len(wifiDevice)))
	payload = append(payload, wifiDevice...)
	payload = append(payload, 0x18, 0x00, 0x20, 0x01)

	frame := make([]byte, 0, 64+len(payload))
	frame = append(frame, 0x00, 0x01, 0x00, 0x05)
	frame = append(frame, []byte("en_US")...)
	frame = append(frame, 0x00, 0x13)
	frame = append(frame, []byte("com.apple.locationd")...)
	frame = append(frame, 0x00, 0x0a)
	frame = append(frame, []byte("8.1.12B411")...)
	frame = append(frame, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00)
	frame = append(frame, byte(len(payload)))
	frame = append(frame, payload...)
	return frame
}

func buildModernWLOCRequest(bssids []string) []byte {
	var payload []byte
	for _, bssid := range bssids {
		wifi := appendFieldString(nil, 1, bssid)
		payload = appendFieldBytes(payload, 2, wifi)
	}

	// Current public CoreLocation research models these as sint32 fields.
	payload = appendFieldVarint(payload, 3, zigzag32(0))
	payload = appendFieldVarint(payload, 4, zigzag32(0))

	var device []byte
	device = appendFieldString(device, 1, "iPhone OS17.5/21F79")
	device = appendFieldString(device, 2, "iPhone12,1")
	payload = appendFieldBytes(payload, 33, device)

	return buildARPC(
		1,
		"en-001_001",
		"com.apple.locationd",
		"18.6.2.22G100",
		1,
		payload,
	)
}

func buildARPC(version uint16, locale, appIdentifier, osVersion string, functionID uint32, payload []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, version)
	writePascalString(&buf, locale)
	writePascalString(&buf, appIdentifier)
	writePascalString(&buf, osVersion)
	_ = binary.Write(&buf, binary.BigEndian, functionID)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(payload)))
	_, _ = buf.Write(payload)
	return buf.Bytes()
}

func writePascalString(buf *bytes.Buffer, value string) {
	_ = binary.Write(buf, binary.BigEndian, uint16(len(value)))
	_, _ = buf.WriteString(value)
}

func appendFieldString(dst []byte, field int, value string) []byte {
	return appendFieldBytes(dst, field, []byte(value))
}

func appendFieldBytes(dst []byte, field int, value []byte) []byte {
	dst = appendVarint(dst, uint64(field<<3|2))
	dst = appendVarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendFieldVarint(dst []byte, field int, value uint64) []byte {
	dst = appendVarint(dst, uint64(field<<3))
	return appendVarint(dst, value)
}

func appendVarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func zigzag32(value int32) uint64 {
	return uint64(uint32(value<<1) ^ uint32(value>>31))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "wloc-direct:", err)
	os.Exit(1)
}
