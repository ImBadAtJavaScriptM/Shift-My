#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "run as root: sudo $0 <public-host> <public-ip> <letsencrypt-email>" >&2
  exit 1
fi
if [[ $# -ne 3 ]]; then
  echo "usage: $0 <public-host> <public-ip> <letsencrypt-email>" >&2
  exit 1
fi

PUBLIC_HOST="${1,,}"
PUBLIC_IP="$2"
LE_EMAIL="$3"
case "$PUBLIC_HOST" in
  apple.com|*.apple.com|icloud.com|*.icloud.com)
    echo "refusing production Apple/iCloud hostname: $PUBLIC_HOST" >&2
    exit 1
    ;;
esac
if ! python3 - "$PUBLIC_IP" <<'PY'
import ipaddress
import sys
try:
    ipaddress.ip_address(sys.argv[1])
except ValueError:
    raise SystemExit(1)
PY
then
  echo "invalid IP address: $PUBLIC_IP" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates certbot git golang-go nginx libnginx-mod-stream python3 sqlite3

if ! id -u shiftmy >/dev/null 2>&1; then
  useradd --system --home /var/lib/shift-my --shell /usr/sbin/nologin shiftmy
fi
install -d -m 0755 /opt/shift-my
install -d -o shiftmy -g shiftmy -m 0750 /var/lib/shift-my /var/lib/shift-my/captures /var/lib/shift-my/backups
install -d -o root -g shiftmy -m 0750 /etc/shift-my

# Take a transactionally consistent SQLite backup before a deployment can
# start a newer binary and run additive schema migration.
if [[ -f /var/lib/shift-my/state.db ]]; then
  BACKUP="/var/lib/shift-my/backups/state-$(date -u +%Y%m%dT%H%M%SZ).db"
  sqlite3 /var/lib/shift-my/state.db ".backup '$BACKUP'"
  chown shiftmy:shiftmy "$BACKUP"
  chmod 0640 "$BACKUP"
  echo "Created pre-deploy database backup: $BACKUP"
fi

SRC=/opt/shift-my/src
if [[ -d "$SRC/.git" ]]; then
  git -C "$SRC" fetch --depth 1 origin main
  git -C "$SRC" checkout --force FETCH_HEAD
else
  rm -rf "$SRC"
  git clone --depth 1 https://github.com/ImBadAtJavaScriptM/Shift-My.git "$SRC"
fi

git -C "$SRC" status --short
go build -C "$SRC" -trimpath -o /opt/shift-my/shift-my-server ./cmd/server
go build -C "$SRC" -trimpath -o /opt/shift-my/shift-my-ca-bootstrap ./cmd/ca-bootstrap

if [[ ! -f /etc/shift-my/root-ca-key.pem ]]; then
  /opt/shift-my/shift-my-ca-bootstrap -out /etc/shift-my
fi
if [[ ! -f /etc/shift-my/identity-ca-key.pem ]]; then
  /opt/shift-my/shift-my-ca-bootstrap \
    -out /etc/shift-my \
    -prefix identity-ca \
    -common-name "Shift-My Test Identity CA"
fi
chown root:shiftmy /etc/shift-my/root-ca-key.pem /etc/shift-my/identity-ca-key.pem
chmod 0640 /etc/shift-my/root-ca-key.pem /etc/shift-my/identity-ca-key.pem
chmod 0644 /etc/shift-my/root-ca.pem /etc/shift-my/identity-ca.pem

ADMIN_PASSWORD=""
if [[ -f /etc/shift-my/shift-my.env ]]; then
  ADMIN_PASSWORD="$(grep '^SHIFT_MY_ADMIN_PASSWORD=' /etc/shift-my/shift-my.env | head -n 1 | cut -d= -f2- || true)"
fi
if [[ -z "$ADMIN_PASSWORD" ]]; then
  ADMIN_PASSWORD="$(od -An -N18 -tx1 /dev/urandom | tr -d ' \n')"
fi

# The public dashboard/DoH endpoint needs a normal publicly trusted certificate.
# On an IPv6-only VM, the public hostname must have a working AAAA record before
# this command runs so the ACME HTTP-01 challenge can reach TCP/80 over IPv6.
systemctl stop nginx || true
certbot certonly --standalone --non-interactive --agree-tos --email "$LE_EMAIL" -d "$PUBLIC_HOST"
install -o root -g shiftmy -m 0640 "/etc/letsencrypt/live/$PUBLIC_HOST/privkey.pem" /etc/shift-my/public-key.pem
install -o root -g shiftmy -m 0644 "/etc/letsencrypt/live/$PUBLIC_HOST/fullchain.pem" /etc/shift-my/public.pem

# Standalone renewal needs port 80 free. After a successful renewal, refresh the
# copies readable by the unprivileged Shift-My service and restart that service.
install -d -m 0755 /etc/letsencrypt/renewal-hooks/pre /etc/letsencrypt/renewal-hooks/post /etc/letsencrypt/renewal-hooks/deploy
cat >/etc/letsencrypt/renewal-hooks/pre/shift-my-stop-nginx <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
systemctl stop nginx.service || true
EOF
cat >/etc/letsencrypt/renewal-hooks/post/shift-my-start-nginx <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
systemctl start nginx.service || true
EOF
cat >/etc/letsencrypt/renewal-hooks/deploy/shift-my-refresh-cert <<EOF
#!/usr/bin/env bash
set -euo pipefail
install -o root -g shiftmy -m 0640 "/etc/letsencrypt/live/$PUBLIC_HOST/privkey.pem" /etc/shift-my/public-key.pem
install -o root -g shiftmy -m 0644 "/etc/letsencrypt/live/$PUBLIC_HOST/fullchain.pem" /etc/shift-my/public.pem
systemctl restart shift-my.service || true
EOF
chmod 0755 /etc/letsencrypt/renewal-hooks/pre/shift-my-stop-nginx \
  /etc/letsencrypt/renewal-hooks/post/shift-my-start-nginx \
  /etc/letsencrypt/renewal-hooks/deploy/shift-my-refresh-cert

cat >/etc/shift-my/shift-my.env <<EOF
SHIFT_MY_PUBLIC_HOST=$PUBLIC_HOST
SHIFT_MY_PUBLIC_IP=$PUBLIC_IP
SHIFT_MY_ADMIN_PASSWORD=$ADMIN_PASSWORD
SHIFT_MY_DB_PATH=/var/lib/shift-my/state.db
SHIFT_MY_CA_CERT=/etc/shift-my/root-ca.pem
SHIFT_MY_CA_KEY=/etc/shift-my/root-ca-key.pem
SHIFT_MY_IDENTITY_CA_CERT=/etc/shift-my/identity-ca.pem
SHIFT_MY_IDENTITY_CA_KEY=/etc/shift-my/identity-ca-key.pem
SHIFT_MY_PUBLIC_CERT=/etc/shift-my/public.pem
SHIFT_MY_PUBLIC_KEY=/etc/shift-my/public-key.pem
SHIFT_MY_CAPTURE_ENABLED=false
SHIFT_MY_CAPTURE_DIR=/var/lib/shift-my/captures
EOF
chmod 0640 /etc/shift-my/shift-my.env
chown root:shiftmy /etc/shift-my/shift-my.env

sed "s/PUBLIC_HOST_PLACEHOLDER/$PUBLIC_HOST/g" "$SRC/deploy/nginx/nginx.conf" >/etc/nginx/nginx.conf
install -m 0644 "$SRC/deploy/systemd/shift-my.service" /etc/systemd/system/shift-my.service
nginx -t
systemctl daemon-reload
systemctl enable --now shift-my.service
systemctl enable nginx.service
systemctl restart nginx.service

echo "Shift-My controlled lab installed for https://$PUBLIC_HOST"
echo "Public IP: $PUBLIC_IP"
echo "Dashboard username: shiftmy"
echo "Dashboard password: $ADMIN_PASSWORD"
echo "Save that password. It is stored in /etc/shift-my/shift-my.env and is preserved on reruns."
echo "Next: open https://$PUBLIC_HOST on your iPhone, authenticate, install the generated profile, and follow docs/testing/device-setup.md."
