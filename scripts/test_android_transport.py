"""Opt-in emulator SSH integration with ephemeral Ed25519 keys and fake provider."""
import argparse
import base64
import struct
import zlib
import getpass
import hashlib
from pathlib import Path
import shutil
import shlex
import socket
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', required=True, type=Path)
    parser.add_argument('--adb', default='adb')
    parser.add_argument('--continuity', action='store_true')
    parser.add_argument('--attachments', action='store_true')
    args = parser.parse_args()
    if args.attachments and args.continuity:
        parser.error('choose one fixture mode')
    host = str(args.host.resolve())
    adb = [args.adb, '-s', 'emulator-5554']

    def run(command, **kwargs):
        return subprocess.run(command, check=True, capture_output=True, timeout=45, **kwargs)

    with tempfile.TemporaryDirectory(prefix='modex-android-ssh-') as directory:
        root = Path(directory)
        for name in ('host', 'client'):
            run(['ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(root / name)])
        def png_chunk(kind, data):
            return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
        (root / 'image.png').write_bytes(b'\x89PNG\r\n\x1a\n' + png_chunk(b'IHDR',struct.pack('>2I5B',1,1,8,2,0,0,0)) + png_chunk(b'IDAT',zlib.compress(b'\x00\xff\x00\x00')) + png_chunk(b'IEND',b''))
        (root / 'notes.txt').write_text('Remote file fixture')
        (root / 'literal*? folder').mkdir()
        (root / 'literal*? folder' / 'inside.txt').write_text('literal path')
        (root / 'authorized_keys').write_bytes((root / 'client.pub').read_bytes())
        with socket.socket() as reserve:
            reserve.bind(('127.0.0.1', 0))
            port = reserve.getsockname()[1]
        config = root / 'sshd_config'
        config.write_text(f'''Port {port}
ListenAddress 127.0.0.1
HostKey {root / 'host'}
PidFile {root / 'pid'}
AuthorizedKeysFile {root / 'authorized_keys'}
StrictModes no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
UsePAM no
PermitRootLogin prohibit-password
AllowUsers {getpass.getuser()}
Subsystem sftp internal-sftp
''')
        # Emulator reaches the host through 10.0.2.2; keys are ephemeral and passwords disabled.
        sshd_log = (root / 'sshd.log').open('w+')
        sshd = subprocess.Popen([shutil.which('sshd'), '-D', '-e', '-f', str(config)], stdout=subprocess.DEVNULL, stderr=sshd_log)
        state = root / ('state[*?] space' if args.attachments else 'state')
        started = False
        try:
            deadline = time.monotonic() + 5
            while True:
                if sshd.poll() is not None:
                    raise RuntimeError('fixture sshd exited')
                try:
                    with socket.create_connection(('127.0.0.1', port), timeout=.1):
                        break
                except OSError:
                    if time.monotonic() > deadline:
                        raise RuntimeError('fixture sshd timeout')
                    time.sleep(.05)
            fake = root / 'fake-codex'
            fake.write_text('''#!/usr/bin/env python3
import json,sys
for line in sys.stdin:
 r=json.loads(line)
 if r.get('method')=='initialized':continue
 thread={'id':'fixture-thread','cwd':sys.argv[0].rsplit('/',1)[0],'modelProvider':'fixture','model':'fixture','reasoningEffort':'low','turns':[{'status':'completed','items':[{'id':'u','type':'userMessage','content':[{'type':'text','text':'Previous question'},{'type':'localImage','path':sys.argv[0].rsplit('/',1)[0]+'/image.png'},{'type':'text','text':'Attached files selected by the user (JSON paths):\\n'+json.dumps([{'name':'notes.txt','path':sys.argv[0].rsplit('/',1)[0]+'/notes.txt'}])}]},{'id':'a','type':'agentMessage','text':'Stored assistant reply'}]}]}
 method=r.get('method')
 result={'config':{'model':'fixture','model_provider':'fixture','model_reasoning_effort':'low'}} if method=='config/read' else {'thread':thread} if method in ('thread/read','thread/resume') else {'data':[thread],'nextCursor':None} if method=='thread/list' else {'data':list(reversed(thread['turns'])),'nextCursor':None} if method=='thread/turns/list' else {}
 if method=='turn/start':
  inputs=r['params']['input']
  assert any(p.get('type')=='localImage' and p['path'].endswith('image.png') for p in inputs)
  assert any(p.get('type')=='text' and 'report*?.txt' in p.get('text','') for p in inputs)
  result={'turn':{'id':'attachment-fixture'}}
 print(json.dumps({'id':r['id'],'result':result}),flush=True)
''')
            fake.chmod(0o700)
            run([host, '--background', '--workspace', str(root), '--state-dir', str(state), '--codex', str(fake)])
            started = True
            run(adb + ['shell', 'run-as', 'dev.modex', 'mkdir', '-p', 'files'])
            run(adb + ['shell', 'run-as', 'dev.modex', 'sh', '-c', '"cat > files/smoke-key"'], input=(root / 'client').read_bytes())
            blob = base64.b64decode((root / 'host.pub').read_text().split()[1])
            pin = 'SHA256:' + base64.b64encode(hashlib.sha256(blob).digest()).decode().rstrip('=')
            stages = ('initial', 'restore') if args.continuity else ('transport',)
            for stage in stages:
                extras = ['-e', 'preview', 'continuity', '-e', 'stage', stage] if args.continuity else ['-e','preview','attachments','-e','root',str(root)] if args.attachments else []
                result = run(adb + ['shell', shlex.join(['am', 'instrument', '-w'] + extras + ['-e', 'port', str(port), '-e', 'user', getpass.getuser(), '-e', 'pin', pin, '-e', 'binary', host, '-e', 'state', str(state), 'dev.modex.test/dev.modex.TransportSmoke'])])
                output = result.stdout.decode()
                if 'PASS:' not in output or 'INSTRUMENTATION_CODE: -1' not in output:
                    raise RuntimeError('Android transport smoke failed: ' + output)
                print(output.strip())
                if args.attachments:
                    uploads = list((state / 'attachments').glob('*/*'))
                    assert len(uploads) == 2, 'partial or missing upload'
                    assert all(p.is_file() and p.stat().st_mode & 0o777 == 0o600 for p in uploads)
                    assert all(p.parent.stat().st_mode & 0o777 == 0o700 for p in uploads)
                    assert sorted(p.read_text() for p in uploads) == ['document-provider-stream','mobile-upload']
                if args.continuity:
                    run(adb + ['shell', 'am', 'force-stop', 'dev.modex'])
                    time.sleep(.4)
        except Exception:
            sshd_log.flush(); sshd_log.seek(0)
            print('Fixture SSH diagnostics:')
            for line in sshd_log.read().splitlines()[-12:]:
                print(line)
            raise
        finally:
            try:
                run(adb + ['shell', 'am', 'force-stop', 'dev.modex'])
                run(adb + ['shell', 'run-as', 'dev.modex', 'rm', '-f', 'files/smoke-key'])
            finally:
                try:
                    if started:
                        run([host, '--stop', '--state-dir', str(state)])
                finally:
                    sshd.terminate()
                    sshd.wait(timeout=5)
                    sshd_log.close()


if __name__ == '__main__':
    main()
