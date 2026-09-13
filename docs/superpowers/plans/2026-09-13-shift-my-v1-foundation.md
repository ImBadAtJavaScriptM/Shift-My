# Shift-My v1 Foundation and On-Device Capture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first working Shift-My prototype path from dashboard and profile installation through selective DoH routing, narrow TLS interception, transparent forwarding, and safe protocol capture on the user's own stock iPhone.

**Architecture:** One Go application provides the dashboard, API, profile generator, DoH handler, state store, and the two TLS services used by the experiment. Nginx `stream` SNI routing owns public TCP/443 and forwards the public hostname to the normal HTTPS listener and the explicit Apple location-host allowlist to the experiment TLS proxy. The first implementation plan deliberately stops after a successful transparent on-device capture, because the exact Apple location payload format must be observed before a correct coordinate transformer can be specified; the follow-up plan will implement the parser/serializer and coordinate mutation from that captured evidence.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite`, `github.com/miekg/dns`, `github.com/DHowett/go-plist`, `golang.org/x/net/http2`, SQLite, Nginx stream module, Certbot/Let's Encrypt, systemd, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-13-shift-my-v1-design.md`

## Global Constraints

- Stock iPhone only; no jailbreak.
- No persistent Mac/Xcode connection after setup.
- Total infrastructure budget stays at or below $15/year; prototype target is $0.
- Single-user, single-device v1.
- Operate only on the user's own device and infrastructure.
- Do not reuse ShiftMy private keys, credentials, certificates, server code, or private infrastructure.
- Profile must be removable; uninstall instructions must restore normal DNS and remove test trust.
- Do not operate an open resolver or general-purpose interception proxy.
- Diagnostic body capture is disabled by default.
- Normal logs must not store a history of the user's real physical location.
- Explicit Apple location-host allowlist for v1:
  - `gs-loc.apple.com`
  - `gs-loc-cn.apple.com`
  - `iphone-services.apple.com`
- After a target location change, the UI must instruct the user to cycle `Settings -> Privacy & Security -> Location Services` off and back on before validation.
- The root CA private key stays server-side and is never embedded in the profile.
- The project hostname uses a publicly trusted certificate; the experiment proxy uses the project test CA.

---

## Planned File Map

```text
.github/workflows/ci.yml
.gitignore
go.mod
go.sum
cmd/
  server/main.go
  ca-bootstrap/main.go
internal/
  config/config.go
  config/config_test.go
  storage/store.go
  storage/store_test.go
  location/service.go
  location/service_test.go
  dashboard/handler.go
  dashboard/handler_test.go
  netpolicy/hosts.go
  netpolicy/hosts_test.go
  pki/ca.go
  pki/ca_test.go
  pki/mint.go
  pki/mint_test.go
  profile/generator.go
  profile/generator_test.go
  doh/handler.go
  doh/handler_test.go
  capture/recorder.go
  capture/recorder_test.go
  proxy/server.go
  proxy/server_test.go
  server/public.go
  server/public_test.go
web/
  templates/index.html
  static/app.js
  static/app.css
deploy/
  nginx/shift-my-stream.conf
  systemd/shift-my.service
  scripts/bootstrap.sh
.env.example
docs/
  testing/device-setup.md
  testing/uninstall.md
  protocol/README.md
```

The responsibilities are intentionally narrow: `storage` owns persistence, `location` owns target-coordinate semantics, `profile` owns plist generation, `doh` owns RFC 8484 behavior, `pki` owns only our CA and leaf certificates, `proxy` owns the allowlisted Apple-facing reverse proxy, and `capture` owns temporary opt-in protocol recording.

---

### Task 1: Repository Skeleton, Configuration, SQLite State, and CI

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `.env.example`
- Create: `cmd/server/main.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/storage/store.go`
- Create: `internal/storage/store_test.go`
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Produces: `config.Load() (config.Config, error)`
- Produces: `storage.Open(path string) (*storage.Store, error)`
- Produces: `(*storage.Store).EnsureInstallation(token string) error`
- Produces: `(*storage.Store).Installation() (storage.Installation, error)`
- Produces: `(*storage.Store).UpdateTarget(lat, lon float64, label string) (storage.Installation, error)`
- Produces: `(*storage.Store).MarkDoHSeen(time.Time) error`
- Produces: `(*storage.Store).MarkProxySeen(time.Time) error`

