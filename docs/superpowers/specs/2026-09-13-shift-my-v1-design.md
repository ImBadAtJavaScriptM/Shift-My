# Shift-My v1 Design Spec

Date: 2026-09-13

## 1. Goal

Build a clean-room, stock-iPhone location experiment inspired by the behavior we observed from ShiftMy, without depending on ShiftMy's backend or proprietary code.

The v1 target is system-wide behavior on a normal iPhone with no jailbreak and no Mac/Xcode tether after setup.

Primary success target: a user-selected location should be reflected by:

1. Apple Maps
2. Another ordinary app that reads iOS Location Services
3. Find My

Apple Maps is the first validation surface because it gives the clearest immediate signal that the iPhone location stack changed.

## 2. User constraints

- Stock iPhone.
- No jailbreak.
- No persistent Mac/Xcode connection after setup.
- One-time profile installation is acceptable.
- Total infrastructure budget must remain at or below $15/year.
- v1 should attempt actual location changing immediately rather than shipping only a control-panel prototype.
- The flow should resemble the observed ShiftMy experience:
  1. install a configuration profile in Settings;
  2. select a target location;
  3. cycle the iPhone Location Services master toggle off and back on to force a fresh location lookup;
  4. verify whether the selected location propagates.

The remembered Location Services toggle is treated as a working hypothesis. If testing shows the original product used a different toggle, the workflow will be updated.

## 3. Clean-room boundary

The project may use behavior we directly observed on our own device and configuration-profile metadata we recovered, but it will not copy ShiftMy server code, private keys, private credentials, or proprietary backend assets.

The v1 implementation will operate only on infrastructure and certificates we control.

## 4. Relevant observed behavior

The recovered ShiftMy profile gave us several architectural clues:

- selective DNS-over-HTTPS rather than replacing all device DNS;
- routing focused on Apple location-related hostnames;
- a profile-installed root CA;
- a per-enrollment identifier embedded in a DoH URL;
- an ACME-based hardware-bound device identity in the production service;
- a declarative bootstrap that referenced a follow-on standard profile;
- a user action that cycles Location Services after changing the selected location.

For v1, the ACME/device-attestation layer is intentionally deferred. It is not required to answer the first feasibility question: can a clean-room selective-DNS + trusted-CA path produce a changed system location on our own stock iPhone?

The recovered declarative bootstrap pattern remains an explicit fallback if current iOS will not activate the minimal manually installed DNS payload we try first.

## 5. Architecture decision

### Recommended architecture

Use one free Ubuntu VM with one lightweight application stack.

Initial hosting target:

- Oracle Cloud Always Free VM if available to the user;
- free dynamic DNS hostname for the prototype;
- no paid domain required for v1.

Application stack:

- Go backend;
- SQLite state store;
- HTTPS dashboard/API;
- configuration-profile generator;
- DNS-over-HTTPS endpoint;
- narrow TLS reverse-proxy/inspection component for the experiment;
- SNI edge router so multiple TLS roles can share one public IPv4 address and port 443;
- structured request/response logging with secrets and precise user coordinates omitted from normal logs.

This single-host architecture is preferred because it is cheapest, simplest to debug, and keeps all packet-flow components under one administrative boundary.

## 6. Repository structure

Planned top-level layout:

```text
cmd/
  server/             main application process
  edge/               SNI routing entry point if kept separate
internal/
  config/             environment and runtime config
  dashboard/          web UI and API handlers
  profile/            mobileconfig generation
  doh/                DNS-over-HTTPS handling
  proxy/              narrow TLS reverse proxy
  location/           selected-coordinate state and transforms
  storage/            SQLite access
  observability/      structured logs and status events
web/
  static/
  templates/
deploy/
  systemd/
  edge/
  scripts/
docs/
  superpowers/specs/
  testing/
```

Each package should expose a small interface and remain independently testable.

## 7. v1 device model

v1 is intentionally single-user and single-device.

There is no account system and no full multi-device enrollment service yet.

