#!/usr/bin/env python3
"""phase29 jail-lifecycle monitor. Appends one JSON line per poll to
results/phase-29-pr-checks/raw/monitor.jsonl on the lab repo.
Polls: frontier, recent producers, pillar isJailed + produced/expected, ZNN supply."""
import json, time, urllib.request, os

RAW = os.path.expanduser('~/zenon-resilience-lab/results/phase-29-pr-checks/raw/monitor.jsonl')
PORT = 47971

def rpc(method, params):
    req = urllib.request.Request(f'http://127.0.0.1:{PORT}',
        data=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode(),
        headers={'Content-Type':'application/json'})
    return json.load(urllib.request.urlopen(req, timeout=10))['result']

seen = 0
with open(RAW, 'a') as out:
    while True:
        try:
            fm = rpc('ledger.getFrontierMomentum', [])
            h = fm['height']
            row = {'ts': int(time.time()), 'height': h, 'momentumTs': fm['timestamp']}
            if h > seen:
                ms = rpc('ledger.getMomentumsByHeight', [max(2, h-4), 5])['list']
                row['producers'] = {m['height']: m['producer'][:12] for m in ms}
                seen = h
            pl = rpc('embedded.pillar.getAll', [0, 10])
            row['pillars'] = {p['name']: {'isJailed': p['isJailed'],
                'produced': p['currentStats']['producedMomentums'],
                'expected': p['currentStats']['expectedMomentums']} for p in pl['list']}
            tk = rpc('embedded.token.getByZts', ['zts1znnxxxxxxxxxxxxx9z4ulx'])
            row['znnSupply'] = tk['totalSupply']
            out.write(json.dumps(row) + '\n'); out.flush()
        except Exception as e:
            out.write(json.dumps({'ts': int(time.time()), 'error': str(e)}) + '\n'); out.flush()
        time.sleep(45)
