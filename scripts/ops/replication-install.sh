#!/usr/bin/env bash
# Install the US controlled replication service and its TTL reaper. Run as root.
#
# A production install must provide CONTRACT so the reaper can compare local
# pins with the live contract state. Use --no-reaper only for a staged data
# plane install before the mainnet contract exists; this deliberately leaves
# the reaper timer disabled.
set -euo pipefail

KUBO_UNIT="${IGIT_KUBO_UNIT:-kubo.service}"
IPFS_PATH_VALUE="${IGIT_IPFS_PATH:-/var/lib/ipfs}"
SERVICE_USER="${IGIT_REPLICATION_USER:-ipfs}"
SERVICE_GROUP="${IGIT_REPLICATION_GROUP:-$SERVICE_USER}"

enable_reaper=1
case "${1:-}" in
  "") ;;
  --no-reaper) enable_reaper=0 ;;
  *) echo "usage: $0 [--no-reaper]" >&2; exit 2 ;;
esac

install -m 0755 "$(dirname "$0")/replication-reaper.sh" /usr/local/bin/igit-replication-reaper.sh
install -m 0755 "$(dirname "$0")/replication-monitor.sh" /usr/local/bin/igit-replication-monitor.sh
install -m 0755 "$(dirname "$0")/replication-config-check.sh" /usr/local/bin/igit-replication-config-check.sh
install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0700 /var/lib/igit-replication
install -d -m 0755 /etc/igit
test -x /usr/local/bin/igit-replicationd || { echo 'install the built igit-replicationd binary at /usr/local/bin first' >&2; exit 1; }
test -f /etc/igit/replication.env || { echo 'missing /etc/igit/replication.env' >&2; exit 1; }
chmod 600 /etc/igit/replication.env
if [ "$enable_reaper" -eq 1 ]; then
  /usr/local/bin/igit-replication-config-check.sh reaper /etc/igit/replication.env
else
  /usr/local/bin/igit-replication-config-check.sh staged /etc/igit/replication.env
fi

cat >/etc/systemd/system/igit-replication.service <<'EOF'
[Unit]
Description=igit controlled US replication and Pin service
After=network-online.target @KUBO_UNIT@
Wants=@KUBO_UNIT@
[Service]
User=@SERVICE_USER@
Group=@SERVICE_GROUP@
Environment=IPFS_PATH=@IPFS_PATH@
Environment=KUBO_API=http://127.0.0.1:5001
Environment=IGIT_REPLICATION_STATE=/var/lib/igit-replication/issued.tsv
EnvironmentFile=/etc/igit/replication.env
ExecStartPre=/usr/local/bin/igit-replication-config-check.sh staged /etc/igit/replication.env
ExecStart=/usr/local/bin/igit-replicationd
Restart=on-failure
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
[Install]
WantedBy=multi-user.target
EOF
cat >/etc/systemd/system/igit-replication-reaper.service <<'EOF'
[Unit]
Description=Reclaim unreferenced igit US Pins after TTL
After=network-online.target @KUBO_UNIT@
[Service]
Type=oneshot
User=@SERVICE_USER@
Group=@SERVICE_GROUP@
Environment=IPFS_PATH=@IPFS_PATH@
Environment=IGIT_REPLICATION_STATE=/var/lib/igit-replication/issued.tsv
EnvironmentFile=/etc/igit/replication.env
ExecStartPre=/usr/local/bin/igit-replication-config-check.sh reaper /etc/igit/replication.env
ExecStart=/usr/local/bin/igit-replication-reaper.sh
EOF
cat >/etc/systemd/system/igit-replication-reaper.timer <<'EOF'
[Unit]
Description=Run igit unreferenced-Pin TTL reaper hourly
[Timer]
OnBootSec=15min
OnUnitActiveSec=1h
Persistent=true
[Install]
WantedBy=timers.target
EOF
cat >/etc/systemd/system/igit-replication-monitor.service <<'EOF'
[Unit]
Description=Emit igit replication metrics
[Service]
Type=oneshot
ExecStart=/usr/local/bin/igit-replication-monitor.sh
EOF
cat >/etc/systemd/system/igit-replication-monitor.timer <<'EOF'
[Unit]
Description=Emit igit replication metrics every five minutes
[Timer]
OnBootSec=3min
OnUnitActiveSec=5min
Persistent=true
[Install]
WantedBy=timers.target
EOF
sed -i \
  -e "s|@KUBO_UNIT@|$KUBO_UNIT|g" \
  -e "s|@SERVICE_USER@|$SERVICE_USER|g" \
  -e "s|@SERVICE_GROUP@|$SERVICE_GROUP|g" \
  -e "s|@IPFS_PATH@|$IPFS_PATH_VALUE|g" \
  /etc/systemd/system/igit-replication.service \
  /etc/systemd/system/igit-replication-reaper.service
systemctl daemon-reload
systemctl enable --now igit-replication.service igit-replication-monitor.timer
if [ "$enable_reaper" -eq 1 ]; then
  systemctl enable --now igit-replication-reaper.timer
else
  systemctl disable --now igit-replication-reaper.timer 2>/dev/null || true
  echo 'staged install: TTL reaper timer remains disabled until CONTRACT is configured'
fi
