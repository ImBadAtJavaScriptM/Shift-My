# Controlled-lab safety boundary

Shift-My is intentionally limited to infrastructure and hostnames owned by the operator.

## Enforced hostname boundary

The only TLS/DNS lab names are derived from the configured public hostname:

- `loc-a.<public-host>`
- `loc-b.<public-host>`
- `device-loc.<public-host>`

The hostname policy rejects `apple.com`, `*.apple.com`, `icloud.com`, and `*.icloud.com` as a base. The profile generator also requires every managed DNS match domain to be a subdomain of the configured project-owned public host.

The DoH handler refuses names outside the controlled allowlist rather than forwarding them. The lab TLS service refuses unknown SNI/Host values. Nginx has exact SNI routes for the public hostname and the three controlled lab names; all other SNI is sent to a dead backend.

CI includes a regression check that fails if known Apple production location-service hostnames appear in active implementation paths.

## Access control

The dashboard, Stage 1 profile download, static UI, location API, and enrollment-reset API require HTTP Basic authentication.

Automatic iOS services cannot answer the dashboard's Basic Auth challenge, so they use separate credentials:

- DoH uses a high-entropy per-installation path token.
- Stage 2 uses a separate high-entropy query token.
- ACME uses signed JWS requests, single-use replay nonces, the current installation ClientIdentifier, and managed-device attestation.

The safe status API never returns those credentials.

## Certificate separation

The deployment uses two independent private CAs:

- **Lab TLS CA:** signs only controlled lab server certificates. Its root certificate is delivered in Stage 2.
- **Identity CA:** signs only short-lived client-auth device certificates after ACME attestation and CSR-key binding. Its root is not delivered as a trusted server root.

Neither private key is embedded in an iPhone profile.

The public dashboard/DoH/ACME hostname uses a normal publicly trusted certificate.

## Apple managed-device attestation

The ACME service trusts the Apple Enterprise Attestation Root CA only for validating an enrolling iPhone's managed-device attestation.

That trust anchor is not used to generate certificates, route Apple traffic, or impersonate an Apple service.

Before identity issuance, the server requires the attestation path and challenge freshness to validate. At finalization, the CSR key must match the public key represented by the accepted attestation.

## Reset behavior

**Reset enrollment** rotates the DoH credential, Stage 2 credential, and ClientIdentifier; removes ACME account/order/nonce state; and clears enrollment/traffic status markers.

It deliberately preserves the selected latitude, longitude, label, and location revision.

## Capture

Diagnostic capture is off by default. When enabled, it applies only to the controlled lab endpoint, redacts `Authorization`, `Cookie`, `Set-Cookie`, and `Proxy-Authorization`, and stores at most 2 MiB of each body.

## Non-goals

This repository does not provide production Apple interception, Apple service impersonation, Find My modification, Core Location bypasses, third-party traffic interception, or instructions for defeating certificate pinning, attestation, or platform trust protections.
