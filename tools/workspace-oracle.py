#!/usr/bin/env python3
"""Compare Super workspace trust against the pinned official Prodex 0.438.2."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SOURCE = 'decfed45f884fe2bef20911303beab7b198698b7'
BINARY_SHA = '3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f'
MANIFEST_SHA = '93b5b5f90d59b23f800e47d17127ee7671d8a16ef72c6cabf30edd1093617725'
ENTRY = re.compile(r'("(?:[^"\\]|\\.)*")=\{trust_level="trusted"\}')
SHIM = '''#!/usr/bin/python3
import json,os,sys
if '--version' in sys.argv:
 print('codex-cli 0.163.0')
elif 'app-server' in sys.argv:
 for line in sys.stdin:
  request=json.loads(line)
  if 'id' in request:
   result={'data':[]} if request.get('method')=='hooks/list' else {}
   print(json.dumps({'id':request['id'],'result':result}),flush=True)
else:
 with open(os.environ['WORKSPACE_ORACLE_ARGS'],'w') as f: json.dump(sys.argv[1:],f)
'''


def output(args):
    return subprocess.check_output(args, text=True, timeout=20).strip()


def verify(options):
    assert hashlib.sha256(Path(options.prodex).read_bytes()).hexdigest() == BINARY_SHA
    manifest = Path(options.prodex).parent / 'release-manifest.json'
    assert hashlib.sha256(manifest.read_bytes()).hexdigest() == MANIFEST_SHA
    assert output([options.prodex, '--version']) == 'prodex 0.438.2'
    for source, commit in [(options.prodex_source, SOURCE), (options.godex_source, options.godex_commit)]:
        assert len(commit) == 40 and output(['git', '-C', source, 'rev-parse', 'HEAD']) == commit
        assert not output(['git', '-C', source, 'status', '--porcelain', '--untracked-files=all'])
    build = output(['go', 'version', '-m', options.godex])
    assert 'vcs.revision=' + options.godex_commit in build and 'vcs.modified=false' in build


def project_paths(arguments):
    overrides = [argument for argument in arguments if argument.startswith('projects=')]
    assert len(overrides) == 1, 'expected exactly one temporary projects override'
    value = overrides[0]
    assert value.startswith('projects={') and value.endswith('}')
    inner = value[len('projects={'):-1]
    entries = list(ENTRY.finditer(inner))
    assert ','.join(entry.group(0) for entry in entries) == inner, 'invalid project trust encoding'
    return [json.loads(entry.group(1)) for entry in entries]


def self_test():
    good = ['-c', 'projects={"/current"={trust_level="trusted"},"/recent"={trust_level="trusted"}}']
    assert project_paths(good) == ['/current', '/recent']
    mutations = [[], [good[1], good[1]], [good[1].replace('trusted', 'untrusted')],
                 [good[1].replace('recent', 'other')], [good[1].replace(',"/recent"={trust_level="trusted"}', '')],
                 [good[1].replace('"/current"', '"/recent"')]]
    for arguments in mutations:
        try:
            assert project_paths(arguments) == ['/current', '/recent']
        except AssertionError:
            continue
        raise AssertionError('workspace mutation escaped oracle')


class Backend(BaseHTTPRequestHandler):
    inference = 0

    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{"data":[{"id":"synthetic-model","object":"model"}]}')

    def do_POST(self):
        Backend.inference += 1
        self.send_error(500, 'unexpected inference in argv fixture')

    def log_message(self, *_):
        pass


def run_product(root, name, binary, fake, endpoint):
    root = root / name
    home, profile = root / 'home', root / 'profile'
    for path in (root, home, profile, home / '.codex' / 'sessions', home / '.codex' / 'archived_sessions'):
        path.mkdir(parents=True, exist_ok=True, mode=0o700)
    config = 'model = "synthetic-model"\n'
    (profile / 'config.toml').write_text(config)
    (root / 'logical').symlink_to(root, target_is_directory=True)
    environment = {'PATH': '/usr/bin:/bin', 'HOME': str(home), 'CODEX_HOME': str(home / '.codex'),
                   'PRODEX_HOME': str(root / 'state'), 'GODEX_HOME': str(root / 'state'),
                   'PRODEX_CODEX_BIN': str(fake), 'GODEX_CODEX_BIN': str(fake),
                   'WORKSPACE_ORACLE_ARGS': str(root / 'args.json'), 'NO_COLOR': '1', 'CI': '1',
                   'TERM': 'dumb', 'PRODEX_NO_UPDATE_CHECK': '1', 'GODEX_NO_UPDATE_CHECK': '1', 'PWD': str(root / 'logical')}
    def invoke(arguments):
        result = subprocess.run([binary] + arguments, env=environment, cwd=root / 'logical', capture_output=True, timeout=25)
        assert result.returncode == 0, f'{name}: {arguments[0]} exit={result.returncode}: {result.stderr.decode()}'
    invoke(['profile', 'add', 'main', '--codex-home', str(profile), '--activate'])
    candidates = {}
    fixtures = [('sessions', 'rollout-z.jsonl', 'tie-z', 300), ('sessions', 'rollout-a.jsonl', 'tie-a', 300),
                ('sessions', 'rollout-new.jsonl', 'newest', 500), ('sessions', 'rollout-duplicate.jsonl', 'newest', 400),
                ('sessions', 'rollout-relative.jsonl', None, 900), ('sessions', 'rollout-invalid.jsonl', 'invalid', 1000),
                ('archived_sessions', 'rollout-old.jsonl', 'archived', 100),
                ('sessions', 'imported-session.jsonl', 'imported', 600)]
    for index, (directory, filename, workspace, modified) in enumerate(fixtures):
        path = home / '.codex' / directory / filename
        cwd = str(root / workspace) if workspace else 'relative/workspace'
        body = json.dumps({'type': 'session_meta', 'payload': {'id': f'01900000-0000-7000-8000-{index + 1:012}', 'cwd': cwd}})
        path.write_text(('invalid prefix\n' if workspace == 'invalid' else '') + body +
                        '\n{"type":"turn_context","payload":{"cwd":"/untrusted-later-context"}}\n')
        os.utime(path, (modified, modified))
        if workspace and workspace != 'invalid':
            candidates[filename] = (cwd, modified)
    observed = []
    for directory in ('sessions', 'archived_sessions'):
        with os.scandir(home / '.codex' / directory) as entries:
            observed.extend(candidates[entry.name] for entry in entries if entry.name in candidates)
    expected = list(dict.fromkeys([str(root)] + [path for path, _ in sorted(observed, key=lambda value: -value[1])]))
    invoke(['super', '--url', endpoint, '--model', 'synthetic-model', '--no-presidio', '--no-sub-agent', 'exec', 'workspace probe'])
    arguments = json.loads((root / 'args.json').read_text())
    assert project_paths(arguments) == expected, f'{name}: current/recent workspace selection differs'
    assert arguments.count('exec') == arguments.count('workspace probe') == 1
    assert arguments.index('exec') < arguments.index('workspace probe'), 'Codex arguments changed'
    assert (profile / 'config.toml').read_text() == config, 'persistent profile config changed'
    assert Backend.inference == 0, 'argv fixture unexpectedly dispatched inference'
    print(f'{name}: current + {len(expected) - 1} newest/stable/deduplicated workspaces, relative/invalid/later context excluded')


def main():
    if not __debug__:
        raise RuntimeError('workspace oracle requires enabled assertions')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--self-test', action='store_true')
    for name in ('prodex', 'prodex-source', 'godex', 'godex-source', 'godex-commit'):
        parser.add_argument('--' + name)
    options = parser.parse_args()
    self_test()
    if options.self_test:
        print('PASS workspace oracle negative controls')
        return
    assert all(getattr(options, key) for key in ('prodex', 'prodex_source', 'godex', 'godex_source', 'godex_commit'))
    verify(options)
    with tempfile.TemporaryDirectory(prefix='godex-workspace-oracle-') as temporary:
        root = Path(temporary)
        fake = root / 'codex'
        fake.write_text(SHIM)
        fake.chmod(0o700)
        server = ThreadingHTTPServer(('127.0.0.1', 0), Backend)
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            for name in ('prodex', 'godex'):
                run_product(root, name, getattr(options, name), fake, f'http://127.0.0.1:{server.server_port}')
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
    print('PASS pinned two-executable workspace trust and negative controls')


if __name__ == '__main__':
    main()
