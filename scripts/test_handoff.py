"""Opt-in same-thread ownership tests with independent fake app-server processes."""
import json
import os
from pathlib import Path
import sys
import subprocess
import tempfile
import time
import unittest

from live_provider_smoke import RPC

BINARY = os.environ.get('MODEX_HOST_BINARY')
FAKE = r'''import fcntl,json,os,sys,threading,time,uuid
root=os.getcwd();loaded={};locks={};pending={};output=threading.Lock()
def send(value):
 with output:print(json.dumps(value),flush=True)
def read(id):
 if id in loaded:return loaded[id]
 return json.load(open(root+'/'+id+'.json'))
def acquire(id):
 if id not in locks:
  f=open(root+'/'+id+'.lock','a')
  try:fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB)
  except BlockingIOError:f.close();raise RuntimeError('active writer')
  locks[id]=f
 loaded[id]=read(id)
def finish(id):
 t=loaded[id];turn=t['turns'][-1];turn['status']='completed';t['status']={'type':'idle'}
 with open(root+'/'+id+'.json','w') as f:json.dump(t,f)
 send({'method':'turn/completed','params':{'threadId':id,'turn':turn}})
def delayed(id):
 while not os.path.exists(root+'/'+id+'.finish'):time.sleep(.02)
 finish(id)
for line in sys.stdin:
 r=json.loads(line);m=r.get('method');i=r.get('id');p=r.get('params',{});id=p.get('threadId')
 try:
  if m=='initialized':continue
  if m is None:
   entry=pending.pop(i);id=entry[0]
   if entry[1] is not None:send({'id':entry[1],'result':{'turn':loaded[id]['turns'][-1]}})
   finish(id);continue
  if m=='initialize':result={}
  elif m=='config/read':result={'config':{'model':'fixture','model_provider':'fixture'}}
  elif m=='thread/start':
   id=str(uuid.uuid4());loaded[id]={'id':id,'cwd':root,'modelProvider':'fixture','status':{'type':'idle'},'turns':[]}
   acquire(id);result={'thread':loaded[id],'model':'fixture'}
  elif m=='thread/read':result={'thread':read(id)}
  elif m=='thread/resume':acquire(id);result={'thread':loaded[id],'model':'fixture'}
  elif m=='thread/unsubscribe':result={'status':'unsubscribed'}
  elif m=='turn/start':
   acquire(id);t=loaded[id];turn={'id':str(uuid.uuid4()),'status':'inProgress','items':[]};t['turns'].append(turn);t['status']={'type':'active'}
   with open(root+'/'+id+'.json','w') as f:json.dump(t,f)
   text=p['input'][0]['text']
   if text=='uncertain':
    send({'id':i,'error':{'code':-32603,'message':'uncertain dispatch'}})
    continue
   if text!='approval-early':send({'id':i,'result':{'turn':turn}})
   if text in ('approval','approval-early'):
    pending[1]=(id,i if text=='approval-early' else None)
    send({'id':1,'method':'item/commandExecution/requestApproval','params':{'threadId':id,'turnId':turn['id'],'itemId':'cmd','command':'echo fixture','availableDecisions':['accept','decline']}})
   elif text=='long':threading.Thread(target=delayed,args=(id,),daemon=True).start()
   else:finish(id)
   continue
  else:result={}
  send({'id':i,'result':result})
 except Exception:send({'id':i,'error':{'code':-32600,'message':'fixture request rejected: active writer or missing thread'}})
'''


