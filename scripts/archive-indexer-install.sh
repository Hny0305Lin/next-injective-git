#!/usr/bin/env bash
# Install the US archive indexer as a five-minute systemd timer.
set -euo pipefail

install -m 0755 "$(dirname "$0")/archive-indexer.sh" /usr/local/bin/archive-indexer.sh
install -d -m 700 /etc/igit
install -d -o ipfs -g ipfs -m 700 /var/lib/igit-archive
test -f /etc/igit/filone.env || {
    echo "missing /etc/igit/filone.env" >&2
    exit 1
}
chmod 600 /etc/igit/filone.env

cat > /etc/systemd/system/igit-archive-indexer.service <<'EOF'
[Unit]
Description=igit US IPFS pin and Fil.one CAR archive indexer
After=network.target kubo.service
Wants=kubo.service

[Service]
Type=oneshot
User=ipfs
Group=ipfs
Environment=IPFS_PATH=/var/lib/ipfs
Environment=IGIT_HOME=/var/lib/igit-archive
Environment=ARCHIVE_ENV=/etc/igit/filone.env
EnvironmentFile=/etc/igit/filone.env
ExecStart=/usr/local/bin/archive-indexer.sh --once
EOF

cat > /etc/systemd/system/igit-archive-indexer.timer <<'EOF'
[Unit]
Description=Run igit US archive indexer every five minutes

[Timer]
OnBootSec=2min
OnUnitActiveSec=5min
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl disable --now igit-us-pin-indexer.timer 2>/dev/null || true
systemctl enable --now igit-archive-indexer.timer
