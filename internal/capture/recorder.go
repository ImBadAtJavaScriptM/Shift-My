package capture

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxBodyBytes = 2 << 20

var safeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Metadata struct {
	Host        string
	Method      string
	Path        string
	ContentType string
	Protocol    string
	Status      int
	BodyLength  int64
	Headers     http.Header
}

type Recorder interface {
	RecordRequest(meta Metadata, body []byte) error
	RecordResponse(meta Metadata, body []byte) error
}

type recorder struct {
	enabled bool
	dir     string
}

type savedMetadata struct {
	Host        string      `json:"host"`
	Direction   string      `json:"direction"`
	Method      string      `json:"method,omitempty"`
	Path        string      `json:"path,omitempty"`
	ContentType string      `json:"content_type,omitempty"`
	Protocol    string      `json:"protocol,omitempty"`
	Status      int         `json:"status,omitempty"`
	BodyLength  int64       `json:"body_length"`
	Truncated   bool        `json:"truncated"`
	Headers     http.Header `json:"headers,omitempty"`
	CapturedAt  time.Time   `json:"captured_at"`
}

func New(enabled bool, dir string) (Recorder, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("capture directory is required")
	}
	if enabled {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create capture directory: %w", err)
		}
	}
	return &recorder{enabled: enabled, dir: dir}, nil
}

func (r *recorder) RecordRequest(meta Metadata, body []byte) error {
	return r.record("request", meta, body)
}

func (r *recorder) RecordResponse(meta Metadata, body []byte) error {
	return r.record("response", meta, body)
}

func (r *recorder) record(direction string, meta Metadata, body []byte) error {
	if !r.enabled {
		return nil
	}
	now := time.Now().UTC()
	suffix, err := randomSuffix()
	if err != nil {
		return err
	}
	host := safeName.ReplaceAllString(strings.ToLower(strings.TrimSpace(meta.Host)), "_")
	if host == "" {
		host = "unknown-host"
	}
	base := fmt.Sprintf("%s_%s_%s_%s", now.Format("20060102T150405.000000000Z"), host, direction, suffix)
	captured := body
	truncated := false
	if len(captured) > maxBodyBytes {
		captured = captured[:maxBodyBytes]
		truncated = true
	}
	bodyPath := filepath.Join(r.dir, base+".body")
	metaPath := filepath.Join(r.dir, base+".json")
	if err := os.WriteFile(bodyPath, captured, 0600); err != nil {
		return fmt.Errorf("write capture body: %w", err)
	}
	saved := savedMetadata{
		Host: meta.Host, Direction: direction, Method: meta.Method, Path: meta.Path,
		ContentType: meta.ContentType, Protocol: meta.Protocol, Status: meta.Status,
		BodyLength: meta.BodyLength, Truncated: truncated, Headers: redactHeaders(meta.Headers), CapturedAt: now,
	}
	encoded, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		_ = os.Remove(bodyPath)
		return fmt.Errorf("encode capture metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, encoded, 0600); err != nil {
		_ = os.Remove(bodyPath)
		return fmt.Errorf("write capture metadata: %w", err)
	}
	return nil
}

func redactHeaders(src http.Header) http.Header {
	out := make(http.Header, len(src))
	for key, values := range src {
		if isSensitiveHeader(key) {
			out.Set(key, "[REDACTED]")
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	return out
}

func isSensitiveHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization":
		return true
	default:
		return false
	}
}

func randomSuffix() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("capture random suffix: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
