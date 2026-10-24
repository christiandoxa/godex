#!/usr/bin/env python3
"""Verify native --last selection with pinned Prodex and official Codex 0.164.0."""
import argparse
import copy
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import runpy
import signal
import select
import sys
import subprocess
import tempfile
import threading

CODEX_SHA = 'b93f6811d3f653f0fb83be4050d857fb9abbb111dfa2ce160563327daff7f306'


def verify_run(row):
    attempts = [call for call in row['calls'] if call['method'] == 'POST']
    assert row['exit'] == 1 and row['timed_out'] is False, 'child did not stop on mock error'
    assert len(attempts) == 1, 'model request was missing or replayed'
    assert attempts[0]['prompt_cache_key'] == row['expected_targets']['exec'], 'wrong native thread'
    assert attempts[0]['auth_matches'] is True, 'lost credential ownership'
    assert row['credential_file_unchanged'] is True, 'changed profile credentials'


def self_test():
    good = {'exit': 1, 'timed_out': False, 'credential_file_unchanged': True,
            'expected_targets': {'exec': 'new', 'cli': 'old'},
            'calls': [{'method': 'POST', 'prompt_cache_key': 'new', 'auth_matches': True}]}
    verify_run(good)
    mutations = []
    for field, value in [('exit', 0), ('timed_out', True), ('credential_file_unchanged', False),
                         ('calls', []), ('calls', good['calls'] * 2)]:
        changed = copy.deepcopy(good)
        changed[field] = value
        mutations.append(changed)
    for field, value in [('prompt_cache_key', 'old'), ('auth_matches', False)]:
        changed = copy.deepcopy(good)
        changed['calls'][0][field] = value
        mutations.append(changed)
    for changed in mutations:
        try:
            verify_run(changed)
        except AssertionError:
            continue
        raise AssertionError('native selection mutation escaped oracle')
    if sys.platform == 'linux' and hasattr(os, 'pidfd_open'):
        check_helper_cleanup()


def reap_native_helpers(profile):
    # Match only this private fixture home; pidfds keep signals tied to that process.
    for process in Path('/proc').iterdir():
        if not process.name.isdigit():
            continue
        try:
            environment = (process / 'environ').read_bytes().split(b'\0')
            if ('CODEX_HOME=' + str(profile)).encode() not in environment:
                continue
            descriptor = os.pidfd_open(int(process.name))
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            continue
        try:
            signal.pidfd_send_signal(descriptor, signal.SIGTERM)
            if not select.select([descriptor], [], [], 1)[0]:
                signal.pidfd_send_signal(descriptor, signal.SIGKILL)
                assert select.select([descriptor], [], [], 1)[0], 'native helper did not stop'
        except ProcessLookupError:
            pass
        finally:
            os.close(descriptor)


