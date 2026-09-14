# Uninstall and restore normal networking

1. On the iPhone, open **Settings → General → VPN & Device Management**.
2. Select **Shift-My Test** and tap **Remove Profile**. Enter the device passcode if requested.
3. Open **Settings → General → About → Certificate Trust Settings**. Confirm the Shift-My test root is no longer trusted; if it remains listed separately, disable trust/remove it.
4. Reconnect Wi-Fi or briefly enable/disable Airplane Mode so network state is refreshed.
5. Confirm normal websites load and that `device-loc.<public-host>` no longer resolves through the removed managed DoH profile unless your normal DNS happens to define it.

## Server-side cleanup

To stop the lab on a VPS:

```bash
sudo systemctl disable --now shift-my.service
sudo systemctl disable --now nginx.service
```

If you are permanently retiring the lab, archive anything you intentionally need, then remove `/var/lib/shift-my`, `/etc/shift-my`, and the dedicated Nginx configuration. Never copy or publish `root-ca-key.pem`.
