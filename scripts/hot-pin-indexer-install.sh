#!/usr/bin/env bash
# Install the HK hot-tier indexer. It never unpins unless ALLOW_UNPIN=true is
# explicitly set in /etc/igit/hot-pin.env.
set -euo pipefail

install -m 0755 "$(dirname "$0")/hot-pin-indexer.sh" /usr/local/bin/hot-pin-indexer.sh
install -d -m 700 /etc/igit /var/lib/igit
touch /etc/igit/important-cids.list /var/lib/igit/durable-cids.list
chmod 600 /etc/igit/important-cids.list /var/lib/igit/durable-cids.list

if [ ! -f /etc/igit/hot-pin.env ]; then
    cat > /etc/igit/hot-pin.env <<'EOF'
NEW_DAYS=14
HOT_WINDOW_DAYS=30
HOT_MIN_HITS=3
ALLOW_UNPIN=false
EOF
    chmod 600 /etc/igit/hot-pin.env
fi

cat > /etc/systemd/system/hot-pin-indexer.service <<'EOF'
[Unit]
Description=igit HK hot IPFS pin indexer
After=network.target ipfs.service
Wants=ipfs.service

[Service]
Type=oneshot
Environment=IPFS_PATH=/var/lib/ipfs
Environment=IGIT_HOME=/var/lib/igit
EnvironmentFile=/etc/igit/hot-pin.env
ExecStart=/usr/local/bin/hot-pin-indexer.sh
EOF

cat > /etc/systemd/system/hot-pin-indexer.timer <<'EOF'
[Unit]
Description=Run igit HK hot pin indexer every five minutes

[Timer]
OnBootSec=3min
OnUnitActiveSec=5min
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now hot-pin-indexer.timer
