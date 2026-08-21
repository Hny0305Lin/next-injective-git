#!/usr/bin/env bash
# Install the US-to-HK durable CID sync timer after archive-sync.key exists.
set -euo pipefail

install -m 0755 "$(dirname "$0")/durable-cid-sync.sh" /usr/local/bin/durable-cid-sync.sh
test -f /etc/igit/archive-sync.key || {
    echo "missing /etc/igit/archive-sync.key" >&2
    exit 1
}
test -f /etc/igit/archive-sync.known_hosts || {
    echo "missing /etc/igit/archive-sync.known_hosts" >&2
    exit 1
}
chmod 600 /etc/igit/archive-sync.key

cat > /etc/systemd/system/igit-durable-cid-sync.service <<'EOF'
[Unit]
Description=Sync igit durable CID allowlist to HK
After=network.target igit-archive-indexer.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/durable-cid-sync.sh
EOF

cat > /etc/systemd/system/igit-durable-cid-sync.timer <<'EOF'
[Unit]
Description=Sync igit durable CID allowlist every five minutes

[Timer]
OnBootSec=3min
OnUnitActiveSec=5min
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now igit-durable-cid-sync.timer
