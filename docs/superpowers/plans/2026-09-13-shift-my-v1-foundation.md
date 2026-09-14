# Shift-My v1 Controlled-Lab Implementation Plan

> This plan supersedes the earlier production-interception draft. The implementation is constrained to project-owned hostnames and does not target Apple, Find My, Core Location, or third-party production services.

**Goal:** Ship and verify the complete stock-iPhone profile → tokenized DoH → controlled TLS lab path, with a dashboard-selected test coordinate, removable trust, safe capture, and dedicated-VPS deployment files.

**Spec:** `docs/superpowers/specs/2026-09-13-shift-my-v1-design.md`

## Constraints

- [x] Stock iPhone; no jailbreak required for the lab validation.
- [x] One removable profile.
- [x] Project-owned root CA; private key stays server-side.
- [x] Exactly three controlled lab subdomains derived from the configured public hostname.
- [x] Reject Apple/iCloud production bases in the hostname policy.
- [x] No open resolver and no generic interception proxy.
- [x] Capture disabled by default, credential headers redacted, body cap 2 MiB.
- [x] Automated tests and `go vet` in GitHub Actions.

## Task 1 — Foundation and state

- [x] Go module and CI.
- [x] Environment configuration.
- [x] SQLite installation state with profile token, selected coordinate, revision, and traffic markers.
- [x] Unit tests for config and storage.

## Task 2 — Dashboard and coordinate state

- [x] Target-coordinate service with coordinate validation and revision increment.
- [x] Safe status API that never returns the profile token.
- [x] Dark dashboard and static assets.
- [x] Tests for API and target behavior.

## Task 3 — Project CA and removable iPhone profile

- [x] Root-CA bootstrap command.
- [x] Short-lived single-host leaf certificate minting.
- [x] `.mobileconfig` generator with project root + managed DoH.
- [x] `SupplementalMatchDomains` limited to controlled lab subdomains.
- [x] Production-domain rejection tests.

## Task 4 — Tokenized controlled DoH

- [x] POST and GET RFC 8484 handling.
- [x] Validate profile token before DNS parsing.
- [x] A answers return the configured lab IPv4.
- [x] AAAA returns NODATA for allowed lab names.
- [x] Unknown names return REFUSED and are never forwarded.
- [x] DoH activity marker in SQLite.

## Task 5 — Controlled TLS lab service

- [x] Reject SNI/Host outside the controlled hostname policy.
- [x] Mint one-SAN lab certificates from the project CA.
- [x] `GET /v1/location` returns the current latitude, longitude, label, and revision.
- [x] Lab TLS activity marker in SQLite.
- [x] Tests for certificate and HTTP behavior.

## Task 6 — Opt-in diagnostic capture

- [x] Disabled recorder produces no files.
- [x] Redact `Authorization`, `Cookie`, `Set-Cookie`, and `Proxy-Authorization`.
- [x] Cap body capture at 2 MiB.
- [x] Timestamp/host/direction/random-suffix filenames; no coordinates in filenames.
- [x] Wire capture only to the controlled TLS endpoint.

## Task 7 — TLS listeners and dedicated-VPS deployment

- [x] Public HTTP handler rejects unexpected Host values.
- [x] Public Go TLS listener on `127.0.0.1:8443` using a publicly trusted certificate.
- [x] Controlled lab TLS listener on `127.0.0.1:9443` using dynamic project-CA leaves.
- [x] Exact-SNI Nginx routing for the public host + three lab subdomains only.
- [x] systemd unit and environment template.
- [x] VPS bootstrap script with production Apple/iCloud hostname rejection.

## Task 8 — Device testing and cleanup documentation

- [x] Dashboard links directly to the generated profile and controlled lab endpoint.
- [x] Device setup walkthrough.
- [x] Uninstall/normal-network restoration walkthrough.
- [x] Protocol documentation.
- [x] Safety boundary documentation.
- [x] README with development and deployment entry points.

## Final verification and merge

- [ ] Add shell syntax validation to CI.
- [ ] Run final `go test ./...` and `go vet ./...` on the feature head.
- [ ] Compare `main...feat/v1-foundation` and review all changed files.
- [ ] Confirm no private keys, generated certs, database files, captures, or secrets are committed.
- [ ] Confirm production Apple/iCloud names occur only in rejection/safety documentation or negative tests—not active routes.
- [ ] Open a PR to `main`, verify PR checks, merge, and verify `main` checks.

## Completion boundary

The merged v1 is a complete **controlled networking lab**, not a production iOS location override. Any future system-location simulation work requires a separate design/review using a legitimate device-testing mechanism.
