#!/usr/bin/env bash
# gateway-us-deploy: enable the public, read-only US IPFS gateway.
#
# The Kubo API and native gateway remain loopback-only. nginx publishes only
# GET/HEAD /ipfs/<cid> and a Kubo-backed /healthz endpoint over HTTPS.
# Run as root ON THE US SERVER: bash gateway-us-deploy.sh [domain] [email]
set -euo pipefail

DOMAIN="${1:-igit-us.haohanyh.ovh}"
EMAIL="${2:-linmengjia20030305@gmail.com}"

if ! systemctl is-active --quiet kubo; then
  echo "kubo.service is not active; refusing to publish a dead gateway" >&2
  exit 1
fi

apt-get update -qq
apt-get install -y -qq --no-install-recommends nginx certbot python3-certbot-nginx

# Preserve an existing default-deny policy while admitting only the public
# HTTPS data plane and ACME's HTTP-01 validation path.
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then
  ufw allow 80/tcp comment 'igit-us ACME and HTTP gateway'
  ufw allow 443/tcp comment 'igit-us HTTPS read-only gateway'
fi

cat > /etc/nginx/sites-available/igit-us-gateway <<EOF
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name ${DOMAIN};

    # Public data plane: only read-only IPFS path requests are admitted.
    location /ipfs/ {
        limit_except GET HEAD { deny all; }
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
        proxy_intercept_errors on;
        error_page 404 500 502 503 504 = @public_fallback;
    }
    # A transient local Kubo miss can still retrieve immutable content from the
    # public IPFS network without exposing any Kubo write/admin endpoints.
    location @public_fallback {
        internal;
        proxy_pass https://ipfs.io;
        proxy_set_header Host ipfs.io;
        proxy_ssl_server_name on;
        proxy_connect_timeout 8s;
        proxy_read_timeout 60s;
    }
    # Health represents the serving Kubo process, not merely nginx's process.
    location = /healthz {
        proxy_pass http://127.0.0.1:5001/api/v0/version;
        proxy_method POST;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
        proxy_connect_timeout 2s;
        proxy_read_timeout 3s;
    }
    location / { return 403; }
}
EOF

rm -f /etc/nginx/sites-enabled/default
ln -sfn /etc/nginx/sites-available/igit-us-gateway /etc/nginx/sites-enabled/igit-us-gateway
nginx -t
systemctl enable --now nginx

certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" \
  --no-redirect --keep-until-expiring
nginx -t
systemctl reload nginx

echo "== verify =="
curl --fail --silent --show-error --max-time 10 "https://${DOMAIN}/healthz" >/dev/null
code=$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 10 "https://${DOMAIN}/")
test "$code" = "403"
echo "US read-only gateway ready: https://${DOMAIN}"
