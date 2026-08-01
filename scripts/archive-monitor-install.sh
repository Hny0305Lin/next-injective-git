#!/usr/bin/env bash
# Install the US archive monitor after /etc/igit/archive-monitor.env is ready.
set -euo pipefail

install -m 0755 "$(dirname "$0")/archive-monitor.sh" /usr/local/bin/archive-monitor.sh
test -f /etc/igit/filone.env || { echo "missing /etc/igit/filone.env" >&2; exit 1; }
test -f /etc/igit/archive-monitor.env || { echo "missing /etc/igit/archive-monitor.env" >&2; exit 1; }
chown root:root /etc/igit/archive-monitor.env
chmod 600 /etc/igit/archive-monitor.env
install -d -m 700 /var/lib/igit-archive-monitor

cat > /etc/systemd/system/igit-archive-monitor.service <<'EOF'
[Unit]
Description=igit US archive health and capacity monitor
After=network.target kubo.service
Wants=kubo.service

[Service]
Type=oneshot
EnvironmentFile=/etc/igit/filone.env
EnvironmentFile=/etc/igit/archive-monitor.env
ExecStart=/usr/local/bin/archive-monitor.sh
EOF

cat > /etc/systemd/system/igit-archive-monitor.timer <<'EOF'
[Unit]
Description=Run igit US archive monitor hourly

[Timer]
OnBootSec=10min
OnUnitActiveSec=1h
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now igit-archive-monitor.timer
