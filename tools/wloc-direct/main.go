package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

const userAgent = "locationd/1753.17 CFNetwork/889.9 Darwin/17.2.0"

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
	bssid := flag.String("bssid", "34:DB:FD:43:E3:A1", "BSSID to query")
	out := flag.String("out", "apple-wloc-response.bin", "file for the raw Apple response")
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

	requestBody := buildWLOCRequest(*bssid)
	reqHash := sha256.Sum256(requestBody)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *endpoint, bytes.NewReader(requestBody))
	if err != nil {
		fatal(err)
	}
	req.Header.Set("User-Agent", userAgent)

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
	fmt.Printf("bssid: %s\n", *bssid)
	fmt.Printf("request_bytes: %d\n", len(requestBody))
	fmt.Printf("request_sha256: %x\n", reqHash)
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

func buildWLOCRequest(bssid string) []byte {
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

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "wloc-direct:", err)
	os.Exit(1)
}