The profile generator creates one random installation token. That token is embedded in the DoH endpoint path and stored in SQLite with the selected target coordinates.

Example conceptual record:

```text
installation_id
profile_token
selected_latitude
selected_longitude
selected_label
location_revision
last_seen_at
```

The token is not treated as a long-term authentication system. It is only enough to keep the v1 experiment isolated from random public traffic.

## 8. iPhone profile

The generated configuration profile contains only the minimum payloads required for the experiment.

### Root CA payload

A project-owned test root CA is installed on the test iPhone so the proxy can present certificates for the narrow Apple location host set during the experiment.

The CA private key never ships in the profile and never leaves the server.

For a manually downloaded profile, current iOS may require the user to additionally enable SSL/TLS trust under:

`Settings -> General -> About -> Certificate Trust Settings`

Therefore the prototype setup UI must detect this as a separate checkpoint rather than assuming profile installation alone establishes TLS trust.

If testing demonstrates that the clean-room declarative installation path can establish the required trust without this manual step, the extra prompt can later be removed.

### Managed DNS payload

Configure DNS-over-HTTPS only for the narrow Apple location-related domains identified during the clean-room analysis.

All unrelated DNS continues through the user's normal resolver.

The DoH URL contains the installation token so the backend can associate DNS activity with the current selected coordinates.

### DNS activation fallback

Because current Apple deployment documentation treats the managed DNS payload primarily as a device-management configuration, v1 must not assume a manually downloaded minimal profile will activate it on every stock-iPhone configuration.

The test order is:

1. try the smallest direct profile matching the relevant payload shape already observed on our test device;
2. verify actual DoH traffic server-side;
3. if the payload installs but no traffic appears, reproduce the clean-room declarative bootstrap/follow-on-profile pattern observed in the ShiftMy configuration rather than adding unrelated MDM infrastructure;
4. keep ACME/device attestation deferred unless evidence shows it is required for this activation path.

### Removal

The profile must remain removable by the user.

The project must also include a clear uninstall/reset procedure that removes the profile, removes any test root trust, and restores normal DNS behavior.

## 9. Location-change flow

The intended user flow is:

1. Open the Shift-My dashboard.
2. Download the generated configuration profile.
3. Install the profile through iPhone Settings.
4. If required by the prototype trust path, enable full trust for the Shift-My test root certificate.
5. Confirm the dashboard reports recent device/DNS activity.
6. Search for or enter a target location.
7. Press `Set Location`.
8. Backend commits the new coordinates and increments a location revision number.
9. UI displays an explicit refresh instruction:
   - Settings -> Privacy & Security -> Location Services;
   - turn Location Services off;
   - wait briefly;
   - turn Location Services back on.
10. Open Apple Maps and observe the blue dot.
11. If Apple Maps succeeds, validate a second Location Services app.
12. Finally test Find My.

The Location Services off/on cycle is a first-class part of the v1 test because the observed commercial service used a similar refresh step after setting a new target location.

## 10. Dashboard

The dashboard stays intentionally small.

Main screen:

```text
SHIFT-MY TEST

Device status: Connected / Not seen
Profile status: Generated / Traffic seen
Certificate trust: Unknown / Confirmed by successful TLS
Target location: Burbank, CA
Coordinates: 34.x, -118.x

[ Search location ]
[ Set Location ]

Experiment status
- DoH traffic seen
- TLS proxy traffic seen
- Apple location request recognized
- response transform applied
- Apple Maps verification pending / passed / failed

Next step
Cycle Location Services off -> on, then open Apple Maps.
```

`Profile status` is inferred from traffic. The server cannot claim that an unsupervised device installed the profile merely because the file was downloaded.

No account system, billing system, social features, maps history, or multi-device management are included in v1.

## 11. DNS behavior

The DoH service has two jobs:

1. answer the selected Apple location hostnames with the experiment server address;
2. forward unrelated eligible DNS queries normally if any reach the endpoint.

The service must not become an open public resolver.

Requests are accepted only when the path includes a valid installation token.

