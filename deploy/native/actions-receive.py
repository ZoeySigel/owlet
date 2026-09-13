#!/usr/bin/python3
"""Forced SSH command: accept a bounded release archive, never a shell command."""
import fcntl
import os
import pathlib
import re
import subprocess
import sys
import tempfile

command = os.environ.get('SSH_ORIGINAL_COMMAND', '')
if not re.fullmatch(r'deploy [0-9a-f]{40}', command):
    sys.exit('Only deploy <40-character commit SHA> is permitted')
sha = command.split()[1]
home = pathlib.Path('/var/lib/owlet-ci')
with (home / 'deploy.lock').open('a') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX)
    fd, name = tempfile.mkstemp(prefix='upload-', dir=home)
    try:
        size = 0
        with os.fdopen(fd, 'wb') as target:
            while chunk := sys.stdin.buffer.read(65536):
                size += len(chunk)
                if size > 80 * 1024 * 1024:
                    sys.exit('Archive exceeds 80 MiB')
                target.write(chunk)
        if size == 0:
            sys.exit('Empty release rejected')
        os.replace(name, home / 'release.tar.gz')
        result = subprocess.run(['sudo', '-n', '/usr/local/sbin/owlet-activate', sha])
        sys.exit(result.returncode)
    finally:
        pathlib.Path(name).unlink(missing_ok=True)
