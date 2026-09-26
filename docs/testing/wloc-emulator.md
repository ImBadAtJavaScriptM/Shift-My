[Reading 294 lines from start (total: 294 lines, 0 remaining)]

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


## Empirical WifiPosition lab model

`EstimateWifiPosition` models the post-ALS handoff observed after
`Network::AlsFinished` without attempting to call or hook private CoreLocation
APIs. The model:

1. deduplicates the live scan by BSSID, retaining the strongest RSSI;
2. matches scan BSSIDs against response records with usable locations;
3. sorts matches by RSSI and retains at most the strongest 18;
4. computes the arithmetic centroid of the retained AP locations.

This is intentionally described as an empirical approximation rather than
Apple's exact proprietary positioning algorithm. Trace analysis across 15
successful cycles shows a hard working-set cap of 18 ALS-located APs: when 20,
21, or 25 usable candidates are present, the corresponding result records use
18 and reject 2, 3, or 7 respectively; when only 18 candidates are present,
all 18 are retained. The rejected entries are the weakest RSSI observations in
those over-cap samples. The arithmetic centroid remains only a simple lab
approximation of the later private weighting/fusion stage.

The controlled 22-BSSID / 114-record `patch-rich` fixture is also covered by an
integration test: all 22 request BSSIDs match valid response locations, the
strongest 18 are selected, and the model resolves exactly to the selected lab
coordinate because all coordinate-bearing response entries were patched to the
same target.

The model is a test harness only. It does not inject data into `locationd`,
trigger `Network::AlsFinished`, or alter Apple production traffic.


## Empirical WifiPosition model

The controlled lab includes a post-ALS position estimator for studying the
`Network::AlsFinished -> WifiPosition` stage without modifying iOS. It is
deliberately labeled empirical rather than an implementation of Apple's private
positioning algorithm.

The current model:

1. deduplicates the live scan by BSSID, retaining the strongest RSSI;
2. joins scan BSSIDs against WLOC response entries with usable locations;
3. sorts matched APs by RSSI and retains at most the strongest 18;
4. returns the arithmetic centroid of those AP coordinates.

The selection stage is trace-backed; the final coordinate solver is not. The
private logs show a later 2.4GHz / stage1+5GHz fusion stage and additional
unlabeled numeric weights. Because those weights are not publicly documented,
the lab model deliberately keeps a simple centroid rather than claiming to
reproduce Apple's exact final solve.

Server-side `wloc_event` estimates are emitted only for the deterministic
synthetic request fixture, where scan RSSIs are known by construction. Arbitrary
requests do not receive a modeled estimate because the WLOC request alone does
not provide the live scan RSSI values needed by this model.


## Observed ALS requester lifecycle

The unified-log traces show that the os_activity ID is useful for following a
WifiPosition execution context, but it is not a stable network-request
identifier. The same ALS requester can be issued under one activity and receive
its network callbacks under another. The opaque numeric requester token in the
ALS tuple is the stronger correlation key across queryLocation / unifiedQuery,
didReceiveResponse, and requesterDidFinish.

A successful observed lifecycle has this shape:

1. WifiPosition receives a live Wi-Fi scan and carries BSSID/RSSI/channel data
   through its Stage1/Stage2 logic.
2. NetworkProvider issues an ALS query; Network::AlsRequestResult is emitted
   almost immediately after dispatch.
3. CFNetwork completes the WLOC HTTP request asynchronously.
4. ALS parses the response, emits didReceiveResponse, then requesterDidFinish.
5. WifiPosition receives Network::AlsFinished and rematches the current scan
   against ALS/tile state.
6. Each scanned BSSID is classified as ALS-located, tile-located, unknown, or
   not-in-db.
7. Usable overlap can produce fix; zero overlap can still receive
   Network::AlsFinished, followed by nofix and Network::AlsAllUnknown.

Therefore Network::AlsFinished means "the requester completed; rematch the
current scan," not "a position fix is ready."

### Observed timing

The trace timebase is 24 MHz mach time. In one successful network cycle:

