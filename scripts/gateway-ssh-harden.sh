#!/usr/bin/env bash
# gateway-ssh-harden: disable SSH password login (key-only) on the HK node.
# Safe by design: writes a drop-in, validates with `sshd -t` BEFORE applying,
# reloads (live sessions unaffected). PREREQ: your public key is already in
# root's authorized_keys (verified). Run as root ON THE SERVER.
set -euo pipefail

DROPIN=/etc/ssh/sshd_config.d/99-igit-hardening.conf
cat > "$DROPIN" <<'EOF'
# igit: key-only root login (drop-in wins via first-match after early Include)
PasswordAuthentication no
PermitRootLogin prohibit-password
KbdInteractiveAuthentication no
EOF
chmod 644 "$DROPIN"

echo "== validate sshd config =="
sshd -t
echo "syntax OK"

echo "== reload ssh (existing sessions keep working) =="
systemctl reload ssh

echo "== effective auth settings now =="
sshd -T | grep -i -e passwordauthentication -e permitrootlogin -e pubkeyauthentication -e kbdinteractive