- [ ] **Step 1: Add module metadata and dependency floor**

Create `go.mod` with the exact module path and initial dependencies:

```go
module github.com/ImBadAtJavaScriptM/Shift-My

go 1.22

require (
    github.com/DHowett/go-plist v1.0.1
    github.com/miekg/dns v1.1.62
    golang.org/x/net v0.30.0
    modernc.org/sqlite v1.33.1
)
```

Create `.gitignore`:

```gitignore
.env
*.db
*.db-shm
*.db-wal
captures/
certs/
bin/
.DS_Store
```

Create `.env.example`:

```dotenv
SHIFT_MY_PUBLIC_HOST=shift-my.example.invalid
SHIFT_MY_PUBLIC_IPV4=203.0.113.10
SHIFT_MY_DB_PATH=/var/lib/shift-my/shift-my.db
SHIFT_MY_CA_CERT=/var/lib/shift-my/ca/root-ca.pem
SHIFT_MY_CA_KEY=/var/lib/shift-my/ca/root-ca-key.pem
SHIFT_MY_PUBLIC_CERT=/etc/letsencrypt/live/shift-my.example.invalid/fullchain.pem
SHIFT_MY_PUBLIC_KEY=/etc/letsencrypt/live/shift-my.example.invalid/privkey.pem
SHIFT_MY_CAPTURE_ENABLED=false
SHIFT_MY_CAPTURE_DIR=/var/lib/shift-my/captures
```

- [ ] **Step 2: Write failing config tests**

Create `internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadRequiresPublicHostAndIPv4(t *testing.T) {
    t.Setenv("SHIFT_MY_PUBLIC_HOST", "")
    t.Setenv("SHIFT_MY_PUBLIC_IPV4", "")
    if _, err := Load(); err == nil {
        t.Fatal("expected missing host/IP error")
    }
}

func TestLoadAcceptsValidIPv4(t *testing.T) {
    t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
    t.Setenv("SHIFT_MY_PUBLIC_IPV4", "203.0.113.10")
    t.Setenv("SHIFT_MY_DB_PATH", t.TempDir()+"/test.db")
    t.Setenv("SHIFT_MY_CA_CERT", "/tmp/ca.pem")
    t.Setenv("SHIFT_MY_CA_KEY", "/tmp/ca-key.pem")
    t.Setenv("SHIFT_MY_PUBLIC_CERT", "/tmp/fullchain.pem")
    t.Setenv("SHIFT_MY_PUBLIC_KEY", "/tmp/privkey.pem")
    t.Setenv("SHIFT_MY_CAPTURE_ENABLED", "false")
    cfg, err := Load()
    if err != nil { t.Fatal(err) }
    if cfg.PublicHost != "test.example.com" { t.Fatalf("host=%q", cfg.PublicHost) }
}
```

- [ ] **Step 3: Run config tests and verify failure**

Run:

```bash
go test ./internal/config -v
```

Expected: FAIL because `Load` and `Config` do not exist.

- [ ] **Step 4: Implement config loading and validation**

Create `internal/config/config.go` with this public shape:

