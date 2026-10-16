#!/usr/bin/env python3
"""Regenerate native Gemini golden cases from the compiled Prodex 0.438.0 kernel.

Usage: python3 tools/gemini-content-oracle.py /path/to/prodex-git-repository
Requires Mojo; compilation and all transforms stay local and use no credentials.
"""
import ctypes as c
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile

REF = '8cab9794b0e08b2cb8507df8321a8e38ebd6492f'
if len(sys.argv) != 2:
    raise SystemExit(__doc__)
temporary = tempfile.TemporaryDirectory(prefix='godex-gemini-source-')
ROOT = Path(temporary.name)
archive = subprocess.check_output(['git', '-C', sys.argv[1], 'archive', REF, 'mojo/prodex_core'])
with tarfile.open(fileobj=io.BytesIO(archive)) as tree:
    tree.extractall(ROOT, filter='data')
subprocess.run(['mojo', 'build', '--emit', 'shared-lib', str(ROOT / 'mojo/prodex_core/gemini_content_hardening.mojo'), '-o', str(ROOT / 'gemini_content_hardening.so')], check=True)
class View(c.Structure):
    _fields_ = [('ptr', c.c_uint64), ('length', c.c_uint64)]
class Node(c.Structure):
    _fields_ = [(key, c.c_int64) for key in ['kind', 'first_child', 'next_sibling', 'parent']] + [('key', View), ('text', View), ('raw_start', c.c_int64), ('raw_length', c.c_int64)]

kernel = c.CDLL(str(ROOT / 'gemini_content_hardening.so')).prodex_mojo_gemini_content_hardening_v1
kernel.argtypes = [c.c_int64] * 13
kernel.restype = c.c_int64

def transform(value, operation):
    nodes, buffers, raw = [], [], bytearray()
    def view(text):
        encoded = text.encode()
        buffer = c.create_string_buffer(encoded)
        buffers.append(buffer)
        return View(c.addressof(buffer), len(encoded))
    def emit(value, parent=-1, key=''):
        kind = 0 if value is None else 1 if value is False else 2 if value is True else 3 if isinstance(value, (int, float)) else 4 if isinstance(value, str) else 5 if isinstance(value, list) else 6
        index = len(nodes)
        node = Node(kind, -1, -1, parent, view(key), view(value if isinstance(value, str) else ''), len(raw), 0)
        nodes.append(node)
        if kind < 5:
            raw.extend(json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode())
        else:
            raw.extend(b'[' if kind == 5 else b'{')
            previous = -1
            for child_key, child in (enumerate(value) if kind == 5 else value.items()):
                if previous >= 0:
                    raw.extend(b',')
                if kind == 6:
                    raw.extend(json.dumps(child_key, ensure_ascii=False).encode() + b':')
                child_index = emit(child, index, child_key if kind == 6 else '')
                if previous < 0:
                    node.first_child = child_index
                else:
                    nodes[previous].next_sibling = child_index
                previous = child_index
            raw.extend(b']' if kind == 5 else b'}')
        node.raw_length = len(raw) - node.raw_start
        return index
    emit(value)
    tree = (Node * len(nodes))(*nodes)
    source = c.create_string_buffer(bytes(raw))
    scratch = (c.c_uint64 * (len(nodes) * 8))()
    metadata = (c.c_int64 * 2)()
    args = [1, operation, 0, c.addressof(tree), len(nodes), c.addressof(source), len(raw), c.addressof(scratch), len(nodes) * 4]
    status = kernel(*args, 1, 0, 0, c.addressof(metadata))
    assert status == 0, (operation, status)
    output = c.create_string_buffer(metadata[1])
    status = kernel(*args, 0, c.addressof(output), len(output), c.addressof(metadata))
    assert status == 0, (operation, status)
    return json.loads(output.raw)

def row(role, *parts, **metadata):
    return dict(role=role, parts=list(parts), **metadata)
def call(identity='a', name='lookup', **metadata):
    return dict(functionCall=dict(id=identity, name=name, args={'q': identity}), **metadata)
UNSET = object()
def response(identity='a', name='lookup', result=UNSET):
    return dict(functionResponse=dict(id=identity, name=name, response={'value': identity} if result is UNSET else result))
