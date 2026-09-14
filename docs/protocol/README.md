# Shift-My v1 controlled protocol

## Public HTTPS service

The configured public hostname serves:

- `GET /` — dashboard
- `GET /profile.mobileconfig` — removable iPhone configuration profile
- `GET /api/status` — safe installation/traffic status; the profile token is never returned
- `POST /api/location` — update the selected test coordinate
- `GET|POST /dns-query/<profile-token>` — RFC 8484 DNS-over-HTTPS

The dashboard, profile, static assets, and `/api/*` routes require HTTP Basic authentication with username `shiftmy` and the server-configured admin password. The DoH route is intentionally exempt from Basic auth because the installed profile authenticates it with an independent high-entropy URL token. The public handler rejects any unexpected Host header with HTTP 421.

## Managed DNS scope

The profile's `SupplementalMatchDomains` contains exactly:

- `loc-a.<public-host>`
- `loc-b.<public-host>`
- `device-loc.<public-host>`

For an authenticated profile token, DoH returns the configured server IPv4 for A queries to those names, returns NODATA for AAAA, and returns DNS REFUSED for names outside the allowlist. It never acts as a general recursive resolver.

## Controlled TLS service

Nginx routes SNI for only the three lab names to the lab TLS listener. The Go service dynamically mints a short-lived single-SAN leaf certificate from the project test CA only when the requested SNI is in the controlled policy.

`GET https://device-loc.<public-host>/v1/location` (and the equivalent `loc-a`/`loc-b` host) returns:

```json
{
  "latitude": 40.758,
  "longitude": -73.9855,
  "label": "Times Square",
  "revision": 1
}
```

Unknown SNI/Host values are rejected rather than proxied or forwarded.

## State markers

A successful DoH query records `doh_seen_at`. A successful controlled TLS location response records `proxy_seen_at`. The dashboard converts those timestamps to boolean status indicators and does not expose the private profile token.