The generated profile should include the VM address as a server address when appropriate so the DoH hostname does not depend on the very resolver being configured.

## 12. Port 443 and SNI routing

One public IPv4 address must serve two different TLS roles:

1. the normal public hostname used by the dashboard and DoH endpoint;
2. the allowlisted Apple location hostnames routed to the experiment proxy.

An edge listener on TCP/443 inspects SNI without decrypting traffic and dispatches connections:

- project hostname -> public HTTPS application listener;
- explicit Apple location hostname allowlist -> experiment proxy listener;
- everything else -> reject.

This avoids requiring multiple public IP addresses and keeps the $0 prototype architecture practical.

The first prototype does not advertise or intentionally support QUIC/HTTP3 on UDP/443. If the target Apple service attempts QUIC, the experiment records that behavior and relies on normal TCP fallback where available before adding any UDP complexity.

## 13. TLS proxy behavior

The proxy listens for connections intended for only the explicit Apple location host allowlist used by the experiment.

For those hosts it:

1. terminates the client TLS session using a leaf certificate signed by the project test CA;
2. creates a separate TLS connection to the real Apple origin;
3. forwards traffic unchanged by default;
4. records protocol metadata required for reverse engineering on our own device;
5. enables a narrowly scoped transform only after the relevant location request/response format is identified.

The proxy is not a general-purpose interception proxy.

All other hostnames are rejected.

## 14. Protocol discovery and transformation

The first server-side milestone is not to guess Apple's payload format. It is to observe the real request and response generated by our own test device after the profile is installed.

The experiment proceeds in checkpoints:

### Checkpoint A - DNS routing

Confirm the iPhone sends the target hostnames through our DoH service.

### Checkpoint B - trusted TLS

Confirm the target iPhone accepts the project-issued leaf certificate for the target system request and the request reaches the proxy.

If this fails, distinguish among:

- root not fully trusted;
- certificate/SAN generation error;
- certificate pinning or a restricted trust policy;
- request not using the expected hostname/path.

Record the exact failure before changing architecture.

### Checkpoint C - transparent forwarding

Forward the request to Apple unchanged and verify normal location behavior still works.

This proves the proxy is not breaking the service before any transformation is attempted.

### Checkpoint D - protocol recognition

Identify the minimum fields in the Apple location request/response that correspond to the returned location estimate.

Binary payloads must be parsed structurally rather than edited using brittle byte offsets.

### Checkpoint E - coordinate transformation

Replace only the relevant returned location fields with the dashboard-selected latitude and longitude, preserving all unrelated fields and framing.

### Checkpoint F - location refresh

After changing the selected coordinates, cycle Location Services off and back on and verify Apple Maps.

## 15. Unknowns and failure handling

The largest unknowns are:

- whether current iOS activates the selective managed-DNS payload through our minimal stock-iPhone installation path;
- whether the system location traffic accepts the project test root after full trust is enabled;
- whether the Apple response includes integrity protections that prevent modification;
- whether Find My uses the same location source/path as Apple Maps.

The architecture intentionally makes each unknown measurable.

If DNS activation fails:

- verify the payload is present;
- reproduce the recovered declarative bootstrap/follow-on-profile pattern;
- introduce ACME/device identity only if the evidence says activation depends on it.

If TLS fails:

- capture the exact failure condition;
- compare the working commercial profile behavior on our own device;
- determine whether the missing piece is profile trust, client identity, endpoint selection, or another configuration mechanism.

If transparent proxying succeeds but transformed responses are rejected:

- compare unchanged vs transformed framing;
- look for signatures, checksums, sequence fields, or request-bound values;
- do not broaden interception to unrelated Apple services.

If Apple Maps succeeds but Find My does not:

- treat Find My as a separate compatibility target rather than declaring the entire experiment failed;
- identify whether it uses a different source, cache, process, or trust path.

## 16. Security boundaries

