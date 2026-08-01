#!/usr/bin/env bash
# Install the restricted HK receiver. Pass the US archive sync public-key file.
set -euo pipefail

key_file="${1:?usage: durable-cid-receiver-install.sh <public-key-file>}"
install -m 0755 "$(dirname "$0")/receive-durable-cids.sh" /usr/local/sbin/receive-durable-cids.sh
id igit-sync >/dev/null 2>&1 || useradd --system --create-home --home-dir /var/lib/igit-sync --shell /bin/sh igit-sync
install -d -o igit-sync -g igit-sync -m 700 /var/lib/igit-sync/.ssh
key=$(cat "$key_file")
printf 'command="/usr/bin/sudo /usr/local/sbin/receive-durable-cids.sh",no-agent-forwarding,no-port-forwarding,no-pty,no-user-rc,no-X11-forwarding %s\n' "$key" > /var/lib/igit-sync/.ssh/authorized_keys
chown igit-sync:igit-sync /var/lib/igit-sync/.ssh/authorized_keys
chmod 600 /var/lib/igit-sync/.ssh/authorized_keys
printf 'igit-sync ALL=(root) NOPASSWD: /usr/local/sbin/receive-durable-cids.sh\n' > /etc/sudoers.d/igit-sync-receive
chmod 440 /etc/sudoers.d/igit-sync-receive
visudo -cf /etc/sudoers.d/igit-sync-receive