text = {'text': 'retain text'}
media = {'inlineData': {'mimeType': 'image/png', 'data': 'c3ludGhldGlj'}}
falsy = [False, 0, '', {}, [], None]
assert all(response(result=value)['functionResponse']['response'] is value for value in falsy)
cases = [
    ('consecutive-model-reversed-results', [row('model', call('a', thoughtSignature='fixture-signature'), first={'keep': True}), row('model', call('b'), later='discard'), row('user', response('b'), response('a'), user_metadata=[1, 2])]),
    ('consecutive-user-responses-and-media', [row('user', text), row('model', call('a'), call('b')), row('user', text, response('b'), media, first='keep'), row('user', response('a'), text, later='discard')]),
    ('duplicate-calls-with-one-result', [row('model', call('a')), row('model', call('a')), row('user', response('a'))]),
    ('same-id-different-names-and-orphan', [row('model', call('a', 'first')), row('model', call('a', 'second')), row('user', response('a', 'other', {'error': 'fixture failure'})), row('user', response('a', 'first'))]),
    ('null-call-and-null-response', [row('model', {'functionCall': None}, call('a')), row('user', media, response('a')), row('user', {'functionResponse': None}, text)]),
    ('null-call-with-missing-result', [row('model', {'functionCall': None}), row('model', text), row('user', text, media)]),
    ('orphan-consecutive-users', [row('user', response('orphan'), text, first='keep'), row('user', media, response('other', 'other'), later='discard')]),
    ('missing-result-with-model-text-tail', [row('model', call('a')), row('model', text, media)]),
    ('invalid-rows-do-not-block-coalescing', [None, row('model', call('a'), first='keep'), {'role': 'user', 'parts': False}, row('model', call('b')), row('user', response('b')), row('model'), row('user', response('a')), {'role': 'model', 'parts': False}]),
    ('custom-roles-do-not-trigger-tool-repair', [row('custom', call('a')), row('custom', response('b')), row('', {'functionCall': None}), row('user', text)]),
    ('part-with-both-call-and-response', [row('model', dict(call('a'), **response('b'))), row('model', call('b')), row('user', dict(response('b'), **call('a')), response('a'))]),
    ('duplicate-missing-calls-remain-explicit', [row('model', call('a')), row('model', call('a')), row('user', text)]),
    ('duplicate-responses-preserve-errors-and-signatures', [row('model', call('a', thoughtSignature='fixture-signature')), row('user', response('a', result={'error': 'fixture failure'})), row('user', response('a', result={'value': 2}), media)]),
    ('orphan-after-non-tool-model', [row('model', text), row('model', media), row('user', response('orphan'))]),
    ('falsy-and-null-response-payloads', [row('model', *(call(str(index)) for index in range(3))), row('model', *(call(str(index)) for index in range(3, 6))), row('user', *(response(str(index), result=falsy[index]) for index in [5, 4, 3])), row('user', *(response(str(index), result=falsy[index]) for index in [2, 1, 0]))]),
]
fixture = {'source_commit': REF, 'operation_order': [0, 1, 2, 4, 3], 'cases': []}
for name, value in cases:
    current = value
    for operation in fixture['operation_order']:
        current = transform(current, operation)
    fixture['cases'].append({'name': name, 'input': value, 'expected': current})
target = Path(__file__).resolve().parents[1] / 'internal/gateway/gemini/testdata/native_compound_04380.json'
target.write_text('{\n  "source_commit": ' + json.dumps(fixture['source_commit']) + ',\n  "operation_order": [0, 1, 2, 4, 3],\n  "cases": [\n' + ',\n'.join('    ' + json.dumps(case, ensure_ascii=False, separators=(',', ':')) for case in fixture['cases']) + '\n  ]\n}\n')
print(f'PASS: {len(cases)} fixtures from compiled tagged Mojo kernel: {target}')

# Every semantic pass must matter to at least one independent golden case.
for omitted in fixture['operation_order']:
    detected = False
    for case in fixture['cases']:
        current = case['input']
        for operation in fixture['operation_order']:
            if operation != omitted:
                current = transform(current, operation)
        detected |= current != case['expected']
    assert detected, f'fixtures failed to detect omitted operation {omitted}'
print('PASS: all five missing-pass negative controls detected')
