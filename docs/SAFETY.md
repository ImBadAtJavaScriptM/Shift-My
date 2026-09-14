# Controlled-lab safety boundary

Shift-My v1 is intentionally limited to infrastructure and hostnames owned by the operator.

## Enforced boundary

The only TLS/DNS lab names are derived from the configured public hostname:

- `loc-a.<public-host>`
- `loc-b.<public-host>`
- `device-loc.<public-host>`

The hostname policy rejects `apple.com`, `*.apple.com`, `icloud.com`, and `*.icloud.com` as a base. The DoH handler refuses names outside the controlled allowlist instead of forwarding them. The lab TLS service refuses unknown SNI/Host values. Nginx has exact SNI routes for the public hostname and the three controlled lab names; all other SNI is sent to a dead backend.

## Certificates

The dashboard/DoH hostname uses a normal publicly trusted certificate. The three lab hosts use short-lived leaf certificates signed by the project test root. The project CA private key remains server-side and is never embedded in the configuration profile.

## Capture

Diagnostic capture is off by default. When enabled, it applies only to the controlled lab endpoint, redacts `Authorization`, `Cookie`, `Set-Cookie`, and `Proxy-Authorization`, and stores at most 2 MiB of each body.

## Non-goals

This repository does not provide production Apple interception, Find My modification, Core Location bypasses, third-party traffic interception, or instructions for defeating certificate pinning, attestation, or platform protections.
