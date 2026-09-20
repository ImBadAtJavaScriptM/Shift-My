# Uninstall and restore normal networking

1. On the iPhone, open **Settings → General → VPN & Device Management**.
2. Select **Shift-My Test** and tap **Remove Profile**. Enter the device passcode if requested.
3. Confirm the profile's managed DNS configuration, declarative Stage 2 configuration, and associated device identity are no longer present.
4. Open **Settings → General → About → Certificate Trust Settings**. Confirm the Shift-My test root is no longer trusted; if a separately trusted copy remains, disable/remove that trust.
5. Reconnect Wi-Fi or briefly enable/disable Airplane Mode so networking refreshes.
6. Confirm normal websites load and that `device-loc.<public-host>` no longer resolves through the removed managed DoH profile unless your normal DNS independently defines it.

If you plan to reinstall rather than permanently remove the lab, use **Reset enrollment** in the authenticated dashboard after removing the old profile. Then download a fresh Stage 1 profile. Older downloads contain enrollment credentials that the reset invalidates.

## Server-side cleanup

To stop the lab:

```bash
sudo systemctl disable --now shift-my.service
sudo systemctl disable --now nginx.service
```

If permanently retiring the lab, archive only what you intentionally need and then remove `/var/lib/shift-my`, `/etc/shift-my`, and the dedicated Nginx configuration.

Never copy or publish either private CA key:

- `root-ca-key.pem`
- `identity-ca-key.pem`
