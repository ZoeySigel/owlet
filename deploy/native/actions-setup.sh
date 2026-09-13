#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || exit 1
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
getent passwd owlet-ci >/dev/null || useradd --system --create-home --home-dir /var/lib/owlet-ci --shell /bin/sh owlet-ci
install -d -m 700 -o owlet-ci -g owlet-ci /var/lib/owlet-ci
install -d -m 755 -o root -g root /var/lib/owlet-ci/.ssh
install -m 755 -o root -g root "$base/actions-receive.py" /usr/local/bin/owlet-ci-receive
install -m 755 -o root -g root "$base/actions-activate.py" /usr/local/sbin/owlet-activate
printf 'restrict,command="/usr/local/bin/owlet-ci-receive" %s\n' "$(cat "$base/actions.pub")" > /var/lib/owlet-ci/.ssh/authorized_keys
chmod 644 /var/lib/owlet-ci/.ssh/authorized_keys
chown root:root /var/lib/owlet-ci/.ssh/authorized_keys
printf 'owlet-ci ALL=(root) NOPASSWD: /usr/local/sbin/owlet-activate *\n' > /etc/sudoers.d/owlet-ci
chmod 440 /etc/sudoers.d/owlet-ci
visudo -cf /etc/sudoers.d/owlet-ci
install -m 755 "$base/backup.sh" /opt/owlet/backup.sh
echo 'Restricted Actions receiver installed; existing application was not restarted.'