def check_helper_cleanup():
    with tempfile.TemporaryDirectory(prefix='godex-native-reap-') as temporary:
        profile = Path(temporary) / 'owned'
        children = []
        try:
            for home in (profile, Path(temporary) / 'unrelated'):
                children.append(subprocess.Popen(
                    [sys.executable, '-c', 'import signal; signal.pause()'],
                    env={'CODEX_HOME': str(home)}, stdin=subprocess.DEVNULL,
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
            reap_native_helpers(profile)
            assert children[0].wait(timeout=3) == -signal.SIGTERM, 'owned helper was not reaped'
            assert children[1].poll() is None, 'unrelated process was terminated'
        finally:
            for child in children:
                if child.poll() is None:
                    child.kill()
                child.wait(timeout=3)


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--self-test', action='store_true')
for option in ('prodex', 'prodex-source', 'godex', 'godex-source', 'godex-commit', 'codex'):
    parser.add_argument('--' + option)
options = parser.parse_args()
if options.self_test:
    self_test()
    print('PASS native last oracle mutation controls')
    sys.exit(0)
for option in ('prodex', 'prodex_source', 'godex', 'godex_source', 'godex_commit', 'codex'):
    if not getattr(options, option):
        parser.error('--' + option.replace('_', '-') + ' is required')
if sys.platform != 'linux' or not hasattr(os, 'pidfd_open'):
    parser.error('native last oracle requires Linux with pidfd support')
runpy.run_path(str(Path(__file__).with_name('workspace-oracle.py')))['verify'](options)
native = Path(options.codex)
assert hashlib.sha256(native.read_bytes()).hexdigest() == CODEX_SHA, 'official Codex digest mismatch'
self_test()
rows = []
with tempfile.TemporaryDirectory(prefix='godex-native-last-') as temporary:
    for product in ('prodex','godex'):
        root = Path(temporary)/product
        profile, shared = root/'profile', root/'shared'
        profile.mkdir(parents=True,mode=0o700)
        root.chmod(0o700)
        shared.mkdir(mode=0o700)
        (shared/'sessions').mkdir(mode=0o700)
        original = b'{"OPENAI_API_KEY":"synthetic-native-key"}'
        auth = profile/'auth.json'
        auth.write_bytes(original)
        auth.chmod(0o600)
        (profile/'sessions').symlink_to(shared/'sessions',target_is_directory=True)
        calls = []
        class Backend(BaseHTTPRequestHandler):
            protocol_version="HTTP/1.1"
            def log_message(self,*_): pass
            def respond(self,status,body):
                encoded=json.dumps(body).encode()
                self.send_response(status)
                self.send_header('Content-Type','application/json')
                self.send_header('Content-Length',str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)
            def do_GET(self):
                calls.append({'method':'GET','path':self.path})
                self.respond(426 if self.headers.get('Upgrade') else 200,{'models':[]})
            def do_POST(self):
                size = int(self.headers.get('Content-Length', '0'))
                assert 0 <= size <= 4 * 1024 * 1024, 'oversized native request'
                raw = self.rfile.read(size)
                try: body=json.loads(raw)
                except (ValueError,UnicodeError): body={}
                calls.append({'method':'POST','path':self.path,
                              'session_id':self.headers.get('session_id'),
                              'conversation_id':self.headers.get('conversation_id'),
                              'prompt_cache_key':body.get('prompt_cache_key'),
                              'auth_matches':self.headers.get('Authorization')=='Bearer synthetic-native-key'})
                self.respond(400,{'error':{'type':'invalid_request_error','message':'synthetic selection stop'}})
        server=ThreadingHTTPServer(('127.0.0.1',0),Backend)
        worker=threading.Thread(target=server.serve_forever)
        worker.start()
        base='http://127.0.0.1:'+str(server.server_port)+'/v1'
        (profile/'config.toml').write_text('model="gpt-5.4"\nopenai_base_url="'+base+'"\n')
        env={'PATH':os.environ['PATH'],'HOME':str(root/'user'),'CODEX_HOME':str(root/'ambient'),
             'PRODEX_HOME':str(root/'state'),'GODEX_HOME':str(root/'state'),'PRODEX_SHARED_CODEX_HOME':str(shared),
             'PRODEX_CODEX_BIN':str(native),'GODEX_CODEX_BIN':str(native),'OPENAI_BASE_URL':base,
             'PRODEX_NO_UPDATE_CHECK':'1','GODEX_NO_UPDATE_CHECK':'1','TERM':'dumb','NO_COLOR':'1','CI':'1',
             'HTTP_PROXY':'http://127.0.0.1:1','HTTPS_PROXY':'http://127.0.0.1:1','NO_PROXY':'127.0.0.1,localhost'}
        try:
            setup=subprocess.run([getattr(options,product),'profile','add','main','--codex-home',str(profile),'--activate'],cwd=root,env=env,capture_output=True,text=True,timeout=15)
            assert setup.returncode==0, product+' profile setup failed'
            target_ids={}
            for kind in ('cli','exec'):
                before=set((shared/'sessions').rglob('rollout-*.jsonl'))
                seed_env=dict(env,CODEX_HOME=str(profile))
                seeded=subprocess.run([str(native),'exec','--skip-git-repo-check','synthetic-seed-'+kind],cwd=root,env=seed_env,
                                      stdin=subprocess.DEVNULL,capture_output=True,text=True,timeout=20)
                assert seeded.returncode==1, 'native seed did not stop on the mock backend'
                created=set((shared/'sessions').rglob('rollout-*.jsonl'))-before
                assert len(created)==1, 'native seed did not create one rollout'
                rollout=created.pop()
                lines=rollout.read_text().splitlines()
                event=json.loads(lines[0])
                target_ids[kind]=event['payload']['id']
                if kind=='cli':
                    stat=rollout.stat()
                    event['payload']['source']='cli'
                    lines[0]=json.dumps(event)
                    rollout.write_text('\n'.join(lines)+'\n')
                    os.utime(rollout,ns=(stat.st_atime_ns,stat.st_mtime_ns))
            assert target_ids['cli'] != target_ids['exec'], 'native seed reused a thread'
            calls.clear()
            command=[getattr(options,product),'run','--skip-quota-check','--no-auto-rotate','--profile','main',
                     'exec','--skip-git-repo-check','resume','--last','continue']
            child=subprocess.Popen(command,cwd=root,env=env,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,start_new_session=True)
            timed_out=False
            try:
                stdout,stderr=child.communicate(timeout=30)
            except subprocess.TimeoutExpired:
                timed_out=True
                os.killpg(child.pid,signal.SIGTERM)
                try: stdout,stderr=child.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(child.pid,signal.SIGKILL)
                    try: stdout,stderr=child.communicate(timeout=5)
                    except subprocess.TimeoutExpired:
                        child.stdout.close()
                        child.stderr.close()
                        stdout,stderr='','synthetic capture closed after bounded shutdown'
                        child.wait(timeout=5)
            assert auth.read_bytes()==original
            rows.append({'product':product,'exit':child.returncode,'timed_out':timed_out,'calls':calls,'expected_targets':target_ids,
                         'credential_file_unchanged': True})
        finally:
            try:
                reap_native_helpers(profile)
            finally:
                server.shutdown()
                worker.join(timeout=5)
                server.server_close()
                assert not worker.is_alive(), 'mock backend did not stop'
for row in rows:
    verify_run(row)
print(json.dumps({'status': 'PASS', 'oracle': 'prodex 0.439.0',
                  'candidate': options.godex_commit, 'codex_sha256': CODEX_SHA, 'runs': rows}, indent=2))
