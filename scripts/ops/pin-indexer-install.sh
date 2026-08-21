#!/usr/bin/env bash
# pin-indexer-install: run the pin-indexer on the HK node as a resident systemd
# timer. Each pushed packfile gets pinned to THIS node's local Kubo (the gateway)
# AND replicated to Filebase — i.e. HK + Filebase stay in sync automatically.
# Prereqs (scp'd first): /usr/local/bin/pin-indexer.sh ; FILEBASE_* creds in
# /etc/igit/monitor.env.
set -euo pipefail

chmod +x /usr/local/bin/pin-indexer.sh
test -f /etc/igit/monitor.env || { echo "missing /etc/igit/monitor.env (FILEBASE_* creds)" >&2; exit 1; }
mkdir -p /var/lib/igit

cat > /etc/systemd/system/pin-indexer.service <<'EOF'
[Unit]
Description=igit pin-indexer (pin pushed packfiles to local Kubo + Filebase)
After=network-online.target ipfs.service
Wants=ipfs.service

[Service]
Type=oneshot
Environment=IPFS_PATH=/var/lib/ipfs
Environment=IGIT_HOME=/var/lib/igit
EnvironmentFile=/etc/igit/monitor.env
ExecStart=/usr/local/bin/pin-indexer.sh --once
EOF

cat > /etc/systemd/system/pin-indexer.timer <<'EOF'
[Unit]
Description=Run igit pin-indexer every 2 minutes

[Timer]
OnBootSec=1min
OnUnitActiveSec=2min
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now pin-indexer.timer
echo "== next scheduled runs =="
systemctl list-timers pin-indexer.timer --no-pager | head -3
echo "== first run (pins existing packfiles to THIS node + Filebase) =="
systemctl start pin-indexer.service
journalctl -u pin-indexer.service -n 40 --no-pager | tail -40
