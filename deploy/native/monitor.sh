#!/bin/sh
set -eu
free=$(df --output=avail -B1 /var/lib/owlet | tail -1 | tr -d ' ')
if [ "$free" -lt 2147483648 ]; then echo 'Owlet disk available below 2 GiB' >&2; exit 1; fi
curl --fail --silent --max-time 10 http://127.0.0.1:18080/api/health >/dev/null