```go
package config

import (
    "fmt"
    "net"
    "os"
    "strconv"
)

type Config struct {
    PublicHost     string
    PublicIPv4     net.IP
    DBPath         string
    CACertPath     string
    CAKeyPath      string
    PublicCertPath string
    PublicKeyPath  string
    CaptureEnabled bool
    CaptureDir     string
}

func Load() (Config, error) {
    host := os.Getenv("SHIFT_MY_PUBLIC_HOST")
    ip := net.ParseIP(os.Getenv("SHIFT_MY_PUBLIC_IPV4")).To4()
    if host == "" || ip == nil {
        return Config{}, fmt.Errorf("SHIFT_MY_PUBLIC_HOST and valid SHIFT_MY_PUBLIC_IPV4 are required")
    }
    capture, err := strconv.ParseBool(envDefault("SHIFT_MY_CAPTURE_ENABLED", "false"))
    if err != nil { return Config{}, fmt.Errorf("capture flag: %w", err) }
    return Config{
        PublicHost: host,
        PublicIPv4: ip,
        DBPath: envDefault("SHIFT_MY_DB_PATH", "./shift-my.db"),
        CACertPath: envDefault("SHIFT_MY_CA_CERT", "./certs/root-ca.pem"),
        CAKeyPath: envDefault("SHIFT_MY_CA_KEY", "./certs/root-ca-key.pem"),
        PublicCertPath: envDefault("SHIFT_MY_PUBLIC_CERT", "./certs/public.pem"),
        PublicKeyPath: envDefault("SHIFT_MY_PUBLIC_KEY", "./certs/public-key.pem"),
        CaptureEnabled: capture,
        CaptureDir: envDefault("SHIFT_MY_CAPTURE_DIR", "./captures"),
    }, nil
}

func envDefault(k, d string) string {
    if v := os.Getenv(k); v != "" { return v }
    return d
}
```

- [ ] **Step 5: Write failing SQLite tests**

Create `internal/storage/store_test.go` with a temporary DB and assertions that a target update increments `LocationRevision` exactly once and survives reopening:

```go
func TestUpdateTargetPersistsAndIncrementsRevision(t *testing.T) {
    path := t.TempDir()+"/state.db"
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()
    if err := s.EnsureInstallation("token-1"); err != nil { t.Fatal(err) }
    got, err := s.UpdateTarget(40.7580, -73.9855, "Times Square")
    if err != nil { t.Fatal(err) }
    if got.LocationRevision != 1 { t.Fatalf("revision=%d", got.LocationRevision) }
    if got.SelectedLabel != "Times Square" { t.Fatalf("label=%q", got.SelectedLabel) }
}
```

- [ ] **Step 6: Run storage tests and verify failure**

Run:

```bash
go test ./internal/storage -v
```

Expected: FAIL because `Open`, `EnsureInstallation`, and `UpdateTarget` do not exist.

- [ ] **Step 7: Implement SQLite schema and store methods**

Create one `installation` row with these columns:

```sql
CREATE TABLE IF NOT EXISTS installation (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    profile_token TEXT NOT NULL,
    selected_latitude REAL,
    selected_longitude REAL,
    selected_label TEXT NOT NULL DEFAULT '',
    location_revision INTEGER NOT NULL DEFAULT 0,
    doh_seen_at TEXT,
    proxy_seen_at TEXT
);
```

`UpdateTarget` must validate latitude `[-90,90]`, longitude `[-180,180]`, update all three target fields in one transaction, and increment `location_revision = location_revision + 1`.

- [ ] **Step 8: Add the minimal server entry point**

Create `cmd/server/main.go` so it only loads config and opens the DB for now; do not add network behavior yet.

- [ ] **Step 9: Add CI**

Create `.github/workflows/ci.yml`:

```yaml
name: ci
on:
  push:
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22.x'
      - run: go test ./...
      - run: go vet ./...
```

- [ ] **Step 10: Run the full task verification**

Run:

```bash
go mod tidy
go test ./...
go vet ./...
```

Expected: all tests PASS and vet exits 0.

- [ ] **Step 11: Commit**

```bash
git add go.mod go.sum .gitignore .env.example cmd internal .github
git commit -m "feat: add Shift-My foundation and state store"
```

---

### Task 2: Target Location Service and Dashboard/API

**Files:**
- Create: `internal/location/service.go`
- Create: `internal/location/service_test.go`
- Create: `internal/dashboard/handler.go`
- Create: `internal/dashboard/handler_test.go`
- Create: `web/templates/index.html`
- Create: `web/static/app.js`
- Create: `web/static/app.css`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `storage.Store`
- Produces: `location.Service.SetTarget(lat, lon float64, label string) (storage.Installation, error)`
- Produces: `dashboard.New(store *storage.Store, loc *location.Service) http.Handler`
- HTTP: `GET /`, `GET /api/status`, `POST /api/location`

- [ ] **Step 1: Write failing location-service tests**

Test that invalid coordinates are rejected and a valid target increments the revision exactly once.

