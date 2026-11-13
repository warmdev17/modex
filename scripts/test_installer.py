"""Exercise release installer with fake HTTPS downloads and isolated destinations."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

INSTALLER = Path(__file__).with_name('install.sh').resolve()

class InstallerTest(unittest.TestCase):
    def run_installer(self, corrupt=False, platform='Linux', arch='x86_64', missing=False, symlink=False, http_failure=False):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            tools = root / 'tools'; tools.mkdir()
            fixture = root / 'fixture'; fixture.mkdir()
            mapped_os = {'Linux': 'linux', 'Darwin': 'darwin'}.get(platform, 'linux')
            mapped_arch = {'x86_64': 'amd64', 'amd64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(arch, 'amd64')
            name = f'modex_0.1.0-alpha.1_{mapped_os}_{mapped_arch}.tar.gz'
            archive = fixture / name
            with tarfile.open(archive, 'w:gz') as tar:
                for binary in ('modex', 'modex-host'):
                    if missing and binary == 'modex-host': continue
                    data = b'fixture binary'; item = tarfile.TarInfo(binary); item.size = len(data)
                    if symlink:
                        item.type = tarfile.SYMTYPE; item.linkname = '/nonexistent'; item.size = 0
                    tar.addfile(item, io.BytesIO(data))
            digest = '0'*64 if corrupt else hashlib.sha256(archive.read_bytes()).hexdigest()
            (fixture / 'checksums.txt').write_text(f'{digest}  {name}\n')
            (tools / 'curl').write_text('#!/bin/sh\nwhile [ "$#" -gt 0 ]; do case "$1" in https:*) url=$1;; -o) shift; output=$1;; esac; shift; done\ncp "$FIXTURE/${url##*/}" "$output"\n')
            (tools / 'uname').write_text(f'#!/bin/sh\ncase "$1" in -s) echo "{platform}";; -m) echo "{arch}";; esac\n')
            if http_failure: (tools/'curl').write_text('#!/bin/sh\nexit 22\n')
            for p in tools.iterdir(): p.chmod(0o755)
            dest = root / 'install space'; dest.mkdir(); (dest/'modex').write_text('old')
            temp = root/'tmp'; temp.mkdir()
            env = dict(os.environ, PATH=f'{tools}:'+os.environ['PATH'], FIXTURE=str(fixture), TMPDIR=str(temp))
            result = subprocess.run(['sh', str(INSTALLER), '--version', 'v0.1.0-alpha.1', '--prefix', str(dest)], env=env, capture_output=True, text=True)
            if corrupt or missing or symlink or http_failure or platform not in ('Linux', 'Darwin') or arch not in ('x86_64', 'amd64', 'aarch64', 'arm64'):
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual((dest/'modex').read_text(), 'old')
            else:
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((dest/'modex-host').read_bytes(), b'fixture binary')
            self.assertEqual(list(temp.iterdir()), [])
            self.assertFalse(list(dest.glob('.modex-install.*')))
    def test_install(self): self.run_installer()
    def test_checksum_failure_preserves_old_binary(self): self.run_installer(corrupt=True)
    def test_unsupported_os(self): self.run_installer(platform='Windows')
    def test_unsupported_cpu(self): self.run_installer(arch='mips')

    def test_missing_sibling_preserves_install(self): self.run_installer(missing=True)
    def test_symlink_archive_cannot_write_targets(self): self.run_installer(symlink=True)
    def test_http_failure_cleans_temp(self): self.run_installer(http_failure=True)

    def test_macos_arm64_mapping(self): self.run_installer(platform='Darwin', arch='arm64')
    def test_linux_arm64_mapping(self): self.run_installer(arch='aarch64')
