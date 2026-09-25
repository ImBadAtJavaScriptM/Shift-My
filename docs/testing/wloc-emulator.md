# Controlled WLOC emulator

Shift-My includes a lab-only WLOC codec and responder for validating the binary
request/response format against hostnames owned by the operator.

## Scope

The responder is available only through the existing controlled TLS listener and
therefore only on the project-owned hosts returned by `netpolicy.ControlledHosts`.
It never forwards a request to a third-party service.

The endpoint is:

```text
POST https://device-loc.<public-host>/clls/wloc
```

The response includes:

```text
Content-Type: application/octet-stream
Cache-Control: no-store
X-Shift-My-Lab: controlled-wloc-emulator
```

The emulator decodes the observed ARPC-style envelope, extracts repeated Wi-Fi
BSSIDs from the embedded protobuf, and returns the currently selected dashboard
latitude/longitude for each requested BSSID using the observed 1e-8 coordinate
scale.

## Fixture coverage

`internal/wloc/codec_test.go` contains exact hex fixtures from the investigation:

- the 75-byte legacy-style WLOC request used for the controlled comparison test;
- the captured 77-byte not-found response containing the `-180,-180` sentinel.

These fixtures validate framing and protobuf parsing without contacting any
external location service.

## Manual test

After deploying this branch to a controlled lab host, send the fixture to the
controlled hostname. The example below intentionally uses `device-loc`, not a
production service hostname.

```bash
printf '%s' \
  '00010005656e5f55530013636f6d2e6170706c652e6c6f636174696f6e64000a382e312e313242343131000000010000001912130a1133343a44423a46443a34333a45333a413118002001' \
  | xxd -r -p > /tmp/wloc-request.bin

curl --fail-with-body \
  --data-binary @/tmp/wloc-request.bin \
  -H 'Content-Type: application/octet-stream' \
  -D - \
  -o /tmp/wloc-response.bin \
  'https://device-loc.<public-host>/clls/wloc'
```

A successful response is binary and carries the `X-Shift-My-Lab` marker.

## Non-goal

This emulator is for protocol validation on project-owned infrastructure. It does
not add production Apple routing, production hostname certificates, or a Core
Location bypass.


## Response-mode comparison

The controlled lab endpoint supports two response shapes through the `mode`
query parameter:

- `mode=preserve` keeps all original top-level protobuf fields while replacing
  WifiDevice location blocks. This is also the default when `mode` is omitted.
- `mode=clear-result-metadata` performs the same location rewrite but omits
  top-level fields 3 (`num_cell_results`), 4 (`num_wifi_results`), and 33
  (`device_type`) to mirror the public reference rewriter's cleanup behavior.
- `mode=patch-rich` builds a deterministic 114-record lab neighborhood
  response (100 coordinate-bearing WifiDevice entries plus 14 entries without
  Location) and then runs the response-side coordinate patcher over it. This
  models the larger response shape observed in the controlled iPhone capture
  without forwarding anything to Apple or another third party.

Each successful response includes `X-Shift-My-WLOC-Mode` with the selected
mode so captures are unambiguous.

These modes are only exposed on the existing project-owned controlled TLS
hosts.


## Opaque root fields and coordinates-only mode

Recent public implementations avoid assigning semantics to several root
AppleWLoc fields and preserve them byte-for-byte. The realistic multi-BSSID
fixture used by this lab contains:

- three actual WifiDevice messages in root field 2;
- one separate BSSID-looking value in root field 1;
- root varints 31 and 32.

The lab therefore records BSSID-looking root field 1 values separately from
WifiDevice BSSIDs. They are treated as opaque protocol state, not as an
additional WifiDevice.

The `mode=coords-only` response mode follows the minimal modern raw-wire
strategy: it changes only Location fields 1 (latitude) and 2 (longitude) in
existing WifiDevice locations. Existing horizontal accuracy, altitude,
motion, timestamp, provider, diagnostic, and unknown fields are preserved
byte-for-byte. If a WifiDevice has no Location, the mode appends a minimal
Location containing only latitude and longitude.

This mode does not infer the meaning of root fields 1, 31, or 32 and never
modifies them. The default remains `mode=preserve` so experiments are
explicit rather than silently changing baseline behavior.


## Response framing robustness

The response-side patcher accepts more than one outer body shape while remaining
lab-only:

- a normal compact or structured WLOC frame beginning at byte zero;
- a WLOC frame preceded by a short wrapper/prefix (the first 256 bytes are
  searched conservatively);
- a gzip-wrapped WLOC body, which is decompressed, patched, and recompressed;
- a raw protobuf payload when at least one valid Wi-Fi or cell Location can be
  identified and patched.

Only existing latitude/longitude fields are rewritten. Missing Location messages
are left missing, and non-coordinate protobuf fields are preserved.


## Realistic 22-BSSID request fixture

`BuildSyntheticStructuredRequestFixture(22)` creates a deterministic structured
ARPC request at the scale observed in the controlled iPhone `locationd`
capture. All BSSIDs are locally administered synthetic values in the
`02:54:4d:*` namespace; the fixture does not contain harvested access-point
identifiers.

The default 22-record fixture is 607 bytes. When posted to the controlled
`patch-rich` endpoint it produces the 114-record lab neighborhood response:
100 coordinate-bearing Wi-Fi entries and 14 entries without Location.

## Structured WLOC event logging

Every successful controlled WLOC request writes one `wloc_event=<json>` line to
the service journal. The event intentionally records shape/timing rather than
raw BSSID values:

- UTC RFC3339Nano timestamp
- controlled host and path
- mode, envelope, and function ID
- request bytes and request BSSID count
- response bytes
- patched Wi-Fi, cell, and Location counts
- selected target revision

This makes future iPhone sysdiagnoses easy to correlate with server-side events
without logging raw scan identifiers.