```go
func TestSetTargetRejectsInvalidLatitude(t *testing.T) {
    svc := newTestService(t)
    if _, err := svc.SetTarget(91, 0, "bad"); err == nil {
        t.Fatal("expected validation error")
    }
}
```

- [ ] **Step 2: Implement `location.Service`**

The service should be intentionally thin:

```go
type Service struct { store *storage.Store }

func New(store *storage.Store) *Service { return &Service{store: store} }

func (s *Service) SetTarget(lat, lon float64, label string) (storage.Installation, error) {
    if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
        return storage.Installation{}, fmt.Errorf("coordinates out of range")
    }
    return s.store.UpdateTarget(lat, lon, strings.TrimSpace(label))
}
```

- [ ] **Step 3: Write failing dashboard handler tests**

Use `httptest` to verify:

- `GET /api/status` returns JSON containing the current revision.
- `POST /api/location` with `{"latitude":40.758,"longitude":-73.9855,"label":"Times Square"}` returns HTTP 200 and revision 1.
- invalid JSON returns 400.

- [ ] **Step 4: Implement dashboard routes and UI**

`POST /api/location` must accept only JSON, cap request bodies to 16 KiB, and return the persisted installation state.

The HTML must visibly show these checkpoints:

```text
Profile: generated / traffic seen
DoH: waiting / seen
TLS proxy: waiting / seen
Target: label + coordinates
Revision: N
Next step: Toggle Location Services off -> on, then open Apple Maps.
```

The UI must not claim success simply because a profile was downloaded.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/location ./internal/dashboard -v
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/location internal/dashboard web cmd/server/main.go
git commit -m "feat: add target location dashboard"
```

---

### Task 3: Project Test CA, Leaf Minting, and iPhone Profile Generation

**Files:**
- Create: `cmd/ca-bootstrap/main.go`
- Create: `internal/pki/ca.go`
- Create: `internal/pki/ca_test.go`
- Create: `internal/pki/mint.go`
- Create: `internal/pki/mint_test.go`
- Create: `internal/profile/generator.go`
- Create: `internal/profile/generator_test.go`
- Modify: `internal/dashboard/handler.go`

**Interfaces:**
- Produces: `pki.GenerateRoot(commonName string) (certPEM, keyPEM []byte, err error)`
- Produces: `pki.Load(certPath, keyPath string) (*pki.Authority, error)`
- Produces: `(*pki.Authority).MintServerCertificate(host string) (tls.Certificate, error)`
- Produces: `profile.Generate(profile.Config) ([]byte, error)`
- HTTP: `GET /profile.mobileconfig`

- [ ] **Step 1: Write failing CA tests**

Generate a root, parse it with `x509.ParseCertificate`, and assert:

```go
if !cert.IsCA { t.Fatal("root must be a CA") }
if cert.KeyUsage&x509.KeyUsageCertSign == 0 { t.Fatal("root must sign certs") }
```

Mint a leaf for `gs-loc.apple.com` and verify the SAN contains exactly that hostname.

- [ ] **Step 2: Implement root generation and cached leaf minting**

Use ECDSA P-256 for the project CA and leaf certificates. Root validity: 5 years. Leaf validity: 24 hours. Each leaf must include:

```go
DNSNames: []string{host}
ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
KeyUsage: x509.KeyUsageDigitalSignature
```

Cache minted `tls.Certificate` values by hostname for their process lifetime.

- [ ] **Step 3: Add the `ca-bootstrap` CLI**

Usage:

```bash
go run ./cmd/ca-bootstrap -out ./certs
```

It must create `root-ca.pem` and `root-ca-key.pem` with key file mode `0600` and refuse to overwrite an existing key unless `-force` is explicitly supplied.

- [ ] **Step 4: Write failing profile-generation tests**

Decode the generated XML plist and assert it contains exactly:

1. one `com.apple.security.root` payload containing the DER root certificate;
2. one `com.apple.dnsSettings.managed` payload;
3. `DNSProtocol = HTTPS`;
4. `ServerURL = https://<public-host>/dns-query/<token>`;
5. `ServerAddresses` containing only the configured public IPv4;
6. `SupplementalMatchDomains` containing the three explicit Apple location hosts;
7. `AllowFailover = false`;
8. top-level `PayloadRemovalDisallowed = false`.

