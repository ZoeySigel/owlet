#!/bin/sh
set -eu
# Run as root with a release directory containing owlet, web/, and native/.
[ "$(id -u)" = 0 ] || { echo 'Run with sudo'; exit 1; }
release=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
getent passwd owlet >/dev/null || useradd --system --home-dir /var/lib/owlet --shell /usr/sbin/nologin owlet
install -d -m 755 /opt/owlet /opt/owlet/web /etc/owlet
install -d -m 700 -o owlet -g owlet /var/lib/owlet /var/lib/owlet/assets
install -m 755 "$release/owlet" /opt/owlet/owlet
cp -R "$release/web/." /opt/owlet/web/
chmod -R a+rX /opt/owlet/web
if ! sudo -u postgres psql -Atqc "SELECT 1 FROM pg_roles WHERE rolname='owlet'" | grep -q 1; then
 sudo -u postgres createuser owlet
fi
if ! sudo -u postgres psql -Atqc "SELECT 1 FROM pg_database WHERE datname='owlet'" | grep -q 1; then
 sudo -u postgres createdb -O owlet owlet
fi
if [ ! -f /etc/owlet/app.env ]; then
 umask 077
 admin_password=$(openssl rand -hex 18)
 printf '%s\n' "$admin_password" > /etc/owlet/admin-initial.txt
 cat > /etc/owlet/app.env <<EOF
DATABASE_URL=postgres://owlet@/owlet?host=/var/run/postgresql&sslmode=disable
LISTEN_ADDR=127.0.0.1:18080
PUBLIC_ORIGIN=http://localhost:15173
COOKIE_SECURE=false
DATA_DIR=/var/lib/owlet/assets
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$admin_password
MODEL_MODE=mock
PAID_CALLS_VERIFIED=false
EOF
fi
install -m 644 "$release/native/owlet.service" /etc/systemd/system/owlet.service
install -d /etc/systemd/system/postgresql@16-main.service.d
printf '[Service]\nMemoryMax=512M\n' > /etc/systemd/system/postgresql@16-main.service.d/owlet-memory.conf
install -d /etc/systemd/system/caddy.service.d
printf '[Service]\nMemoryMax=128M\n' > /etc/systemd/system/caddy.service.d/owlet-memory.conf
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "ALTER SYSTEM SET shared_buffers='128MB'" -c "ALTER SYSTEM SET max_connections='30'" -c "ALTER SYSTEM SET work_mem='4MB'"
if [ ! -f /etc/caddy/Caddyfile.before-owlet ]; then cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.before-owlet; fi
install -m 644 "$release/native/Caddyfile.staging" /etc/caddy/Caddyfile
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl daemon-reload
systemctl restart postgresql@16-main
systemctl enable --now owlet caddy
systemctl restart owlet caddy
echo 'Owlet installed in loopback-only staging mode; use an SSH tunnel to verify.'
