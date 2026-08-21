#!/usr/bin/env bash
# monitor-install: install the Filebase usage monitor as a daily systemd timer.
# Run as root ON THE SERVER. Prereqs (placed by scp before running):
#   /usr/local/bin/filebase-monitor.sh   the checker script (committed)
#   /etc/igit/monitor.env                 600, holds Filebase + Resend creds
set -euo pipefail

need=""
command -v jq  >/dev/null 2>&1 || need="$need jq"
command -v aws >/dev/null 2>&1 || need="$need awscli"
if [ -n "$need" ]; then apt-get update -qq && apt-get install -y -qq $need; fi
chmod +x /usr/local/bin/filebase-monitor.sh
test -f /etc/igit/monitor.env || { echo "missing /etc/igit/monitor.env" >&2; exit 1; }
chmod 600 /etc/igit/monitor.env

cat > /etc/systemd/system/filebase-monitor.service <<'EOF'
[Unit]
Description=Filebase free-tier usage monitor (email alert via Resend)
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/filebase-monitor.sh
EOF

cat > /etc/systemd/system/filebase-monitor.timer <<'EOF'
[Unit]
Description=Run Filebase usage monitor hourly (email cadence capped in-script)

[Timer]
OnCalendar=hourly
RandomizedDelaySec=120
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now filebase-monitor.timer
echo "== next scheduled run =="
systemctl list-timers filebase-monitor.timer --no-pager | head -3

echo "== test run (real thresholds; should be 'nothing to send') =="
systemctl start filebase-monitor.service
sleep 2
journalctl -u filebase-monitor.service -n 12 --no-pager
