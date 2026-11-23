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
    parser.add_argument('--connections', action='store_true')
    parser.add_argument('--continuity', action='store_true')
    parser.add_argument('--attachments', action='store_true')
    parser.add_argument('--journal-recovery', action='store_true')
    parser.add_argument('--stale-instance', action='store_true', help='Seed a previous supervisor identity in the journal recovery fixture.')
    parser.add_argument('--serial', default='emulator-5554')
    args = parser.parse_args()
    if sum((args.attachments, args.continuity, args.journal_recovery, args.connections)) > 1:
        parser.error('choose one fixture mode')
    if args.stale_instance and not args.journal_recovery:
        parser.error('--stale-instance requires --journal-recovery')
    host = str(args.host.resolve())
    adb = [args.adb, '-s', args.serial]

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
        second_started = False
        second_state = root / "second-state"
        remote_key = '/data/local/tmp/modex-journal-' + root.name + '.key'
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
            if args.journal_recovery:
                (root / 'journal-recovery').touch()
            fake = root / 'fake-codex'
            fake.write_text('''#!/usr/bin/env python3
import json,sys
from pathlib import Path
root=Path(sys.argv[0]).parent
recovery=(root/'journal-recovery').exists()
flooded=False
for line in sys.stdin:
 r=json.loads(line)
 if recovery:
  with (root/'calls.log').open('a') as log:
   log.write(json.dumps({'method':r.get('method'),'id':r.get('id'),'result':r.get('result')})+'\\n')
  if 'method' not in r:
   assert r.get('id')==70001 and r.get('result')=={'decision':'accept'}
   continue
 if r.get('method')=='initialized':continue
 thread={'id':'fixture-thread','cwd':sys.argv[0].rsplit('/',1)[0],'modelProvider':'fixture','model':'fixture','reasoningEffort':'low','turns':[{'status':'completed','items':[{'id':'u','type':'userMessage','content':[{'type':'text','text':'Previous question'},{'type':'localImage','path':sys.argv[0].rsplit('/',1)[0]+'/image.png'},{'type':'text','text':'Attached files selected by the user (JSON paths):\\n'+json.dumps([{'name':'notes.txt','path':sys.argv[0].rsplit('/',1)[0]+'/notes.txt'}])}]},{'id':'a','type':'agentMessage','text':'Stored assistant reply'}]}]}
 method=r.get('method')
 if recovery:
  thread['turns'].append({'id':'running-turn','status':'inProgress','items':[]})
  thread['status']={'type':'active'}
 result={'config':{'model':'fixture','model_provider':'fixture','model_reasoning_effort':'low'}} if method=='config/read' else {'thread':thread} if method in ('thread/read','thread/resume') else {'data':[thread],'nextCursor':None} if method=='thread/list' else {'data':list(reversed(thread['turns'])),'nextCursor':None} if method=='thread/turns/list' else {}
 if method=='turn/start':
  inputs=r['params']['input']
  assert any(p.get('type')=='localImage' and p['path'].endswith('image.png') for p in inputs)
  assert any(p.get('type')=='text' and 'report*?.txt' in p.get('text','') for p in inputs)
  result={'turn':{'id':'attachment-fixture'}}
 print(json.dumps({'id':r['id'],'result':result}),flush=True)
 if recovery and method=='config/read' and not flooded:
  flooded=True
  print(json.dumps({'id':70001,'method':'item/commandExecution/requestApproval','params':{'threadId':'fixture-thread','turnId':'running-turn','itemId':'approval','command':'printf JOURNAL_REPLAY_OK','availableDecisions':['accept','decline']}}),flush=True)
  print(json.dumps({'id':70002,'method':'item/tool/requestUserInput','params':{'threadId':'fixture-thread','turnId':'running-turn','itemId':'question','questions':[{'id':'choice','header':'Choose','question':'Continue?','options':[{'label':'Yes','description':'Continue fixture'},{'label':'No','description':'Keep waiting'}]}]}}),flush=True)
  for i in range(1300):
   print(json.dumps({'method':'item/agentMessage/delta','params':{'threadId':'fixture-thread','turnId':'running-turn','itemId':'stream','delta':'x'}}),flush=True)
''')
            fake.chmod(0o700)
            host_args = [host, '--background', '--workspace', str(root), '--state-dir', str(state), '--codex', str(fake)]
            if args.journal_recovery:
                (root / 'codex-home').mkdir(mode=0o700)
                host_args += ['--codex-home', str(root / 'codex-home')]
            run(host_args)
            started = True
            if args.connections:
                run([host, "--background", "--workspace", str(root), "--state-dir", str(second_state), "--codex", str(fake)])
                second_started = True
            if args.journal_recovery:
                from live_supervisor_smoke import AttachedRPC
                warmup = AttachedRPC(host, state, root)
                try:
                    warmup.call('status')
                    warmup.call('status')
                    assert warmup.cursor > 1024, 'fixture did not expire the initial cursor'
                finally:
                    warmup.close()
                import json
                instance = json.loads((state / 'endpoint.json').read_text())['instance']
            if args.journal_recovery or args.connections:
                command = shlex.join(['sh', '-c', 'umask 077; cat > "$1"', 'modex-journal', remote_key])
                run(adb + ['shell', '-T', command], input=(root / 'client').read_bytes())
            else:
                run(adb + ['shell', 'run-as', 'dev.modex', 'mkdir', '-p', 'files'])
                run(adb + ['shell', 'run-as', 'dev.modex', 'sh', '-c', '"cat > files/smoke-key"'], input=(root / 'client').read_bytes())
            blob = base64.b64decode((root / 'host.pub').read_text().split()[1])
            pin = 'SHA256:' + base64.b64encode(hashlib.sha256(blob).digest()).decode().rstrip('=')
            stages = ('initial', 'restore') if args.continuity or args.connections else ('transport',)
            for stage in stages:
                extras = ['-e', 'preview', 'continuity', '-e', 'stage', stage] if args.continuity else ['-e','preview','attachments','-e','root',str(root)] if args.attachments else []
                if args.connections:
                    extras = ["-e", "preview", "connection-management", "-e", "stage", stage, "-e", "state2", str(second_state), "-e", "keyPath", remote_key]
                if args.journal_recovery:
                    saved_instance = 'previous-supervisor' if args.stale_instance else instance
                    extras = ['-e', 'preview', 'journal-recovery', '-e', 'instance', saved_instance, '-e', 'keyPath', remote_key]
                result = run(adb + ['shell', shlex.join(['am', 'instrument', '-w'] + extras + ['-e', 'port', str(port), '-e', 'user', getpass.getuser(), '-e', 'pin', pin, '-e', 'binary', host, '-e', 'state', str(state), 'dev.modex.test/dev.modex.TransportSmoke'])])
                output = result.stdout.decode()
                if 'PASS:' not in output or 'INSTRUMENTATION_CODE: -1' not in output:
                    raise RuntimeError('Android transport smoke failed: ' + output)
                print(output.strip())
                if args.journal_recovery:
                    assert json.loads((state / 'endpoint.json').read_text())['instance'] == instance, 'supervisor restarted'
                    calls = [json.loads(line) for line in (root / 'calls.log').read_text().splitlines()]
                    assert sum(c['method'] == 'initialize' for c in calls) == 1, 'Codex backend restarted'
                    assert not any(c['method'] == 'turn/start' for c in calls), 'uncertain prompt was replayed'
                    answers = [c for c in calls if c['method'] is None]
                    assert len(answers) == 1 and answers[0]['id'] == 70001, 'decision was replayed or answered automatically'
                if args.attachments:
                    uploads = list((state / 'attachments').glob('*/*'))
                    assert len(uploads) == 2, 'partial or missing upload'
                    assert all(p.is_file() and p.stat().st_mode & 0o777 == 0o600 for p in uploads)
                    assert all(p.parent.stat().st_mode & 0o777 == 0o700 for p in uploads)
                    assert sorted(p.read_text() for p in uploads) == ['document-provider-stream','mobile-upload']
                if args.continuity or args.connections:
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
                if args.journal_recovery or args.connections:
                    run(adb + ['shell', 'rm', '-f', remote_key])
                else:
                    run(adb + ['shell', 'run-as', 'dev.modex', 'rm', '-f', 'files/smoke-key'])
            finally:
                try:
                    if second_started:
                        run([host, "--stop", "--state-dir", str(second_state)])
                    if started:
                        run([host, '--stop', '--state-dir', str(state)])
                finally:
                    sshd.terminate()
                    sshd.wait(timeout=5)
                    sshd_log.close()


if __name__ == '__main__':
    main()
