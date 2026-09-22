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

Each successful response includes `X-Shift-My-WLOC-Mode` with the selected
mode so captures are unambiguous.

These modes are only exposed on the existing project-owned controlled TLS
hosts.
