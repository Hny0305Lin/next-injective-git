#!/usr/bin/env bash
# gateway-deploy: one-shot setup for the HK IPFS read-only gateway node.
# Target: Debian 11+, 1C/1G/5G — tuned for low memory and small disk.
# Run as root ON THE SERVER:  bash gateway-deploy.sh
set -euo pipefail

KUBO_VER="v0.42.0"          # keep in sync with dev machines
STORAGE_MAX="1GB"           # small 5G disk: cap the IPFS repo hard
SWAP_SIZE_MB=512            # small 5G disk: modest swap as OOM backstop

echo "== [1/5] swap =="
if ! swapon --show | grep -q swapfile; then
  fallocate -l ${SWAP_SIZE_MB}M /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi
free -m | grep -i swap

echo "== [2/5] kubo binary =="
if ! command -v ipfs >/dev/null; then
  cd /tmp
  curl -sSL -o kubo.tar.gz \
    "https://dist.ipfs.tech/kubo/${KUBO_VER}/kubo_${KUBO_VER}_linux-amd64.tar.gz"
  tar xzf kubo.tar.gz
  install -m 755 kubo/ipfs /usr/local/bin/ipfs
  rm -rf kubo kubo.tar.gz
fi
ipfs version

echo "== [3/5] ipfs init + lowpower tuning =="
export IPFS_PATH=/var/lib/ipfs
mkdir -p "$IPFS_PATH"
if [ ! -f "$IPFS_PATH/config" ]; then
  ipfs init --profile=lowpower,server
fi
ipfs config Datastore.StorageMax "$STORAGE_MAX"
ipfs config --json Swarm.ConnMgr.LowWater 32
ipfs config --json Swarm.ConnMgr.HighWater 96
ipfs config --json Swarm.ResourceMgr.MaxMemory '"256MB"'
# gateway/API bind: local only; nginx fronts the gateway
ipfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8080
ipfs config Addresses.API /ip4/127.0.0.1/tcp/5001
ipfs config --json Gateway.NoFetch false

echo "== [4/5] systemd service =="
cat > /etc/systemd/system/ipfs.service <<'EOF'
[Unit]
Description=IPFS kubo daemon
After=network.target

[Service]
Environment=IPFS_PATH=/var/lib/ipfs
ExecStart=/usr/local/bin/ipfs daemon --migrate
Restart=on-failure
RestartSec=10
MemoryHigh=450M
MemoryMax=750M
MemorySwapMax=512M
LimitNOFILE=8192

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now ipfs
sleep 8
systemctl --no-pager status ipfs | head -5

echo "== [5/5] nginx read-only gateway =="
apt-get update -qq && apt-get install -y -qq nginx >/dev/null
cat > /etc/nginx/sites-available/ipfs-gateway <<'EOF'
server {
    listen 80 default_server;
    server_name _;

    # read-only path gateway: /ipfs/<cid> only
    location /ipfs/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_read_timeout 120s;
        client_max_body_size 0;
    }
    location = /healthz { return 200 "ok\n"; }
    location / { return 403; }
}
EOF
ln -sf /etc/nginx/sites-available/ipfs-gateway /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx

echo "== done =="
echo "verify: curl http://<ip>/healthz ; curl http://<ip>/ipfs/<cid>"
