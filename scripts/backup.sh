#!/usr/bin/env sh
# Run from repository root. Briefly stops writes for a consistent DB/assets pair.
set -eu
umask 077
mkdir -p backups
stamp=$(date -u +%Y%m%dT%H%M%SZ)
docker compose stop app
trap 'docker compose start app >/dev/null' EXIT INT TERM
docker compose exec -T db pg_dump -U owlet -d owlet -Fc > "backups/$stamp.dump"
docker compose run --rm --no-deps --entrypoint tar app -C /data -czf - . > "backups/$stamp.assets.tgz"
echo "Backup complete: $stamp. Copy encrypted backups off-host; keep no more than 7 days."
