# Shift-My controlled protocol

## Public HTTPS service

The configured public hostname serves three authentication classes.

### Dashboard/admin routes

These require HTTP Basic authentication with username `shiftmy` and the server-configured admin password:

- `GET /` — dashboard
- `GET /profile.mobileconfig` — Stage 1 iPhone configuration profile
- `GET /api/status` — safe lifecycle/location status
- `POST /api/location` — update the selected test coordinate
- `POST /api/enrollment/reset` — rotate enrollment credentials and clear enrollment lifecycle state while preserving the selected target

### Automatic Stage 2

- `GET /api/profile/standard.mobileconfig?p=<stage2-token>`

This route does not use dashboard Basic Auth because iOS retrieves it automatically. It requires the independent Stage 2 token embedded in the Stage 1 declaration. A successful delivery increments the Stage 2 delivery counter.

### Device-facing routes

- `GET|POST /dns-query/<profile-token>` — RFC 8484 DNS-over-HTTPS
- `/acme/device/*` — ACME device-identity service

These routes do not use dashboard Basic Auth. DoH is protected by its high-entropy path token. ACME uses RFC 8555-style signed requests, replay nonces, a single enrolled account key, a one-installation permanent identifier, and managed-device attestation.

The public handler rejects an unexpected Host header with HTTP 421.

## Stage 1 profile

Stage 1 contains:

1. `com.apple.declarations`
   - a Legacy Profile declaration points to the tokenized Stage 2 URL;
   - an Activation Simple declaration activates that standard configuration.
2. `com.apple.security.acme`
   - directory: `https://<public-host>/acme/device/directory`
   - P-256 hardware-bound key
   - attestation enabled
   - non-extractable key
   - client-auth EKU only
3. `com.apple.dnsSettings.managed`
   - encrypted DNS for only the controlled lab hostnames.

Stage 1 does not contain the project TLS root CA.

## Stage 2 profile

Stage 2 contains:

1. `com.apple.security.root` — the project-controlled lab TLS root certificate.
2. `com.apple.dnsSettings.managed` — the same controlled DoH rules.

The Stage 2 token is installation-scoped so iOS may retry retrieval. It is rotated by **Reset enrollment**.

## ACME identity flow

The single-iPhone ACME service exposes:

- `GET /acme/device/directory`
- `HEAD|GET /acme/device/new-nonce`
- `POST /acme/device/new-account`
- `POST /acme/device/new-order`
- `POST /acme/device/account/1`
- `POST /acme/device/account/1/orders`
- `POST /acme/device/order/<id>`
- `POST /acme/device/authz/<id>`
- `POST /acme/device/challenge/<id>`
- `POST /acme/device/order/<id>/finalize`
- `POST /acme/device/cert/<id>`

An order must contain exactly one `permanent-identifier`, and its value must match the current installation's `ClientIdentifier`.

The authorization exposes one `device-attest-01` challenge. The server validates:

- the WebAuthn/CBOR Apple attestation object;
- the certificate path to the Apple Enterprise Attestation Root CA;
- the freshness extension against SHA-256 of the current challenge token;
- the attested key type required by the profile;
- at finalization, that the CSR public key is the same key attested during the challenge.

Only after those checks does the separate identity CA issue a short-lived certificate. Issued identity certificates contain client-auth EKU only; they do not contain server-auth EKU.

ACME nonces are stored as SHA-256 hashes and consumed once. Orders/accounts/issuance metadata persist in SQLite across service restarts.

## Managed DNS scope

`SupplementalMatchDomains` contains exactly:

- `loc-a.<public-host>`
- `loc-b.<public-host>`
- `device-loc.<public-host>`

For a valid profile token, DoH answers only the configured address family. Names outside the allowlist return DNS REFUSED; the service never acts as a general recursive resolver.

## Controlled TLS service

Nginx listens on IPv4/IPv6 at the edge and routes SNI for only the three lab names to the controlled TLS listener. The Go service mints short-lived, single-SAN leaf certificates from the project lab CA only when the requested SNI is in the controlled policy.

`GET https://device-loc.<public-host>/v1/location` (and the equivalent `loc-a` / `loc-b` host) returns:

```json
{
  "latitude": 40.758,
  "longitude": -73.9855,
  "label": "Times Square",
  "revision": 1
}
```

Unknown SNI/Host values are rejected rather than proxied or forwarded.

## Lifecycle status

The dashboard derives booleans from persisted facts:

- Identity enrolled
- Stage 2 delivered
- DoH seen
- Lab TLS seen

Sensitive values such as the DoH token, Stage 2 token, ClientIdentifier, and ACME account material are not returned by `/api/status`.
