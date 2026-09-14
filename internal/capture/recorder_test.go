package capture

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestDisabledRecorderCreatesNoFiles(t *testing.T) {
	dir := t.TempDir()
	r, err := New(false, dir)
	if err != nil { t.Fatal(err) }
	meta := Metadata{Host: "loc-a.lab.example.test", Method: http.MethodGet, Path: "/v1/location"}
	if err := r.RecordRequest(meta, []byte("hello")); err != nil { t.Fatal(err) }
	entries, err := os.ReadDir(dir)
	if err != nil { t.Fatal(err) }
	if len(entries) != 0 { t.Fatalf("files=%d", len(entries)) }
}

func TestRecorderRedactsSensitiveHeadersAndCapsBody(t *testing.T) {
	dir := t.TempDir()
	r, err := New(true, dir)
	if err != nil { t.Fatal(err) }
	body := []byte(strings.Repeat("x", maxBodyBytes+128))
	meta := Metadata{
		Host: "device-loc.lab.example.test",
		Method: http.MethodPost,
		Path: "/v1/location",
		ContentType: "application/json",
		Protocol: "HTTP/2.0",
		Status: 200,
		BodyLength: int64(len(body)),
		Headers: http.Header{
			"Authorization": []string{"Bearer secret"},
			"Cookie": []string{"session=secret"},
			"Set-Cookie": []string{"session=secret"},
			"Proxy-Authorization": []string{"secret"},
			"X-Lab": []string{"ok"},
		},
	}
	if err := r.RecordResponse(meta, body); err != nil { t.Fatal(err) }

	entries, err := os.ReadDir(dir)
	if err != nil { t.Fatal(err) }
	if len(entries) != 2 { t.Fatalf("files=%d want=2", len(entries)) }
	var metaBytes, bodyBytes []byte
	for _, entry := range entries {
		name := entry.Name()
		if !strings.Contains(name, "device-loc.lab.example.test") || !strings.Contains(name, "response") {
			t.Fatalf("unexpected filename %q", name)
		}
		data, err := os.ReadFile(dir + "/" + name)
		if err != nil { t.Fatal(err) }
		if strings.HasSuffix(name, ".json") { metaBytes = data }
		if strings.HasSuffix(name, ".body") { bodyBytes = data }
	}
	if len(bodyBytes) != maxBodyBytes { t.Fatalf("body bytes=%d", len(bodyBytes)) }
	var saved savedMetadata
	if err := json.Unmarshal(metaBytes, &saved); err != nil { t.Fatal(err) }
	for _, key := range []string{"Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization"} {
		if got := saved.Headers.Get(key); got != "[REDACTED]" { t.Fatalf("%s=%q", key, got) }
	}
	if got := saved.Headers.Get("X-Lab"); got != "ok" { t.Fatalf("X-Lab=%q", got) }
	if !saved.Truncated { t.Fatal("expected truncated metadata") }
	if saved.BodyLength != int64(len(body)) { t.Fatalf("body length=%d", saved.BodyLength) }
}
