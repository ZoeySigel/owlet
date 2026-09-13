#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || exit 1
release=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install -m 755 "$release/owlet-tests-linux" /opt/owlet/verification-tests
if ! sudo -u postgres psql -Atqc "SELECT 1 FROM pg_database WHERE datname='owlet_verification'" | grep -q 1; then sudo -u postgres createdb owlet_verification; fi
sudo -u postgres env 'TEST_DATABASE_URL=postgres://postgres@/owlet_verification?host=/var/run/postgresql&sslmode=disable' /opt/owlet/verification-tests -test.v
install -m 755 "$release/backup.sh" /opt/owlet/backup.sh
install -m 755 "$release/monitor.sh" /opt/owlet/monitor.sh
for unit in owlet-backup.service owlet-backup.timer owlet-monitor.service owlet-monitor.timer; do
 install -m 644 "$release/$unit" "/etc/systemd/system/$unit"
done
systemctl daemon-reload
systemctl enable --now owlet-backup.timer owlet-monitor.timer
systemctl start owlet-monitor.service
systemctl start owlet-backup.service
systemctl --no-pager status owlet-backup.service || test "$?" = 3
systemctl show owlet caddy postgresql@16-main -p Id -p ActiveState -p MemoryCurrent
df -h /