- [ ] **Step 5: Implement `profile.Generate` using `go-plist`**

Use cryptographically random UUIDs for every payload and the top-level configuration. Do not reuse the UUIDs or identifiers from the recovered commercial profile.

Use project identifiers such as:

```text
io.shiftmytest.location
io.shiftmytest.location.ca
io.shiftmytest.location.dns
```

- [ ] **Step 6: Add `GET /profile.mobileconfig`**

Return:

```text
Content-Type: application/x-apple-aspen-config
Content-Disposition: attachment; filename="shift-my-test.mobileconfig"
Cache-Control: no-store
```

The endpoint reads the one installation token already in SQLite; it must not create a new token on every download.

- [ ] **Step 7: Verify**

```bash
go test ./internal/pki ./internal/profile ./internal/dashboard -v
go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/ca-bootstrap internal/pki internal/profile internal/dashboard
git commit -m "feat: generate test CA and iPhone profile"
```

---

### Task 4: Tokenized Selective DNS-over-HTTPS

**Files:**
- Create: `internal/netpolicy/hosts.go`
- Create: `internal/netpolicy/hosts_test.go`
- Create: `internal/doh/handler.go`
- Create: `internal/doh/handler_test.go`
- Modify: `internal/server/public.go`

**Interfaces:**
- Produces: `netpolicy.AllowedHost(string) bool`
- Produces: `doh.New(store *storage.Store, publicIPv4 net.IP) http.Handler`
- HTTP: `GET|POST /dns-query/{token}`

- [ ] **Step 1: Write allowlist tests**

```go
func TestAllowedHost(t *testing.T) {
    yes := []string{"gs-loc.apple.com", "gs-loc-cn.apple.com", "iphone-services.apple.com"}
    for _, h := range yes { if !AllowedHost(h) { t.Fatalf("expected %s", h) } }
    if AllowedHost("example.com") { t.Fatal("unexpected host allowed") }
}
```

Normalize a single trailing dot before comparison.

- [ ] **Step 2: Write failing DoH tests**

Construct DNS wire messages with `miekg/dns` and verify:

- valid token + A query for allowlisted host returns one A record containing the experiment server IPv4;
- valid token + AAAA query for allowlisted host returns `NOERROR` with zero answers to avoid IPv6 bypass;
- invalid token returns HTTP 404 without parsing the DNS body;
- non-allowlisted host returns DNS `REFUSED`;
- successful request updates `doh_seen_at`.

- [ ] **Step 3: Implement RFC 8484 handler**

Support:

- POST body with `Content-Type: application/dns-message`;
- GET `?dns=<base64url-no-padding>` for diagnostic compatibility.

Cap request wire messages at 4096 bytes. Return `application/dns-message`. Do not forward arbitrary DNS to an upstream resolver; because the profile's supplemental domains are narrow, refusing non-allowlisted names is safer and prevents an open resolver.

- [ ] **Step 4: Wire DoH into the public HTTPS mux**

Create `internal/server/public.go` with a mux that serves dashboard routes and `/dns-query/` on the same public hostname.

- [ ] **Step 5: Verify**

```bash
go test ./internal/netpolicy ./internal/doh ./internal/server -v
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/netpolicy internal/doh internal/server
git commit -m "feat: add selective tokenized DoH"
```

---

### Task 5: Narrow Apple TLS Proxy With Transparent Forwarding