- Test only devices the user owns or controls.
- Do not reuse ShiftMy certificates, secrets, tokens, or private infrastructure.
- Root CA private key stays server-side.
- Keep the target-domain allowlist explicit.
- Do not operate an open DNS resolver or general public interception proxy.
- Profile is removable.
- Provide a kill switch that disables interception and causes target DNS to resolve normally.
- Store only data required for the test.
- Do not store a history of real physical locations.
- Selected target coordinates may be deleted/reset from the dashboard.
- Diagnostic body capture is disabled by default and, when temporarily enabled for protocol discovery on the user's own device, should have size limits and an explicit deletion path.

## 17. Deployment

The deployment should be reproducible from the repository.

Expected deployment shape:

- Ubuntu VM;
- systemd services for Shift-My components;
- firewall allowing only required ports;
- SNI edge listener on TCP/443;
- public TLS certificate for the dashboard/DoH hostname;
- separate project test CA used only for the narrow experiment proxy;
- SQLite database under a dedicated application directory;
- environment file containing secrets and CA paths;
- one bootstrap script for a fresh VM.

The public dashboard certificate is separate from the project test CA used for the narrow experiment proxy.

## 18. Testing strategy

### Unit tests

- profile generation;
- token validation;
- target-host allowlist;
- coordinate storage;
- DNS response construction;
- SNI route selection;
- protocol parser/serializer once discovered;
- kill-switch behavior.

### Integration tests

- DoH request -> selected IP response;
- SNI project hostname -> public app;
- SNI allowlisted Apple hostname -> experiment proxy;
- TLS proxy -> upstream forwarding;
- profile generated with correct token and endpoint;
- location update increments revision and is visible to proxy.

### Device test sequence

1. Install profile.
2. Enable test-root trust if required.
3. Verify DoH traffic.
4. Verify trusted TLS path.
5. Verify unchanged upstream forwarding.
6. Set a clearly distant target location.
7. Cycle Location Services off/on.
8. Open Apple Maps.
9. Record pass/fail and diagnostic stage.
10. Test a second app if Maps passes.
11. Test Find My last.

## 19. Milestones

### M1 - repository and local skeleton

- Go project;
- config package;
- SQLite;
- dashboard health page;
- tests and CI.

### M2 - profile and DoH

- project test CA generation workflow;
- profile generator;
- tokenized DoH endpoint;
- target-domain routing;
- direct-profile activation test;
- declarative bootstrap fallback if required;
- uninstall/reset documentation.

### M3 - edge and transparent proxy

- TCP/443 SNI router;
- allowlisted TLS proxy;
- dynamic leaf certificates from project CA;
- clean forwarding to Apple origin;
- protocol-safe request/response capture for our own device.

### M4 - protocol discovery

- identify the actual location exchange;
- build parser and serializer;
- prove unchanged round-trip parity.

### M5 - coordinate override attempt

- dashboard target coordinates feed the transform;
- refresh instruction shown;
- Apple Maps test.

### M6 - compatibility

- ordinary third-party Location Services app;
- Find My;
- document which surfaces pass and which remain unsupported.

## 20. v1 definition of done

v1 is considered technically complete when all of the following exist:

- reproducible deployment on the free VM;
- installable/removable profile;
- tokenized selective DoH routing or a clearly diagnosed iOS activation barrier;
- narrow trusted TLS proxy for the test device or a clearly diagnosed trust-policy barrier;
- working dashboard target selector;
- explicit Location Services off/on refresh flow;
- end-to-end Apple Maps test result;
- diagnostics that clearly identify the failed checkpoint if the coordinate override does not work;
- uninstall/kill-switch path that restores normal behavior.

The aspirational success condition remains system-wide propagation to Apple Maps, ordinary Location Services apps, and Find My.

## 21. Deferred work

Do not build these until the core mechanism is proven:

- ACME device attestation unless M2 evidence makes it necessary;
- hardware-bound client identity unless M2 evidence makes it necessary;
- multiple users;
- multiple devices;
- subscriptions or billing;
- paid domain;
- native iOS app;
- polished map UI;
- location history;
- high-availability infrastructure.

Those features add complexity without answering the first technical question.