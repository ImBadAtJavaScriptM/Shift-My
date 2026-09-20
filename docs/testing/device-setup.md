# iPhone controlled-lab setup

These steps validate the two-stage profile → hardware-bound ACME identity → controlled DoH → controlled TLS path on your own iPhone. They do not change Apple Maps, Find My, Core Location, or Apple production services.

## Before the phone

1. Deploy the server using a hostname you control.
2. If the deployment is IPv6-only, first confirm the iPhone's current Wi-Fi or cellular network can load the dashboard over IPv6.
3. Save the dashboard credentials printed by the bootstrap script. The username is `shiftmy`; the password is stored in `/etc/shift-my/shift-my.env`.
4. Confirm `https://<public-host>/` loads in Safari and authenticate.
5. In the dashboard, set a clearly recognizable test coordinate.

## Replace the old test profile

If an earlier **Shift-My Test** profile is installed:

1. Open **Settings → General → VPN & Device Management**.
2. Remove the old Shift-My Test profile.
3. Return to Safari.

Do not install both generations simultaneously.

## Install Stage 1

1. Open the authenticated dashboard in Safari.
2. Tap **Download iPhone profile**.
3. Open **Settings → General → VPN & Device Management**.
4. Select **Shift-My Test** and review the payloads.
5. Install it.

Stage 1 contains the declarative bootstrap, a hardware-bound attested device identity request, and controlled DoH configuration. It does not contain the lab TLS root CA.

After installation, iOS should contact the project's ACME directory, generate the hardware-bound identity key, answer the managed-device attestation challenge, finalize the identity certificate request, and retrieve Stage 2 through the declaration.

## Watch enrollment status

Return to the dashboard. The relevant cards are independent, so they may change in a different order during retries:

- **Identity** → `seen` after the attested identity certificate is issued.
- **Stage 2** → `seen` after iOS retrieves the second profile.
- **DoH** → `seen` after the managed DNS path is used.
- **Lab TLS** → `seen` after a controlled lab hostname completes TLS.

If Stage 2 installs the project test root but iOS still asks for explicit certificate trust, open **Settings → General → About → Certificate Trust Settings** and enable trust only for the Shift-My test root you just installed.

## Validate the controlled path

1. Wait for **Stage 2** and **DoH** to show `seen`.
2. Open:
   `https://device-loc.<public-host>/v1/location`
3. Confirm the JSON contains the dashboard's selected latitude, longitude, label, and revision.
4. Return to the dashboard and confirm **Lab TLS** is `seen`.
5. Change the target and reload the controlled URL; its revision and coordinates should update.

A successful result proves the two-stage enrollment, attested identity, controlled DNS, and controlled TLS plumbing. It is not evidence that iOS system location has been overridden.

## Recover from a failed enrollment

If Identity or Stage 2 remains stuck after retrying the install:

1. Remove the current Shift-My Test profile from the iPhone.
2. In the authenticated dashboard, press **Reset enrollment** and confirm.
3. The server rotates the DoH token, Stage 2 token, and ClientIdentifier and clears ACME enrollment state.
4. Your saved target coordinates, label, and revision remain unchanged.
5. Download and install the newly generated Stage 1 profile.

Do not reuse an older downloaded Stage 1 profile after resetting enrollment because its identifiers and Stage 2 URL are no longer valid.

## Optional diagnostics

Server-side capture remains disabled by default. If deliberately enabled with `SHIFT_MY_CAPTURE_ENABLED=true`, it applies only to the controlled lab endpoint, redacts common credential headers, and caps stored bodies at 2 MiB.