**Files:**
- Create: `internal/proxy/server.go`
- Create: `internal/proxy/server_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `pki.Authority`, `netpolicy.AllowedHost`, `storage.Store`
- Produces: `proxy.New(authority *pki.Authority, store *storage.Store, capture capture.Recorder) *proxy.Server`
- Listens internally on `127.0.0.1:9443`

- [ ] **Step 1: Write failing TLS-host rejection test**

Unit-test the certificate callback directly:

```go
_, err := srv.CertificateForHost("example.com")
if err == nil { t.Fatal("non-allowlisted host must be rejected") }
```

Verify `gs-loc.apple.com` returns a certificate whose parsed SAN includes only that hostname.

- [ ] **Step 2: Write a transparent reverse-proxy integration test**

Start an `httptest.NewTLSServer` as fake upstream, inject a custom upstream dialer into the proxy, then assert an inbound request reaches upstream with:

- method unchanged;
- path/query unchanged;
- body byte-for-byte unchanged;
- response status/body byte-for-byte unchanged.

This test proves the proxy is transparent before any capture or transform logic is added.

- [ ] **Step 3: Implement the experiment TLS listener**

Use:

```go
tls.Config{
    MinVersion: tls.VersionTLS12,
    NextProtos: []string{"h2", "http/1.1"},
    GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
        return server.CertificateForHost(hello.ServerName)
    },
}
```

Configure `golang.org/x/net/http2` on the inbound `http.Server` and set the upstream `http.Transport` with `ForceAttemptHTTP2: true`.

The upstream URL must remain `https://<original-host>` and rely on the VM's normal resolver, not the iPhone profile's DoH path.

- [ ] **Step 4: Mark successful proxy traffic**

After a request successfully reaches the allowlisted handler, call `store.MarkProxySeen(time.Now().UTC())`.

- [ ] **Step 5: Verify**

```bash
go test ./internal/proxy -v
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/proxy cmd/server/main.go
git commit -m "feat: add allowlisted transparent TLS proxy"
```

---

### Task 6: Explicit, Opt-In Protocol Capture and Diagnostics

**Files:**
- Create: `internal/capture/recorder.go`
- Create: `internal/capture/recorder_test.go`
- Modify: `internal/proxy/server.go`
- Modify: `internal/dashboard/handler.go`
- Create: `docs/protocol/README.md`

**Interfaces:**
- Produces: `capture.New(enabled bool, dir string) (capture.Recorder, error)`
- Produces: `Recorder.RecordRequest(meta Metadata, body []byte) error`
- Produces: `Recorder.RecordResponse(meta Metadata, body []byte) error`

- [ ] **Step 1: Write recorder tests**

Verify all of these behaviors:

- when disabled, no files are created;
- `Authorization`, `Cookie`, `Set-Cookie`, and `Proxy-Authorization` values are stored as `[REDACTED]`;
- body files are capped at 2 MiB;
- filenames contain timestamp, allowed hostname, direction, and a random suffix but no user-selected coordinates;
- metadata JSON includes method, URL path, content type, protocol, status, and body length.

- [ ] **Step 2: Implement safe body capture**

Capture only allowlisted-host traffic. Read and restore request/response bodies so forwarding remains unchanged. If the body exceeds 2 MiB, record the first 2 MiB and set `truncated: true` in metadata.

Do not parse or log location fields yet.

- [ ] **Step 3: Add dashboard capture-state visibility**

`GET /api/status` must expose only:

```json
{
  "capture_enabled": false,
  "doh_seen": true,
  "proxy_seen": true
}
```

It must not return the capture directory path or captured body contents.

- [ ] **Step 4: Add protocol README**

Document that raw captures may contain device identifiers or other sensitive fields, must stay out of Git, and are used only long enough to derive a sanitized minimal fixture for the follow-up coordinate-transform plan.

- [ ] **Step 5: Verify**

```bash
go test ./internal/capture ./internal/proxy ./internal/dashboard -v
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/capture internal/proxy internal/dashboard docs/protocol
git commit -m "feat: add opt-in protocol capture"
```

---

### Task 7: Public HTTPS Listener, Nginx SNI Edge, systemd, and Bootstrap

**Files:**
- Create: `internal/server/public.go`
- Create: `internal/server/public_test.go`
- Create: `deploy/nginx/shift-my-stream.conf`
- Create: `deploy/systemd/shift-my.service`
- Create: `deploy/scripts/bootstrap.sh`
- Modify: `cmd/server/main.go`
- Modify: `.env.example`

**Interfaces:**
- Public Go TLS listener: `127.0.0.1:8443`
- Experiment Go TLS listener: `127.0.0.1:9443`
- Nginx public edge: `0.0.0.0:443`
- Certbot HTTP challenge: TCP/80

- [ ] **Step 1: Write public-listener tests**

Verify the public server only accepts the configured public Host for dashboard/DoH routes and returns `421 Misdirected Request` for arbitrary Host headers.

