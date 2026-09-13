#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || exit 1
release=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
expected=${OWLET_EXPECTED_IP:?Set the expected server IPv4 address}
getent ahostsv4 owl-et.me | awk '{print $1}' | grep -Fxq "$expected" || { echo 'DNS does not match target server'; exit 1; }
caddy validate --config "$release/Caddyfile.production" --adapter caddyfile
umask 077
if [ ! -f /etc/owlet/app.env.before-https ]; then cp /etc/owlet/app.env /etc/owlet/app.env.before-https; fi
if [ ! -f /etc/caddy/Caddyfile.before-https ]; then cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.before-https; fi
sed -i 's|^PUBLIC_ORIGIN=.*|PUBLIC_ORIGIN=https://owl-et.me|;s|^COOKIE_SECURE=.*|COOKIE_SECURE=true|' /etc/owlet/app.env
install -m 644 "$release/Caddyfile.production" /etc/caddy/Caddyfile
systemctl restart owlet
systemctl reload caddy
echo 'Canonical origin set to https://owl-et.me; paid model calls remain disabled.'
