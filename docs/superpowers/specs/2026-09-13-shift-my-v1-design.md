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
- a follow-on standard profile;
- a user action that cycles Location Services after changing the selected location.

For v1, the ACME/device-attestation layer is intentionally deferred. It is not required to answer the first feasibility question: can a clean-room selective-DNS + trusted-CA path produce a changed system location on our own stock iPhone?

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
- structured request/response logging with secrets and precise user coordinates omitted from normal logs.

This single-host architecture is preferred because it is cheapest, simplest to debug, and keeps all packet-flow components under one administrative boundary.

## 6. Repository structure

Planned top-level layout:

```text
cmd/
  server/             main process
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
  nginx-or-caddy/
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
last_seen_at
```

The token is not treated as a long-term authentication system. It is only enough to keep the v1 experiment isolated from random public traffic.

## 8. iPhone profile

The generated configuration profile contains only the minimum payloads required for the experiment.

### Root CA payload

A project-owned test root CA is installed on the test iPhone so the proxy can present certificates for the narrow Apple location host set during the experiment.

The CA private key never ships in the profile and never leaves the server.

### Managed DNS payload

Configure DNS-over-HTTPS only for the narrow Apple location-related domains identified during the clean-room analysis.

All unrelated DNS continues through the user's normal resolver.

The DoH URL contains the installation token so the backend can associate DNS activity with the current selected coordinates.

### Removal

The profile must remain removable by the user.

The project must also include a clear uninstall/reset procedure that removes the profile and restores normal DNS behavior.

## 9. Location-change flow

The intended user flow is:

1. Open the Shift-My dashboard.
2. Download the generated configuration profile.
3. Install the profile through iPhone Settings.
4. Confirm the dashboard reports recent device/DNS activity.
5. Search for or enter a target location.
6. Press `Set Location`.
7. Backend commits the new coordinates and increments a location revision number.
8. UI displays an explicit refresh instruction:
   - Settings -> Privacy & Security -> Location Services;
   - turn Location Services off;
   - wait briefly;
   - turn Location Services back on.
9. Open Apple Maps and observe the blue dot.
10. If Apple Maps succeeds, validate a second Location Services app.
11. Finally test Find My.

The Location Services off/on cycle is a first-class part of the v1 test because the observed commercial service used a similar refresh step after setting a new target location.

## 10. Dashboard

The dashboard stays intentionally small.

Main screen:

```text
SHIFT-MY TEST

Device status: Connected / Not seen
Profile status: Generated
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

No account system, billing system, social features, maps history, or multi-device management are included in v1.

## 11. DNS behavior

The DoH service has two jobs:

1. answer the selected Apple location hostnames with the experiment server address;
2. forward unrelated eligible DNS queries normally if any reach the endpoint.

The service must not become an open public resolver.

Requests are accepted only when the path includes a valid installation token.

## 12. TLS proxy behavior

The proxy listens for connections intended for only the explicit Apple location host allowlist used by the experiment.

For those hosts it:

1. terminates the client TLS session using a leaf certificate signed by the project test CA;
2. creates a separate TLS connection to the real Apple origin;
3. forwards traffic unchanged by default;
4. records protocol metadata required for reverse engineering on our own device;
5. enables a narrowly scoped transform only after the relevant location request/response format is identified.

The proxy is not a general-purpose interception proxy.

All other hostnames are rejected or bypassed.

## 13. Protocol discovery and transformation

The first server-side milestone is not to guess Apple's payload format. It is to observe the real request and response generated by our own test device after the profile is installed.

The experiment proceeds in checkpoints:

### Checkpoint A - DNS routing

Confirm the iPhone sends the target hostnames through our DoH service.

### Checkpoint B - trusted TLS

Confirm the target iPhone accepts the project-issued leaf certificate for the target system request and the request reaches the proxy.

If this fails, record the exact trust failure before changing architecture.

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

## 14. Unknowns and failure handling

The largest unknown is whether current iOS system location traffic accepts a user/profile-installed test root for this path and whether the Apple response includes integrity protections that prevent modification.

The architecture intentionally makes that unknown measurable.

If TLS fails:

- capture the exact failure condition;
- compare the working commercial profile behavior on our own device;
- determine whether the missing piece is profile trust, client identity, endpoint selection, or another documented configuration mechanism.

If transparent proxying succeeds but transformed responses are rejected:

- compare unchanged vs transformed framing;
- look for signatures, checksums, sequence fields, or request-bound values;
- do not broaden interception to unrelated Apple services.

If Apple Maps succeeds but Find My does not:

- treat Find My as a separate compatibility target rather than declaring the entire experiment failed;
- identify whether it uses a different source, cache, process, or trust path.

## 15. Security boundaries

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

## 16. Deployment

The deployment should be reproducible from the repository.

Expected deployment shape:

- Ubuntu VM;
- systemd service for Shift-My;
- firewall allowing only required ports;
- TLS certificate for the public dashboard/DoH hostname;
- SQLite database under a dedicated application directory;
- environment file containing secrets and CA paths;
- one bootstrap script for a fresh VM.

The public dashboard certificate is separate from the project test CA used for the narrow experiment proxy.

## 17. Testing strategy

### Unit tests

- profile generation;
- token validation;
- target-host allowlist;
- coordinate storage;
- DNS response construction;
- protocol parser/serializer once discovered;
- kill-switch behavior.

### Integration tests

- DoH request -> selected IP response;
- TLS proxy -> upstream forwarding;
- profile generated with correct token and endpoint;
- location update increments revision and is visible to proxy.

### Device test sequence

1. Install profile.
2. Verify DoH traffic.
3. Verify trusted TLS path.
4. Verify unchanged upstream forwarding.
5. Set a clearly distant target location.
6. Cycle Location Services off/on.
7. Open Apple Maps.
8. Record pass/fail and diagnostic stage.
9. Test a second app if Maps passes.
10. Test Find My last.

## 18. Milestones

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
- uninstall/reset documentation.

### M3 - transparent proxy

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

## 19. v1 definition of done

v1 is considered technically complete when all of the following exist:

- reproducible deployment on the free VM;
- installable/removable profile;
- tokenized selective DoH routing;
- narrow trusted TLS proxy for the test device;
- working dashboard target selector;
- explicit Location Services off/on refresh flow;
- end-to-end Apple Maps test result;
- diagnostics that clearly identify the failed checkpoint if the coordinate override does not work;
- uninstall/kill-switch path that restores normal behavior.

The aspirational success condition remains system-wide propagation to Apple Maps, ordinary Location Services apps, and Find My.

## 20. Deferred work

Do not build these until the core mechanism is proven:

- ACME device attestation;
- hardware-bound client identity;
- multiple users;
- multiple devices;
- subscriptions or billing;
- paid domain;
- native iOS app;
- polished map UI;
- location history;
- high-availability infrastructure.

Those features add complexity without answering the first technical question.