- [ ] **Step 2: Implement `cmd/server` startup**

Startup order:

1. load config;
2. open SQLite;
3. ensure one cryptographically random profile token exists;
4. load project CA;
5. create dashboard, DoH, capture, and proxy services;
6. load public Let's Encrypt certificate/key;
7. start public TLS server on `127.0.0.1:8443`;
8. start experiment TLS server on `127.0.0.1:9443`;
9. handle SIGINT/SIGTERM with graceful shutdown.

Generate the one profile token using 32 random bytes encoded with `base64.RawURLEncoding`.

- [ ] **Step 3: Create Nginx stream configuration**

`deploy/nginx/shift-my-stream.conf` must use `ssl_preread` and an exact SNI map:

```nginx
map $ssl_preread_server_name $shift_my_backend {
    PUBLIC_HOST_PLACEHOLDER       127.0.0.1:8443;
    gs-loc.apple.com              127.0.0.1:9443;
    gs-loc-cn.apple.com           127.0.0.1:9443;
    iphone-services.apple.com     127.0.0.1:9443;
    default                       127.0.0.1:9;
}

server {
    listen 443;
    proxy_pass $shift_my_backend;
    ssl_preread on;
    proxy_connect_timeout 5s;
    proxy_timeout 60s;
}
```

`bootstrap.sh` replaces only `PUBLIC_HOST_PLACEHOLDER` with the validated configured hostname before enabling the file.

- [ ] **Step 4: Create systemd unit**

Run as a dedicated `shiftmy` user with:

```ini
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/shift-my
```

The service executable path is `/opt/shift-my/shift-my` and environment file is `/etc/shift-my/shift-my.env`.

- [ ] **Step 5: Create bootstrap script**

The script must:

1. require root;
2. install `git`, `golang-go`, `nginx`, `libnginx-mod-stream`, `certbot`, `sqlite3`, and `ufw`;
3. verify `go version` is at least 1.22;
4. create user/group `shiftmy`;
5. create `/opt/shift-my`, `/var/lib/shift-my`, `/var/lib/shift-my/ca`, `/var/lib/shift-my/captures`, `/etc/shift-my`;
6. clone/pull the repo into `/opt/shift-my/src`;
7. build `./cmd/server` and `./cmd/ca-bootstrap`;
8. generate the project CA if one does not already exist;
9. obtain the public certificate using Certbot standalone on port 80;
10. install the Nginx stream config with the real hostname;
11. install and enable the systemd unit;
12. configure UFW to allow only SSH, TCP/80, and TCP/443;
13. explicitly leave UDP/443 unopened so QUIC is not advertised by this prototype;
14. print the profile URL `https://<host>/profile.mobileconfig`.

The script must never print the CA private key or profile token.

- [ ] **Step 6: Add a dry-run syntax check**

The bootstrap script must run:

```bash
nginx -t
systemd-analyze verify /etc/systemd/system/shift-my.service
```

before enabling/restarting services.

- [ ] **Step 7: Verify locally**

```bash
go test ./...
go vet ./...
bash -n deploy/scripts/bootstrap.sh
```

Expected: PASS/exit 0.

- [ ] **Step 8: Commit**

```bash
git add internal/server cmd/server deploy .env.example
git commit -m "feat: add deployment and SNI edge"
```

---

### Task 8: Device Setup, Uninstall, and First On-Device Checkpoint

**Files:**
- Create: `docs/testing/device-setup.md`
- Create: `docs/testing/uninstall.md`
- Modify: `web/templates/index.html`

**Interfaces:**
- Human/device checkpoint; produces an evidence package for the next protocol-transform plan.

- [ ] **Step 1: Write device setup instructions**

The guide must use this exact order:

1. Open the profile URL in Safari.
2. Open Settings and install `Shift-My Test`.
3. If iOS requires it, go to `Settings -> General -> About -> Certificate Trust Settings` and enable full trust for the Shift-My test root.
4. Return to the dashboard and verify `DoH: seen` appears after the phone performs a location-related lookup.
5. Verify `TLS proxy: seen` appears.
6. Before capture is enabled, open Apple Maps and confirm normal location still works through transparent forwarding.
7. Only then set `SHIFT_MY_CAPTURE_ENABLED=true`, restart the service, and reproduce one Apple Maps location lookup.
8. Disable capture again immediately after the needed sample is collected.

