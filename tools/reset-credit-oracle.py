#!/usr/bin/env python3
"""Check display-only reset-credit expiry against exact Prodex 0.439.0."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

FINITE = {'credits': [
    {'status': 'consumed', 'expires_at': '2020-01-01T00:00:00Z'},
    {'status': 'available', 'expires_at': '2026-10-22T19:00:00Z'},
    {'status': 'available', 'expires_at': '2026-10-23T01:23:45.987+07:00'},
    {'status': 'available', 'expires_at': None},
]}
ORIGINATORS = {
    'originator_custom': ('codex-tui', 'codex-tui'),
    'originator_empty': ('', ''),
    'originator_invalid': ('invalid\noriginator', 'codex_cli_rs'),
}
CASES = [
    ('finite', 3, 200, FINITE, '3 available | next expiry: 2026-10-22 18:23:45 +00:00'),
    ('never', 2, 200, {'credits': [{'status': 'available', 'expires_at': None}]}, '2 available | next expiry: never'),
    ('failed', 3, 500, {}, '3 available | next expiry: unavailable'),
    ('incomplete', 3, 200, {'credits': [{'status': 'available'}]}, '3 available | next expiry: unavailable'),
    ('invalid', 3, 200, {'credits': [{'status': 'available', 'expires_at': 'bad'}]}, '3 available | next expiry: unavailable'),
    ('uppercase_credits', 3, 200, '{"Credits":[{"status":"available","expires_at":null}]}', '3 available | next expiry: unavailable'),
    ('uppercase_status', 3, 200, '{"credits":[{"Status":"available","expires_at":null}]}', '3 available | next expiry: unavailable'),
    ('uppercase_expiry', 3, 200, '{"credits":[{"status":"available","Expires_At":null}]}', '3 available | next expiry: unavailable'),
    ('duplicate_credits', 3, 200, '{"credits":[],"credits":[{"status":"available","expires_at":null}]}', '3 available | next expiry: unavailable'),
    ('duplicate_status', 3, 200, '{"credits":[{"status":"consumed","status":"available","expires_at":null}]}', '3 available | next expiry: unavailable'),
    ('duplicate_expiry', 3, 200, '{"credits":[{"status":"available","expires_at":"bad","expires_at":null}]}', '3 available | next expiry: unavailable'),
    ('unknown_duplicates', 3, 200, '{"credits":[{"status":"available","expires_at":null,"unknown":1,"unknown":2}],"ignored":1,"ignored":2}', '3 available | next expiry: never'),
    ('zero', 0, 200, FINITE, '0 available'),
    ('raw', 3, 200, FINITE, ''),
]
CASES += [(name, 1, 200, {'credits': [{'status': 'available', 'expires_at': None}]},
           '1 available | next expiry: never') for name in ORIGINATORS]


def verify_result(case, result):
    name, count, _, _, summary = case
    calls, text, code, unchanged = result
    expected_paths = ['/backend-api/wham/usage']
    if count > 0 and name != 'raw':
        expected_paths.append('/backend-api/wham/rate-limit-reset-credits')
    assert code == 0 and unchanged, 'failed command or changed credentials'
    assert [call['path'] for call in calls] == expected_paths, 'wrong request count or route'
    assert all(call['auth'] and call['account'] and call['residency'] == 'us' for call in calls), 'accepted identity/residency lost'
    originator = ORIGINATORS.get(name, (None, 'codex_cli_rs'))[1]
    assert all(call['originator'] == originator for call in calls), 'wrong auth request originator'
    if name == 'raw':
        assert json.loads(text)['rate_limit_reset_credits']['available_count'] == count
        assert 'next expiry' not in text
    else:
        assert summary in ' '.join(text.split()), 'wrong available count or expiry'
        assert text.count('next expiry:') == (1 if count > 0 else 0), 'duplicate or invented expiry'


def self_test():
    for case in CASES:
        name, count, _, _, summary = case
        calls = [{'path': '/backend-api/wham/usage', 'auth': True, 'account': True, 'residency': 'us',
                  'originator': ORIGINATORS.get(name, (None, 'codex_cli_rs'))[1]}]
        if count > 0 and name != 'raw':
            calls.append(dict(calls[0], path='/backend-api/wham/rate-limit-reset-credits'))
        text = json.dumps({'rate_limit_reset_credits': {'available_count': count}}) if name == 'raw' else summary
        verify_result(case, (calls, text, 0, True))
        mutations = [(calls + calls, text, 0, True), (calls, text, 1, True), (calls, text, 0, False),
                     (calls, text.replace(str(count), '999'), 0, True),
                     ([dict(call, auth=False) for call in calls], text, 0, True),
                     ([dict(call, originator='invented') for call in calls], text, 0, True)]
        for changed in mutations:
            try:
                verify_result(case, changed)
            except AssertionError:
                continue
            raise AssertionError('accepted a mutated expiry result')


def run_case(root, binary, product, case):
    name, count, detail_status, details, _ = case
    directory = root / product / name
    directory.parent.mkdir(mode=0o700, exist_ok=True)
    directory.mkdir(mode=0o700)
    profile = directory / 'profile'
    profile.mkdir(mode=0o700)
    auth = profile / 'auth.json'
    original = b'{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-access","account_id":"synthetic-account"}}'
    auth.write_bytes(original)
    auth.chmod(0o600)
    (profile / 'config.toml').write_text("enforce_residency='us'\n")
    usage = {'plan_type': 'plus', 'rate_limit_reset_credits': {'available_count': count},
             'rate_limit': {'primary_window': {'used_percent': 20, 'reset_at': 4102444800, 'limit_window_seconds': 18000}}}
    calls = []

    class Backend(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            calls.append({'path': self.path, 'auth': self.headers.get('Authorization') == 'Bearer synthetic-access',
                          'account': self.headers.get('ChatGPT-Account-Id') == 'synthetic-account',
                          'originator': self.headers.get('originator'),
                          'residency': self.headers.get('x-openai-internal-codex-residency')})
            is_usage = self.path == '/backend-api/wham/usage'
            body = details.encode() if not is_usage and isinstance(details, str) else json.dumps(usage if is_usage else details).encode()
            self.send_response(200 if is_usage else detail_status)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    environment = {'PATH': os.environ['PATH'], 'HOME': str(directory / 'user'), 'CODEX_HOME': str(directory / 'ambient'),
                   'PRODEX_HOME': str(directory / 'state'), 'GODEX_HOME': str(directory / 'state'),
                   'PRODEX_CODEX_BIN': '/bin/true', 'GODEX_CODEX_BIN': '/bin/true', 'TZ': 'UTC',
                   'PRODEX_NO_UPDATE_CHECK': '1', 'GODEX_NO_UPDATE_CHECK': '1', 'NO_COLOR': '1', 'CI': '1', 'TERM': 'dumb'}
    if name in ORIGINATORS:
        environment['CODEX_INTERNAL_ORIGINATOR_OVERRIDE'] = ORIGINATORS[name][0]
    registered = subprocess.run([binary, 'profile', 'add', 'main', '--codex-home', str(profile), '--activate'],
                                cwd=directory, env=environment, capture_output=True, text=True, timeout=15)
    assert registered.returncode == 0, registered.stderr
    server = ThreadingHTTPServer(('127.0.0.1', 0), Backend)
    worker = threading.Thread(target=server.serve_forever)
    worker.start()
    try:
        arguments = ['quota', '--profile', 'main', '--base-url', f'http://127.0.0.1:{server.server_port}/backend-api']
        arguments.append('--raw' if name == 'raw' else '--once')
        result = subprocess.run([binary, *arguments], cwd=directory, env=environment, capture_output=True, text=True, timeout=25)
    finally:
        server.shutdown()
        worker.join()
        server.server_close()
    verify_result(case, (calls, result.stdout, result.returncode, auth.read_bytes() == original))
    return {'product': product, 'case': name, 'requests': len(calls), 'exit_status': result.returncode, 'summary': case[4]}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--self-test', action='store_true')
    for name in ('prodex', 'prodex-source', 'godex', 'godex-source', 'godex-commit'):
        parser.add_argument('--' + name)
    options = parser.parse_args()
    self_test()
    if options.self_test:
        print('PASS reset-credit oracle mutation controls')
        return
    assert all(getattr(options, name) for name in ('prodex', 'prodex_source', 'godex', 'godex_source', 'godex_commit'))
    runpy.run_path(str(Path(__file__).with_name('workspace-oracle.py')))['verify'](options)
    rows = []
    with tempfile.TemporaryDirectory(prefix='godex-reset-credit-parity-') as temporary:
        for product in ('prodex', 'godex'):
            for case in CASES:
                try:
                    rows.append(run_case(Path(temporary), getattr(options, product), product, case))
                except AssertionError as error:
                    raise AssertionError(f'{product} {case[0]}: {error}') from error
    print(json.dumps({'status': 'PASS', 'godex_commit': options.godex_commit,
                      'driver_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'cases': rows}, indent=2))


if __name__ == '__main__':
    main()
