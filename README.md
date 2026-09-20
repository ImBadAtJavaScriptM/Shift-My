# Shift-My

Shift-My is a controlled stock-iPhone networking lab for testing configuration profiles, hardware-bound ACME device identity, declarative profile delivery, tokenized DNS-over-HTTPS, project-owned TLS endpoints, and selected test coordinates.

## What the current lab does

- Serves a password-protected dark dashboard for choosing a test latitude/longitude and downloading a removable Stage 1 iPhone configuration profile.
- Stage 1 contains:
  - declarative configuration that activates a Stage 2 profile;
  - a hardware-bound, non-extractable P-256 ACME identity request with managed-device attestation enabled;
  - managed DoH scoped to exactly three project-owned hostnames.
- Runs a single-device ACME service under `/acme/device/`. It accepts one `permanent-identifier`, requires a `device-attest-01` response, validates the Apple managed-device attestation chain and challenge freshness, binds the CSR key to the attested hardware key, and issues a short-lived client-auth-only identity certificate from a separate identity CA.
- Stage 2 installs the project test root CA and the same controlled DoH configuration. Its URL is protected by an independent high-entropy installation token.
- Runs a tokenized DoH endpoint that resolves only:
  - `loc-a.<public-host>`
  - `loc-b.<public-host>`
  - `device-loc.<public-host>`
- Runs a controlled TLS endpoint at `/v1/location` that returns the selected coordinate and revision.
- Persists the selected target and enrollment state in SQLite so server restarts do not discard the installation.
- Provides an authenticated **Reset enrollment** action that rotates profile/ACME credentials and clears enrollment lifecycle state while preserving the saved target coordinates and revision.
- Supports optional diagnostic capture for the controlled lab endpoint only. Capture is off by default, redacts credential headers, and caps bodies at 2 MiB.

## Safety boundary

This repository does **not** route, intercept, impersonate, or alter Apple production services, Apple Maps, Find My, Core Location, or third-party production location services.

The active profile generator accepts only subdomains of the configured project-owned public hostname. Apple and iCloud production domains are explicitly rejected and CI contains a regression guard against adding known Apple location-service hostnames to active implementation paths.

The Apple Enterprise Attestation Root is used only to verify managed-device attestation presented by the enrolling iPhone. It is not used to impersonate Apple services.

## Development

Requires Go 1.22+.

```bash
go test ./...
go vet ./...
```

## Deployment

Set a hostname to the VM's public IP, then run:

```bash
sudo ./deploy/scripts/bootstrap.sh lab.example.com 2001:db8::10 you@example.com
```

`SHIFT_MY_PUBLIC_IP` accepts IPv4 or IPv6. The bootstrap script creates two separate private CAs:

- **Lab TLS CA** — signs only the project's controlled lab TLS endpoints and is installed by Stage 2.
- **Identity CA** — signs only short-lived client-auth device identity certificates and is never installed as a trusted TLS root on the iPhone.

The script also obtains the normal publicly trusted certificate for the public dashboard/DoH/ACME hostname, generates the dashboard password when one does not already exist, and preserves that password on subsequent runs.

For Google Cloud IPv6 deployment details, see [`docs/testing/google-cloud.md`](docs/testing/google-cloud.md). For the iPhone test sequence, see [`docs/testing/device-setup.md`](docs/testing/device-setup.md). For removal, see [`docs/testing/uninstall.md`](docs/testing/uninstall.md).

See [`docs/SAFETY.md`](docs/SAFETY.md) for the enforced boundary and [`docs/protocol/README.md`](docs/protocol/README.md) for the wire behavior.
