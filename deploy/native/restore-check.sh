#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || exit 1
stamp=${1:?Pass a backup timestamp}
printf '%s' "$stamp" | grep -Eq '^20[0-9]{6}T[0-9]{6}Z$' || exit 1
backup="/var/backups/owlet/$stamp"
target="/var/lib/owlet-restore-check/$stamp"
database="owlet_restore_$(printf '%s' "$stamp" | tr 'TZ' '__')"
[ -d "$backup" ] && [ ! -e "$target" ] || { echo 'Missing backup or restore destination already exists'; exit 1; }
(cd "$backup" && sha256sum -c SHA256SUMS)
install -d -m 755 /var/lib/owlet-restore-check
install -d -m 700 -o owlet -g owlet "$target"
tar -C "$target" -xzf "$backup/assets.tgz"
(cd "$target" && sha256sum -c "$backup/assets.sha256")
sudo -u postgres createdb -O owlet "$database"
sudo -u postgres pg_restore --exit-on-error --no-owner --role=owlet -d "$database" < "$backup/database.dump"
sudo -u postgres psql -At -d "$database" -c "SELECT 'documents',count(*) FROM documents UNION ALL SELECT 'assets',count(*) FROM assets UNION ALL SELECT 'versions',count(*) FROM versions UNION ALL SELECT 'jobs',count(*) FROM jobs UNION ALL SELECT 'quotas',count(*) FROM quotas UNION ALL SELECT 'ledger',count(*) FROM ledger ORDER BY 1" > "$target/restored-counts.txt"
diff -u "$backup/counts.txt" "$target/restored-counts.txt"
# Start an isolated restored app, with no API key and no paid-call authorization.
systemd-run --unit=owlet-restore-check --property=User=owlet --property=Group=owlet --property=MemoryMax=384M \
 --setenv="DATABASE_URL=postgres://owlet@/$database?host=/var/run/postgresql&sslmode=disable" \
 --setenv=LISTEN_ADDR=127.0.0.1:18082 --setenv=PUBLIC_ORIGIN=http://localhost:15173 \
 --setenv="DATA_DIR=$target/assets" --setenv=MODEL_MODE=mock --setenv=PAID_CALLS_VERIFIED=false /opt/owlet/owlet
sleep 2
curl --fail --silent http://127.0.0.1:18082/api/health
printf '\nRESTORE_OK database=%s directory=%s\n' "$database" "$target"