- [ ] **Step 2: Add explicit ShiftMy-like refresh instructions to the UI**

After `Set Location`, show:

```text
Refresh Location Services
1. Open Settings -> Privacy & Security -> Location Services.
2. Turn Location Services OFF.
3. Wait 5 seconds.
4. Turn Location Services ON.
5. Open Apple Maps.
```

For this foundation plan, the target is stored and the refresh instruction is shown, but the captured Apple response is still forwarded unchanged until the follow-up transformer is implemented.

- [ ] **Step 3: Write uninstall/reset instructions**

Include:

1. remove the Shift-My configuration profile;
2. confirm the Shift-My root no longer appears in Certificate Trust Settings;
3. restart the iPhone if Apple location traffic remains cached;
4. verify Apple Maps returns to normal;
5. server-side kill switch: stop `shift-my` and disable the Nginx stream config if needed.

- [ ] **Step 4: Execute the server-side smoke checks on the deployed VM**

Run:

```bash
curl -fsS https://$SHIFT_MY_PUBLIC_HOST/api/status
curl -fsSI https://$SHIFT_MY_PUBLIC_HOST/profile.mobileconfig
sudo ss -lntp | grep -E ':(443|8443|9443)'
sudo nginx -t
sudo systemctl --no-pager --full status shift-my
```

Expected:

- status endpoint returns JSON;
- profile endpoint returns 200;
- Nginx owns public 443;
- Go owns localhost 8443 and 9443;
- Nginx config test passes;
- service is active.

- [ ] **Step 5: Perform the first phone checkpoint**

Success for this task means all three are observed on the dashboard/server:

```text
Profile downloaded and installed manually
DoH traffic seen from the phone
Allowlisted TLS proxy traffic seen from the phone
```

Then verify transparent forwarding does not break Apple Maps.

- [ ] **Step 6: Collect one temporary capture and create a sanitized observation note**

Do **not** commit raw capture files.

Create `docs/protocol/observed-location-exchange.md` containing only:

- hostname;
- HTTP version;
- request method/path with unique tokens removed;
- request and response content types;
- request/response byte sizes;
- whether payloads appear plist, protobuf, JSON, gzip, or another identifiable framing;
- any stable field names or message types that can be identified without retaining personal identifiers;
- whether changing real device position causes a corresponding field/byte region to change.

Do not include the user's real coordinates, cookies, authorization values, device IDs, or account identifiers.

- [ ] **Step 7: Commit documentation only**

```bash
git add docs/testing web/templates/index.html docs/protocol/observed-location-exchange.md
git commit -m "docs: record first on-device protocol checkpoint"
```

---

## Plan Boundary and Required Follow-Up

This plan intentionally ends at the first real, transparent protocol capture. That is the earliest point where the coordinate-mutation implementation can be specified without guessing Apple's current payload format or integrity checks.

After Task 8, create a second implementation plan from the captured evidence with this required scope:

```text
1. parser + serializer for the observed response format
2. byte-for-byte unchanged round-trip test
3. selected-coordinate transform wired to location revision state
4. integrity/checksum/signature handling if present
5. Apple Maps coordinate-override device test
6. ordinary third-party Location Services app test
7. Find My compatibility test
8. kill-switch and rollback verification
```

The second plan is not optional if the project is to satisfy the approved system-wide location-changing goal; it is separated only because its exact code contract depends on the protocol evidence produced by this plan.

## Self-Review Results

- **Spec coverage:** M1-M3 and the evidence-producing portion of M4 are fully covered here. M4 parser/serializer, M5 coordinate mutation, and M6 compatibility are deliberately assigned to the evidence-driven follow-up plan because their concrete message schema is not knowable before Task 8.
- **Placeholder scan:** Runtime environment values use named configuration keys; no implementation step depends on an undefined `TBD`/`TODO`.
- **Type consistency:** Storage, location, profile, DoH, PKI, proxy, capture, and dashboard interfaces are named once and consumed consistently by later tasks.
