"""Local build/install contract; all destinations and shell profiles are isolated."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().with_name('build.sh')

@unittest.skipUnless(os.name == 'posix', 'POSIX local installer')
class LocalBuildTests(unittest.TestCase):
    def setUp(self):
        self.fixture = tempfile.TemporaryDirectory(prefix='modex-local-build-')
        self.addCleanup(self.fixture.cleanup)
        self.root = Path(self.fixture.name)
        tools = self.root / 'tools'
        tools.mkdir()
        go = tools / 'go'
        go.write_text('''#!/bin/sh
set -eu
modex_output=
while [ "$#" -gt 0 ]; do
 case "$1" in
  -o) shift; modex_output=$1 ;;
  ./cmd/modex) if [ "${MODEX_BUILD_FAIL:-}" = 1 ]; then exit 9; fi ;;
 esac
 shift
done
printf '#!/bin/sh\\nprintf fixture\\n' > "$modex_output"
chmod 755 "$modex_output"
''')
        go.chmod(0o700)
        self.prefix = self.root / "prefix with ' quote"
        self.host = self.root / 'phone' / 'modex-host'
        self.env = dict(os.environ, HOME=str(self.root), XDG_CONFIG_HOME=str(self.root / '.config'),
                        PREFIX=str(self.prefix), HOST_BINARY=str(self.host), SHELL='/bin/bash',
                        PATH=str(tools) + os.pathsep + '/usr/bin:/bin')

    def build(self):
        return subprocess.run(['sh', str(SCRIPT)], env=self.env, text=True, capture_output=True, timeout=10)

    def test_installs_both_commands_and_phone_binary_with_idempotent_quoted_path(self):
        (self.root / '.bashrc').write_text('# preserve my shell configuration\n')
        for _ in range(2):
            result = self.build()
            self.assertEqual(result.returncode, 0, result.stderr)
        for binary in [self.prefix / 'bin/modex', self.prefix / 'bin/modex-host', self.host]:
            self.assertTrue(os.access(binary, os.X_OK), binary)
        self.assertEqual(self.host.read_bytes(), (self.prefix / 'bin/modex-host').read_bytes())
        profile = (self.root / '.bashrc').read_text()
        self.assertTrue(profile.startswith('# preserve my shell configuration\n'))
        self.assertEqual(profile.count('export PATH='), 1)
        resolved = subprocess.run(['/bin/sh', '-c', '. "$HOME/.profile"; command -v modex'], env=self.env, text=True, capture_output=True, check=True)
        self.assertEqual(resolved.stdout.strip(), str(self.prefix / 'bin/modex'))

    def test_failed_second_build_keeps_previous_binaries_and_profiles(self):
        for binary in [self.prefix / 'bin/modex', self.prefix / 'bin/modex-host', self.host]:
            binary.parent.mkdir(parents=True, exist_ok=True)
            binary.write_text('old binary')
        self.env['MODEX_BUILD_FAIL'] = '1'
        result = self.build()
        self.assertNotEqual(result.returncode, 0)
        for binary in [self.prefix / 'bin/modex', self.prefix / 'bin/modex-host', self.host]:
            self.assertEqual(binary.read_text(), 'old binary')
        self.assertFalse((self.root / '.profile').exists())

    def test_fish_path_and_existing_path_do_not_touch_other_shell_profiles(self):
        self.env['SHELL'] = '/bin/fish'
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stderr)
        conf = self.root / '.config/fish/conf.d/modex-path.fish'
        self.assertIn('fish_add_path', conf.read_text())
        self.assertFalse((self.root / '.bashrc').exists())
        if shutil.which('fish'):
            checked = subprocess.run([shutil.which('fish'), '-n', str(conf)], capture_output=True, text=True)
            self.assertEqual(checked.returncode, 0, checked.stderr)
        conf.unlink()
        self.env['PATH'] = str(self.prefix / 'bin') + os.pathsep + self.env['PATH']
        self.assertEqual(self.build().returncode, 0)
        self.assertFalse(conf.exists())
