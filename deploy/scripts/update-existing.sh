#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "run as root: sudo $0" >&2
  exit 1
fi

SRC=/opt/shift-my/src
ENV_FILE=/etc/shift-my/shift-my.env
STATE_DB=/var/lib/shift-my/state.db
BACKUP_DIR=/var/lib/shift-my/backups
SERVER_BIN=/opt/shift-my/shift-my-server
PREV_BIN=/opt/shift-my/shift-my-server.prev
CA_BOOTSTRAP=/opt/shift-my/shift-my-ca-bootstrap

if [[ ! -d "$SRC/.git" ]]; then
  echo "existing Shift-My checkout not found at $SRC" >&2
  exit 1
fi
if [[ ! -f "$ENV_FILE" ]]; then
  echo "existing Shift-My environment not found at $ENV_FILE" >&2
  exit 1
fi

install -d -o shiftmy -g shiftmy -m 0750 "$BACKUP_DIR"

BACKUP=""
if [[ -f "$STATE_DB" ]]; then
  BACKUP="$BACKUP_DIR/state-$(date -u +%Y%m%dT%H%M%SZ).db"
  python3 - "$STATE_DB" "$BACKUP" <<'PY'
import sqlite3
import sys

src, dst = sys.argv[1:3]
source = sqlite3.connect(f"file:{src}?mode=ro", uri=True)
target = sqlite3.connect(dst)
with target:
    source.backup(target)
target.close()
source.close()
PY
  chown shiftmy:shiftmy "$BACKUP"
  chmod 0640 "$BACKUP"
  echo "Created pre-upgrade database backup: $BACKUP"
fi

# Refresh the checkout only after the backup exists.
git -C "$SRC" fetch --depth 1 origin main
git -C "$SRC" checkout --force FETCH_HEAD

# Keep the currently running binary available for an immediate service rollback.
if [[ -x "$SERVER_BIN" ]]; then
  cp -a "$SERVER_BIN" "$PREV_BIN"
fi

go build -C "$SRC" -trimpath -o "$SERVER_BIN.new" ./cmd/server
go build -C "$SRC" -trimpath -o "$CA_BOOTSTRAP.new" ./cmd/ca-bootstrap
install -o root -g root -m 0755 "$SERVER_BIN.new" "$SERVER_BIN"
install -o root -g root -m 0755 "$CA_BOOTSTRAP.new" "$CA_BOOTSTRAP"
rm -f "$SERVER_BIN.new" "$CA_BOOTSTRAP.new"

# The attested enrollment flow uses a CA that is separate from the lab TLS CA.
if [[ ! -f /etc/shift-my/identity-ca-key.pem || ! -f /etc/shift-my/identity-ca.pem ]]; then
  "$CA_BOOTSTRAP"     -out /etc/shift-my     -prefix identity-ca     -common-name "Shift-My Test Identity CA"
fi
chown root:shiftmy /etc/shift-my/identity-ca-key.pem
chmod 0640 /etc/shift-my/identity-ca-key.pem
chmod 0644 /etc/shift-my/identity-ca.pem

if ! grep -q '^SHIFT_MY_IDENTITY_CA_CERT=' "$ENV_FILE"; then
  echo 'SHIFT_MY_IDENTITY_CA_CERT=/etc/shift-my/identity-ca.pem' >>"$ENV_FILE"
fi
if ! grep -q '^SHIFT_MY_IDENTITY_CA_KEY=' "$ENV_FILE"; then
  echo 'SHIFT_MY_IDENTITY_CA_KEY=/etc/shift-my/identity-ca-key.pem' >>"$ENV_FILE"
fi
chown root:shiftmy "$ENV_FILE"
chmod 0640 "$ENV_FILE"

# shellcheck disable=SC1090
source "$ENV_FILE"
: "${SHIFT_MY_PUBLIC_HOST:?SHIFT_MY_PUBLIC_HOST missing from environment}"

install -m 0644 "$SRC/deploy/systemd/shift-my.service" /etc/systemd/system/shift-my.service
sed "s/PUBLIC_HOST_PLACEHOLDER/${SHIFT_MY_PUBLIC_HOST}/g" "$SRC/deploy/nginx/nginx.conf" >/etc/nginx/nginx.conf
nginx -t
systemctl daemon-reload

if ! systemctl restart shift-my.service; then
  echo "new service failed to start; restoring previous server binary" >&2
  if [[ -x "$PREV_BIN" ]]; then
    install -o root -g root -m 0755 "$PREV_BIN" "$SERVER_BIN"
    systemctl restart shift-my.service || true
  fi
  exit 1
fi
systemctl restart nginx.service

if [[ "$(systemctl is-active shift-my.service)" != "active" ]]; then
  echo "shift-my.service is not active after upgrade" >&2
  exit 1
fi
if [[ "$(systemctl is-active nginx.service)" != "active" ]]; then
  echo "nginx.service is not active after upgrade" >&2
  exit 1
fi

echo "Shift-My existing VM upgraded successfully."
if [[ -n "$BACKUP" ]]; then
  echo "Database backup: $BACKUP"
fi
echo "Dashboard/database state was preserved."
echo "The first start initializes the new single-iPhone enrollment credentials."
echo "Next: remove the old Shift-My Test profile from the iPhone, then install the new Stage 1 profile from the dashboard."
