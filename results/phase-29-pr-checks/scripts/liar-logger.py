#!/usr/bin/env python3
"""phase29 liar-test logger for the PR #306 devnet check.
Appends {ts, phase, height, syncState, target, peers} to raw/liar-test.jsonl.
The phase label is read each poll from raw/liar-phase.txt (warmup/A-postfix/B-prefix)."""
import json, time, urllib.request, os

RAW = os.path.expanduser('~/zenon-resilience-lab/results/phase-29-pr-checks/raw')
PORT = 47985

def rpc(method, params):
    req = urllib.request.Request(f'http://127.0.0.1:{PORT}',
        data=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode(),
        headers={'Content-Type':'application/json'})
    return json.load(urllib.request.urlopen(req, timeout=10))['result']

with open(f'{RAW}/liar-test.jsonl', 'a') as out:
    while True:
        try:
            phase = open(f'{RAW}/liar-phase.txt').read().strip() or 'warmup'
            fm = rpc('ledger.getFrontierMomentum', [])
            si = rpc('stats.syncInfo', [])
            ni = rpc('stats.networkInfo', [])
            out.write(json.dumps({'ts': int(time.time()), 'phase': phase, 'height': fm['height'],
                                  'syncState': si['state'], 'target': si['targetHeight'],
                                  'peers': len(ni['peers'])}) + '\n')
            out.flush()
        except Exception as e:
            out.write(json.dumps({'ts': int(time.time()), 'error': str(e)}) + '\n'); out.flush()
        time.sleep(20)
