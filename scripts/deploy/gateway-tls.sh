#!/usr/bin/env bash
# gateway-tls: obtain a Let's Encrypt cert for the HK IPFS gateway and enable
# HTTPS with HTTP redirected after ACME validation. Idempotent.
# Run as root ON THE SERVER:  bash gateway-tls.sh [domain] [email]
set -euo pipefail

DOMAIN="${1:-igit-hk.haohanyh.ovh}"
EMAIL="${2:-linmengjia20030305@gmail.com}"

echo "== [1/4] certbot install =="
if ! command -v certbot >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y -qq --no-install-recommends certbot python3-certbot-nginx
fi
certbot --version

echo "== [2/4] nginx server_name = $DOMAIN =="
cat > /etc/nginx/sites-available/ipfs-gateway <<EOF
server {
    listen 80 default_server;
    server_name $DOMAIN;

    # read-only path gateway: local Kubo first, public-gateway fallback on miss
    location /ipfs/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_connect_timeout 5s;
        proxy_read_timeout 25s;
        proxy_intercept_errors on;
        error_page 404 500 502 503 504 = @pubgw;
        client_max_body_size 0;
    }
    # fallback: relay to a LIVE public IPFS gateway (cloudflare-ipfs is dead; ipfs.io works)
    location @pubgw {
        proxy_pass https://ipfs.io;
        proxy_set_header Host ipfs.io;
        proxy_ssl_server_name on;
        proxy_connect_timeout 8s;
        proxy_read_timeout 60s;
    }
    location = /healthz { return 200 "ok\n"; }
    location / { return 403; }
}
EOF
ln -sf /etc/nginx/sites-available/ipfs-gateway /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx

echo "== [3/4] obtain cert + enable TLS with HTTP redirect =="
certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" \
  --redirect --keep-until-expiring
nginx -t && systemctl reload nginx

echo "== [4/4] verify + auto-renew =="
curl -s -m 10 "https://$DOMAIN/healthz" && echo " <- https healthz OK (from server)"
echo "-- renew timer --"
systemctl list-timers certbot.timer --no-pager 2>/dev/null | head -2 || true
certbot certificates 2>/dev/null | grep -E "Certificate Name|Expiry|Domains" || true
