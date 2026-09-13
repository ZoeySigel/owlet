#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || exit 1
exec 9>/run/lock/owlet-maintenance.lock
flock -x 9
umask 077
base=/var/backups/owlet
install -d -m 700 "$base"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dest="$base/$stamp"
mkdir "$dest"
systemctl stop owlet
trap 'systemctl start owlet' EXIT
sudo -u postgres pg_dump -Fc owlet > "$dest/database.dump"
tar -C /var/lib/owlet -czf "$dest/assets.tgz" assets
(cd /var/lib/owlet && find assets -type f -exec sha256sum {} \;) > "$dest/assets.sha256"
sudo -u postgres psql -At -d owlet -c "SELECT 'documents',count(*) FROM documents UNION ALL SELECT 'assets',count(*) FROM assets UNION ALL SELECT 'versions',count(*) FROM versions UNION ALL SELECT 'jobs',count(*) FROM jobs UNION ALL SELECT 'quotas',count(*) FROM quotas UNION ALL SELECT 'ledger',count(*) FROM ledger ORDER BY 1" > "$dest/counts.txt"
(cd "$dest" && sha256sum database.dump assets.tgz > SHA256SUMS)
echo "Backup complete: $dest"
# Delete only completed timestamp directories belonging to this backup root.
find "$base" -mindepth 1 -maxdepth 1 -type d -name '20??????T??????Z' -mtime +6 -exec rm -rf -- {} +
