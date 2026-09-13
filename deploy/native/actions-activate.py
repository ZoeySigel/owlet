#!/usr/bin/python3
"""Root-owned deploy helper. Only application files change; configuration/data stay put."""
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request
import uuid

root = pathlib.Path('/opt/owlet')
releases = root / 'releases'

def switch(target):
    temporary = root / ('current-' + uuid.uuid4().hex)
    temporary.symlink_to(target, target_is_directory=True)
    temporary.replace(root / 'current')

def restart():
    subprocess.run(['systemctl', 'restart', 'owlet'], check=True)

def health():
    for _ in range(20):
        try:
            with urllib.request.urlopen('http://127.0.0.1:18080/api/health', timeout=2) as r:
                if r.status == 200:
                    return
        except Exception:
            pass
        time.sleep(1)
    raise RuntimeError('Application health check failed')

def unpack(archive, destination, sha):
    with tarfile.open(archive, 'r:gz') as source:
        members = source.getmembers()
        if len(members) > 5000 or sum(m.size for m in members) > 120 * 1024 * 1024:
            raise ValueError('Expanded release exceeds limits')
        for member in members:
            path = pathlib.PurePosixPath(member.name)
            if path.is_absolute() or '..' in path.parts or not path.parts:
                raise ValueError('Invalid archive path')
            if path.parts[0] not in ('owlet', 'web', 'REVISION'):
                raise ValueError('Unexpected release member')
            if path.parts[0] != 'web' and len(path.parts) != 1:
                raise ValueError('Unexpected nested member')
            if not (member.isfile() or member.isdir()):
                raise ValueError('Links and special files are forbidden')
        for member in members:
            target = destination / member.name
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                target.chmod(0o755)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                with source.extractfile(member) as src, target.open('wb') as dst:
                    shutil.copyfileobj(src, dst)
                target.chmod(0o755 if member.name == 'owlet' else 0o644)
    if (destination / 'REVISION').read_text().strip() != sha:
        raise ValueError('Commit SHA does not match archive')
    if (destination / 'web/revision.txt').read_text().strip() != sha:
        raise ValueError('Frontend SHA does not match archive')
    if not (destination / 'web/index.html').is_file():
        raise ValueError('Missing frontend entry')
    with (destination / 'owlet').open('rb') as binary:
        if binary.read(4) != b'\x7fELF':
            raise ValueError('Expected a Linux ELF executable')

def main():
    import fcntl
    if os.geteuid() != 0 or len(sys.argv) != 2 or not re.fullmatch(r'[0-9a-f]{40}', sys.argv[1]):
        sys.exit('Invalid deployment invocation')
    sha = sys.argv[1]
    releases.mkdir(mode=0o755, exist_ok=True)
    # Back up before acquiring the same lock used by backup.sh.
    subprocess.run(['/opt/owlet/backup.sh'], check=True)
    with open('/run/lock/owlet-maintenance.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        with tempfile.TemporaryDirectory(prefix='.incoming-', dir=releases) as work:
            work = pathlib.Path(work)
            archive = work / 'release.tar.gz'
            shutil.copyfile('/var/lib/owlet-ci/release.tar.gz', archive)
            extracted = work / 'files'
            extracted.mkdir(mode=0o755)
            unpack(archive, extracted, sha)
            destination = releases / (sha + '-' + uuid.uuid4().hex[:8])
            extracted.rename(destination)
        if not (root / 'current').is_symlink():
            baseline = releases / ('baseline-' + uuid.uuid4().hex[:8])
            baseline.mkdir(mode=0o755)
            subprocess.run(['systemctl', 'stop', 'owlet'], check=True)
            (root / 'owlet').rename(baseline / 'owlet')
            (root / 'web').rename(baseline / 'web')
            switch(baseline)
            (root / 'owlet').symlink_to('current/owlet')
            (root / 'web').symlink_to('current/web', target_is_directory=True)
        previous = (root / 'current').resolve()
        try:
            switch(destination)
            restart()
            health()
            print('DEPLOYED ' + sha)
        except Exception:
            switch(previous)
            restart()
            print('Application files rolled back; database backup was preserved', file=sys.stderr)
            raise

if __name__ == '__main__':
    main()
