# Shift-My v1 Controlled-Lab Design Spec

Date: 2026-09-13

## Goal

Build a clean-room stock-iPhone experiment that reproduces the observable configuration-profile, selective DoH, dashboard, and project-owned TLS plumbing we learned from earlier research, while keeping every active network endpoint under the operator's control.

The v1 feasibility question is: **can a manually installed iPhone profile route only project-owned lab names through our tokenized DoH service and complete trusted TLS to a controlled endpoint that reflects a user-selected test coordinate?**

## Explicit non-goals

v1 does not intercept or modify Apple Maps, Find My, Core Location, Apple production services, or third-party location services. It does not defeat certificate pinning, device attestation, authentication, or other platform protections. A successful v1 test proves the networking/profile path only; it is not a system-location override.

## Constraints

- Stock iPhone; no jailbreak.
- No persistent Mac/Xcode tether after setup.
- One removable configuration profile is acceptable.
- Single-user/single-device prototype.
- Infrastructure and certificates are owned by the operator.
- No ShiftMy private server code, credentials, keys, certificates, or infrastructure.
- Capture is opt-in and disabled by default.
- The project CA private key never leaves the server.

## Controlled hostname policy

Given public host `lab.example.com`, the only managed/test names are:

- `loc-a.lab.example.com`
- `loc-b.lab.example.com`
- `device-loc.lab.example.com`

The hostname policy rejects Apple/iCloud production bases. DoH refuses any name outside the allowlist. The lab TLS server rejects unknown SNI and HTTP Host values. Nginx exact-SNI routing sends unknown names to a dead backend.

## Architecture

One Go service provides:

1. SQLite state for the profile token, selected coordinate, revision, and traffic markers.
2. A dark dashboard/API for selecting the test coordinate and checking status.
3. A removable `.mobileconfig` containing the project test root and managed DoH payload scoped only to the three controlled names.
4. Tokenized RFC 8484 DoH that returns the configured server IPv4 for allowed A queries, NODATA for allowed AAAA queries, and REFUSED otherwise.
5. A controlled TLS endpoint that dynamically mints a short-lived, single-SAN certificate from the project test root and serves `GET /v1/location`.
6. Optional capture for the controlled TLS endpoint only, with credential-header redaction and a 2 MiB body cap.

On a dedicated VPS, Nginx uses TLS SNI preread on public TCP/443 and routes:

- the public hostname → Go public TLS listener on `127.0.0.1:8443`;
- the three controlled lab names → Go lab TLS listener on `127.0.0.1:9443`;
- everything else → a dead backend.

The public hostname uses a normal publicly trusted certificate. Controlled lab leaf certificates use the private project test CA included in the removable profile.

## User flow

1. Deploy the service to a hostname/IP owned by the operator.
2. Open the dashboard and choose a test coordinate.
3. Download and install the removable iPhone profile.
4. If iOS requires it, explicitly trust only the Shift-My test root installed by that profile.
5. Confirm the dashboard sees an authenticated DoH request.
6. Open `https://device-loc.<public-host>/v1/location` on the iPhone.
7. Confirm the returned coordinate/label/revision matches the dashboard and that Lab TLS changes to `seen`.
8. Remove the profile and test trust when finished.

## Security and privacy

- Profile token is stored server-side and embedded only in the DoH URL; dashboard status never returns it.
- Coordinate history is not logged by default; SQLite keeps only the current selected target and revision.
- Diagnostic capture is off by default.
- When capture is enabled, `Authorization`, `Cookie`, `Set-Cookie`, and `Proxy-Authorization` are redacted and body capture is capped at 2 MiB.
- No generic recursive DNS resolver or generic TLS proxy exists.

## v1 success criteria

v1 is successful when automated tests pass and an operator can demonstrate on their own iPhone that:

- the profile installs and is removable;
- DoH activity reaches the tokenized endpoint;
- only the three controlled lab hostnames resolve through it;
- the controlled TLS endpoint presents a certificate from the project root and returns the current selected target;
- removing the profile restores the phone to its normal DNS/trust state.

Any future work on actual system-location simulation must use a separately reviewed, legitimate device-testing mechanism rather than production-service interception.
