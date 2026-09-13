import importlib.util
import io
import os
import pathlib
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('activate', pathlib.Path(__file__).with_name('actions-activate.py'))
activate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(activate)
SHA = 'a' * 40

class ReleaseArchiveTests(unittest.TestCase):
    def archive(self, path, extra=None, revision=SHA):
        with tarfile.open(path, 'w:gz') as tar:
            for name, data in {'owlet': b'\x7fELFdemo', 'web/index.html': b'<html/>', 'web/revision.txt': revision.encode(), 'REVISION': revision.encode()}.items():
                info = tarfile.TarInfo(name)
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
            if extra:
                tar.addfile(extra, io.BytesIO(b''))

    def test_accepts_release_and_preserves_executable_mode(self):
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            self.archive(root / 'release.tgz')
            target = root / 'out'
            target.mkdir()
            activate.unpack(root / 'release.tgz', target, SHA)
            self.assertEqual((target / 'REVISION').read_text(), SHA)
            if os.name == 'posix':
                self.assertTrue((target / 'owlet').stat().st_mode & 0o100)

    def test_rejects_paths_links_and_unexpected_files(self):
        for name, kind in [('../escaped', tarfile.REGTYPE), ('/tmp/escaped', tarfile.REGTYPE), ('web/link', tarfile.SYMTYPE), ('web/hard', tarfile.LNKTYPE), ('app.env', tarfile.REGTYPE)]:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as root:
                root = pathlib.Path(root)
                extra = tarfile.TarInfo(name)
                extra.type = kind
                extra.linkname = '/etc/passwd'
                self.archive(root / 'release.tgz', extra)
                with self.assertRaises(ValueError):
                    activate.unpack(root / 'release.tgz', root / 'out', SHA)

    def test_rejects_revision_mismatch(self):
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            self.archive(root / 'release.tgz', revision='b' * 40)
            with self.assertRaises(ValueError):
                activate.unpack(root / 'release.tgz', root / 'out', SHA)

if __name__ == '__main__':
    unittest.main()
