# Shift-My

Shift-My is a controlled stock-iPhone networking lab for testing configuration profiles, tokenized DNS-over-HTTPS, project-owned TLS endpoints, and selected test coordinates.

## What v1 does

- Serves a password-protected dark dashboard for choosing a test latitude/longitude and downloading a removable iPhone configuration profile.
- Generates a project-owned test root CA and profile scoped to exactly three project-owned lab hostnames: `loc-a.<public-host>`, `loc-b.<public-host>`, and `device-loc.<public-host>`.
- Runs a tokenized DoH endpoint that resolves only those three names to the configured lab server IP and refuses other DNS names.
- Runs a controlled TLS endpoint at `/v1/location` that returns the selected coordinate and revision.
- Supports optional diagnostic capture for the controlled lab endpoint only. Capture is off by default, redacts credential headers, and caps bodies at 2 MiB.
- Includes an Nginx SNI router with IPv4/IPv6 edge listeners, a systemd unit, automatic public-certificate renewal hooks, and bootstrap script.

## What v1 does not do

This version does **not** intercept or alter Apple Maps, Find My, Core Location, Apple production services, or third-party location services. It proves the profile → DoH → controlled TLS plumbing on a stock iPhone without bypassing platform protections.

## Development

Requires Go 1.22+ and `SHIFT_MY_ADMIN_PASSWORD` when running the server.

```bash
go test ./...
go vet ./...
```

## Deployment

Set a hostname to the VM's public IP, then run:

```bash
sudo ./deploy/scripts/bootstrap.sh lab.example.com 2001:db8::10 you@example.com
```

`SHIFT_MY_PUBLIC_IP` accepts either IPv4 or IPv6. For a Google Cloud deployment that avoids a billed external IPv4, use an external IPv6 address and publish an AAAA record for the public hostname before running the bootstrap script. See [`docs/testing/google-cloud.md`](docs/testing/google-cloud.md).

The script rejects Apple/iCloud production hostnames, obtains a normal public certificate for the dashboard/DoH hostname, generates the private lab CA server-side, creates a long random dashboard password, and configures exact-SNI routing only for the public hostname and the three derived lab subdomains. It prints the dashboard username (`shiftmy`) and generated password at the end; save the password securely.

After deployment, follow [`docs/testing/device-setup.md`](docs/testing/device-setup.md). To remove the test profile and trust cleanly, follow [`docs/testing/uninstall.md`](docs/testing/uninstall.md).

See [`docs/SAFETY.md`](docs/SAFETY.md) for the enforced lab boundary and [`docs/protocol/README.md`](docs/protocol/README.md) for the v1 wire behavior.