- unifiedQuery -> Network::AlsRequestResult: about 55 microseconds;
- request -> didReceiveResponse: about 1.02 seconds;
- didReceiveResponse -> requesterDidFinish: about 0.97 ms;
- requesterDidFinish -> Network::AlsFinished: about 53.5 ms;
- Network::AlsFinished -> source classification: about 2.6 ms;
- classification -> ALS result: about 1.45 ms;
- ALS result -> fix: under 0.4 ms.

In an all-unknown sample, requesterDidFinish -> Network::AlsFinished took about
18.3 ms. The handler classified the scan, emitted nofix, and entered
Network::AlsAllUnknown about 0.12 ms later. This strongly supports an in-memory
async completion dispatch rather than a disk-watcher handoff.

### Source classification

The tilesals tuple is consistent with integer percentages of the current scan.
Examples:

- 72 | 0 | 24 | 4 on 25 scanned APs corresponds to 18 ALS-located, 0
  tile-only, 6 unknown, and 1 not-in-db;
- 74 | 0 | 25 | 0 on 27 scanned APs corresponds to 20 ALS-located, 0
  tile-only, 7 unknown, and 0 not-in-db;
- 0 | 0 | 0 | 100 is the all-not-in-db/no-fix path.

The working-set behavior is consistent across the parsed successful cycles.
Observed usable-candidate counts of 18, 20, 21, and 25 map to selected counts of
18, 18, 18, and 18, with rejected counts of 0, 2, 3, and 7. Comparing the
candidate RSSIs shows that the over-cap samples retain the strongest 18. This
establishes the observed selection rule for these captures, while the later
coordinate weighting/fusion algorithm remains private and only partially
decoded.

EvaluateALSLifecycle models only the well-supported post-requester boundary. It
reports scan totals, ALS-located count, response records without usable
locations, not-returned BSSIDs, overlap percentages, an observed working-set
hint of 18, and a lab outcome. It does not create ALS requester objects, invoke
private APIs, emit private events, or alter Apple production traffic.

The trace-backed strongest-18 selection is followed in the lab by the
cross-validated RSSI/accuracy weighting described below. That weighting remains
an empirical approximation; the exact private CoreLocation fusion formula is
not claimed to be recovered.


## Trace-validated horizontal solver approximation

Further analysis across successful `WifiPosition` cycles shows that the horizontal
ALS working set is capped at 18 APs. Observed examples include 18 candidates ->
18 used, 20 -> 18, 21 -> 18, and 25 -> 18. When the candidate set exceeds 18,
the retained set follows the strongest RSSI observations; there is no fixed RSSI
floor because very weak APs remain when the candidate set is already at or below
18.

After that trace-backed selection stage, the lab estimator uses a deliberately
mild empirical weight:

`weight = exp(0.009 * (RSSI + 100)) / horizontal_accuracy^0.35`

The +100 RSSI shift is normalization-only and cancels out after weights are
normalized. The weighting law is not claimed to be Apple's private formula.
It was fitted on distinct successful cycles from one parsed trace and validated
against a separate capture. The held-out capture remained sub-meter across the
observed successful cycles, while the trace-backed 18-AP selection rule held in
both captures.

Two anonymized regression fixtures preserve relative AP geometry, RSSI, and
horizontal accuracy from independent captures while replacing all BSSIDs and
translating the coordinate frame. They verify both an 18-candidate/no-drop case
and a 20-candidate/top-18 case without storing the original network identifiers
or physical coordinates.

## Deferred ALS completion dispatch model

`ALSCompletionDispatcher` models the asynchronous boundary suggested by the
trace: parsed `requesterDidFinish` completions merge into a shared in-memory AP
location service and mark work pending. A later `Flush` coalesces one or more
completed requesters into a single `Network::AlsFinished`-style re-evaluation of
the current scan. The lab model can therefore reproduce both a successful fix
and the observed `Network::AlsAllUnknown` path without calling private
CoreLocation APIs or generating production Apple traffic.


## ALS completion timing and cached re-evaluation