@unittest.skipUnless(BINARY and os.name == 'posix', 'requires built host and Unix file locks')
class SessionHandoffTests(unittest.TestCase):
    def setUp(self):
        self.fixture = tempfile.TemporaryDirectory(prefix='modex-handoff-test-')
        self.root = Path(self.fixture.name)
        self.fake = self.root / 'fake-codex'
        self.fake.write_text('#!' + sys.executable + '\n' + FAKE)
        self.fake.chmod(0o700)
        self.host = RPC([str(Path(BINARY).resolve()), '--serve', '--workspace', str(self.root), '--codex', str(self.fake)], self.root)
        self.peer = RPC([str(self.fake)], self.root)
        self.peer.call('initialize', {'clientInfo': {'name': 'fixture', 'version': '1'}})
        self.peer.send({'method': 'initialized'})

    def tearDown(self):
        self.host.close()
        self.peer.close()
        self.fixture.cleanup()

    def create(self):
        return self.host.call('threads/start')['thread']['id']

    def start(self, thread, text):
        return self.host.call('turn/start', {'thread_id': thread, 'text': text})

    def event(self, method, thread=None):
        deadline = time.monotonic() + 5
        while True:
            frame = self.host.events.popleft() if self.host.events else self.host.receive(deadline)
            if frame.get('event') == method and (thread is None or frame.get('params', {}).get('threadId') == thread):
                return frame['params']

    def acquire(self, thread):
        deadline = time.monotonic() + 4
        while True:
            try:
                result = self.peer.call('thread/resume', {'threadId': thread})
                self.assertEqual(thread, result['thread']['id'])
                return
            except RuntimeError:
                if time.monotonic() > deadline:
                    raise
                time.sleep(.02)

    def test_supervisor_socket_detach_releases_only_after_running_turn_finishes(self):
        from live_supervisor_smoke import AttachedRPC
        state = self.root / 'supervisor'
        control = [str(Path(BINARY).resolve()), '--state-dir', str(state)]
        subprocess.run(control + ['--background', '--workspace', str(self.root), '--codex', str(self.fake)], check=True, capture_output=True, timeout=40)
        client = None
        try:
            client = AttachedRPC(str(Path(BINARY).resolve()), state, self.root)
            thread = client.call('threads/start')['thread']['id']
            client.call('turn/start', {'thread_id': thread, 'text': 'long'})
            cursor, instance = client.cursor, client.instance
            client.close()
            time.sleep(.1)
            with self.assertRaises(RuntimeError):
                self.peer.call('thread/resume', {'threadId': thread})
            (self.root / (thread + '.finish')).touch()
            self.acquire(thread)
            self.peer.close()
            client = AttachedRPC(str(Path(BINARY).resolve()), state, self.root, cursor, instance)
            client.call('threads/resume', {'thread_id': thread})
            client.close()
            self.peer = RPC([str(self.fake)], self.root)
            self.peer.call('initialize', {'clientInfo': {'name': 'fixture', 'version': '1'}})
            self.peer.send({'method': 'initialized'})
            self.acquire(thread)
        finally:
            if client is not None:
                client.close()
            subprocess.run(control + ['--stop'], check=True, capture_output=True, timeout=10)
            deadline = time.monotonic() + 5
            while (state / 'owner.lock').exists() and time.monotonic() < deadline:
                time.sleep(.02)
            self.assertFalse((state / 'owner.lock').exists())

    def test_idle_handoff_preserves_other_running_and_empty_threads(self):
        first, second, empty = self.create(), self.create(), self.create()
        self.start(first, 'long')
        self.start(second, 'finish')
        self.event('turn/completed', second)
        self.host.call('client/attached', {'attached': False})
        self.acquire(second)
        with self.assertRaises(RuntimeError):
            self.peer.call('thread/resume', {'threadId': first})
        with self.assertRaises(RuntimeError):
            self.start(second, 'must-not-dispatch')
        self.host.call('client/attached', {'attached': True})
        self.start(empty, 'finish')
        self.event('turn/completed', empty)
        self.host.call('client/attached', {'attached': False})
        self.acquire(empty)
        (self.root / (first + '.finish')).touch()
        self.event('turn/completed', first)
        self.acquire(first)
        self.assertIsNone(self.host.process.poll())

    def test_worker_decisions_with_same_upstream_id_route_independently(self):
        first, second = self.create(), self.create()
        self.start(first, 'approval')
        a = self.event('decision/required')
        self.start(second, 'approval')
        b = self.event('decision/required')
        self.assertNotEqual(a['decision_id'], b['decision_id'])
        for thread, decision in [(second, b), (first, a)]:
            self.assertEqual(thread, decision['thread_id'])
            self.host.call('decision/respond', {'decision_id': decision['decision_id'], 'result': {'decision': 'accept'}})
            self.event('turn/completed', thread)
            self.host.call('client/attached', {'attached': False})
            self.acquire(thread)

    def test_uncertain_turn_start_is_not_closed_or_replayed_on_detach(self):
        thread = self.create()
        with self.assertRaises(RuntimeError):
            self.start(thread, 'uncertain')
        self.host.call('client/attached', {'attached': False})
        with self.assertRaises(RuntimeError):
            self.peer.call('thread/resume', {'threadId': thread})
        history = json.loads((self.root / (thread + '.json')).read_text())
        self.assertEqual(1, len(history['turns']))

    def test_worker_approval_before_turn_ack_does_not_deadlock(self):
        thread = self.create()
        self.host.sequence += 1
        self.host.send({'id': self.host.sequence, 'method': 'turn/start', 'params': {'thread_id': thread, 'text': 'approval-early'}})
        decision = self.event('decision/required')
        self.assertEqual(thread, decision['thread_id'])
        self.host.call('decision/respond', {'decision_id': decision['decision_id'], 'result': {'decision': 'accept'}})
        self.event('turn/completed', thread)
        self.host.call('client/attached', {'attached': False})
        self.acquire(thread)

    def test_idle_resume_releases_and_later_turn_reacquires_same_id(self):
        thread = self.create()
        self.start(thread, 'finish')
        self.event('turn/completed', thread)
        self.host.call('client/attached', {'attached': False})
        self.acquire(thread)
        self.peer.close()
        self.host.call('client/attached', {'attached': True})
        self.host.call('threads/resume', {'thread_id': thread})
        self.start(thread, 'finish')
        self.event('turn/completed', thread)
        self.host.call('client/attached', {'attached': False})
        history = json.loads((self.root / (thread + '.json')).read_text())
        self.assertEqual(2, len(history['turns']))
        self.peer = RPC([str(self.fake)], self.root)
        self.peer.call('initialize', {'clientInfo': {'name': 'fixture', 'version': '1'}})
        self.peer.send({'method': 'initialized'})
        self.acquire(thread)


if __name__ == '__main__':
    unittest.main()
