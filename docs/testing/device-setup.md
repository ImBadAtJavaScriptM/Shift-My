# iPhone controlled-lab setup

These steps validate the Shift-My v1 profile → DoH → controlled TLS path on your own iPhone. They do not change Apple Maps or Core Location.

## Before the phone

1. Deploy the server on a VPS or Compute Engine VM using a hostname you control.
2. If the public deployment uses only IPv6, confirm the iPhone's current Wi-Fi or cellular network has IPv6 connectivity by loading the dashboard in Safari first.
3. Save the dashboard credentials printed by the bootstrap script. The username is `shiftmy`; the password is randomly generated and stored in `/etc/shift-my/shift-my.env` on the server.
4. Confirm `https://<public-host>/` loads in Safari or another browser and authenticate when prompted.
5. In the dashboard, enter a clearly recognizable test coordinate and press **Set target**.

## Install the profile

1. On the iPhone, open `https://<public-host>/` in Safari and authenticate with the dashboard credentials.
2. Tap **Download iPhone profile** and allow the download.
3. Open **Settings → General → VPN & Device Management** and select the downloaded **Shift-My Test** profile.
4. Review the payloads and install the profile.
5. If iOS requires explicit trust for the project test root, open **Settings → General → About → Certificate Trust Settings** and enable full trust only for the Shift-My test root you just installed.

The generated profile is removable and its managed DoH rules are scoped only to `loc-a.<public-host>`, `loc-b.<public-host>`, and `device-loc.<public-host>`. The dashboard/profile/API require Basic authentication; the DoH endpoint does not because the installed profile uses its independent high-entropy path token.

## Validate DoH

1. Return to the authenticated dashboard.
2. Wait for the **DoH** card to change from `waiting` to `seen`.
3. If it stays on `waiting`, confirm the profile is installed, the public hostname resolves to the server's configured address family, and TCP/443 is reachable from the iPhone's current network.

## Validate controlled TLS

1. On the same iPhone, open `https://device-loc.<public-host>/v1/location`.
2. The page should return JSON containing the target latitude, longitude, label, and revision from the dashboard.
3. Return to the dashboard and confirm **Lab TLS** changes to `seen`.
4. Change the target in the dashboard and reload the controlled lab URL; the JSON revision should increment and the coordinate should update.

A successful result proves that the installed profile can route the controlled hostname through the project's DoH endpoint and complete TLS to a project-owned lab service. It is not evidence that iOS system location has been overridden.

## Optional diagnostics

Server-side capture is disabled by default. If you deliberately enable `SHIFT_MY_CAPTURE_ENABLED=true`, captures are limited to the controlled lab endpoint, redact common credential headers, and cap stored bodies at 2 MiB. Do not enable capture unless you need it for this lab.