Relative monotonic timestamps remain useful even when a single TraceV3 file
cannot recover absolute wall-clock time. The parsed trace counter uses the
device's ~24 MHz mach timebase; longer embedded wall-clock spans independently
validate a rate near 24 million ticks per second. The parser's synthetic 1970
timestamp must therefore not be treated as if the raw `time` value were
nanoseconds.

Two independent captures produced essentially the same completion-triggered
median: about 18.267 ms and 18.277 ms from the most recent
`requesterDidFinish` to the first `Network::AlsFinished`. Most ordinary cases
fell in the roughly 6-28 ms range, with occasional scheduler/coalescing cases
around 57-65 ms.

Multiple requester completions can be coalesced before the provider runs. One
captured burst contained two completions spanning about 2.47 ms and dispatched
about 12.52 ms after the later completion. A larger burst contained four
completions spread across about 114.88 ms; the provider dispatched about
18.92 ms after the final completion (about 133.80 ms after the earliest one).
This does not look like a fixed-duration debounce timer, so the lab model
represents dispatch as deferred/coalescible instead of sleeping for a hard-coded
duration.

The traces also contain repeated `Network::AlsFinished` passes with no new
`requesterDidFinish` immediately beforehand. Measured cached follow-on passes
occurred about 1.70-11.11 ms after the prior provider pass. In a captured
four-completion burst, three consecutive provider passes reused the same
27-BSSID scan, the same 18-used/2-rejected working set, and the same final
coordinate. `ReevaluateCached` models these passes as idempotent cached
re-evaluations rather than new network input; it is allowed only after an
initial completion-triggered dispatch and while no new completion remains
pending.


## Requester serial semantics

The private ALS tuple contains two monotonic-looking serial fields. The lab names
`IssuedSerial` and `CompletedSerial` are descriptive only, but the trace supports
these practical semantics:

- the issued serial advances with high-level `queryLocation` / `unifiedQuery`
  calls;
- one issued serial may fan out into several requester tokens and several
  network completions;
- the completed serial advances once per completed requester;
- a late child completion from an older issued serial can arrive after one
  provider pass and be coalesced with completions from a newer issued serial.

Because of that fan-out, `IssuedSerial - CompletedSerial` is retained only as a
serial-gap hint and must not be interpreted as the number of pending HTTP
requests. The dispatcher intentionally coalesces completed requester tokens
without requiring them to share one issued serial.

## Solver state beyond visible AP fields

Repeated captures with identical visible AP latitude/longitude, horizontal
accuracy, and RSSI can still produce centimeter-scale changes in the final
Wi-Fi fix. One repeated cycle also shows a 1 dBm RSSI increase accompanied by
an internal timestamp refresh, with the output moving toward that AP. This is
consistent with RSSI and freshness/state both contributing to the private
solver.

The empirical RSSI/accuracy weighting therefore models the observable horizontal
solve well but should not be treated as an exact reconstruction. The private
logs also expose band/stage labels such as `2.4GHz`, `stage1+5GHz`, and
`placebad`; their exact proprietary meanings remain unresolved.


## Decoded `tilesals` source tuple

The private WifiPosition tuple labeled `tilesals` is now trace-decoded with high
confidence. Across every checked occurrence, its four integer fields are the
independently truncated percentages of the current scan classified as:

1. ALS-located
2. tile-located
3. unknown (a returned record exists but has no usable Location)
4. not-in-db (no returned record exists)

Examples from the capture include 20/27 ALS + 7/27 unknown -> `74 | 0 | 25 | 0`,
21/31 ALS + 7/31 unknown + 3/31 not-in-db -> `67 | 0 | 22 | 9`, and
25/37 ALS + 7/37 unknown + 5/37 not-in-db -> `67 | 0 | 18 | 13`.
The percentages use integer truncation rather than rounding and therefore need
not sum to exactly 100.

The lab lifecycle model exposes these four fields through
`TraceSourcePercentages`. The independent tile-location source is currently
modeled as zero because no controlled tile source has been added to the lab.
