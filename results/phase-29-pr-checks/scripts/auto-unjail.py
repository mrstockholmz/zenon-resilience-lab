#!/usr/bin/env python3
"""phase29 auto-unjail watcher (LAB devnet only, chain 70).
Polls the p29 devnet; when dev3 shows isJailed=true:
  1. snapshots ZNN totalSupply -> raw/supply-before-unjail.txt
  2. runs the p29probe `unjail 7 10000000000 dev3` (derived 7 = dev3 stake address)
  3. snapshots supply again -> raw/supply-after-unjail.txt
  4. restarts node n3 and waits for dev3 to produce again
  5. appends each step to raw/auto-unjail.log and exits
Safe to run under nohup; idempotent-ish (exits once done). If unjail fails it retries
every 5 min (the receive may need a few momentums)."""
import json, time, subprocess, os, sys, urllib.request

RAW = os.path.expanduser('~/zenon-resilience-lab/results/phase-29-pr-checks/raw')
PROBE = os.path.expanduser('~/zenon-resilience-lab/results/phase-29-pr-checks/probe/p29probe')
N3 = os.path.expanduser('~/lab-devnet-p29/n3')
ZNND = os.path.expanduser('~/p29-310/build/znnd')
ENV = dict(os.environ, LAB_RPC_URL='http://127.0.0.1:47971',
           LAB_DEVNET_HOME=os.path.expanduser('~/lab-devnet-p29/n1'))

def log(msg):
    line = f"{time.strftime('%FT%TZ', time.gmtime())} {msg}"
    print(line, flush=True)
    with open(f'{RAW}/auto-unjail.log', 'a') as f: f.write(line + '\n')

def rpc(method, params):
    req = urllib.request.Request('http://127.0.0.1:47971',
        data=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode(),
        headers={'Content-Type':'application/json'})
    return json.load(urllib.request.urlopen(req, timeout=10))['result']

def jailed():
    pl = rpc('embedded.pillar.getAll', [0, 10])
    for p in pl['list']:
        if p['name'] == 'dev3':
            return p['isJailed']
    return False

def supply():
    t = rpc('embedded.token.getByZts', ['zts1znnxxxxxxxxxxxxx9z4ulx'])
    return int(t['totalSupply'])

def dev3_produced_recently():
    fm = rpc('ledger.getFrontierMomentum', [])
    ms = rpc('ledger.getMomentumsByHeight', [max(2, fm['height']-5), 6])['list']
    return any(m['producer'].startswith('z1qzedcjmds6c') for m in ms)

log('watcher started')
while True:
    try:
        if jailed():
            log('dev3 IS JAILED — starting unjail sequence')
            break
    except Exception as e:
        log(f'poll error: {e}')
    time.sleep(60)

open(f'{RAW}/supply-before-unjail.txt', 'w').write(str(supply()))
log(f'supply before: {supply()}')

for attempt in range(12):
    r = subprocess.run([PROBE, 'unjail', '7', '10000000000', 'dev3'],
                       capture_output=True, text=True, env=ENV, timeout=600)
    out = (r.stdout + r.stderr).strip()
    log(f'unjail attempt {attempt}: {out[-160:]}')
    if 'included=true' in out:
        break
    time.sleep(300)

time.sleep(30)
open(f'{RAW}/supply-after-unjail.txt', 'w').write(str(supply()))
log(f'supply after: {supply()}')
log(f"isJailed now: {jailed()}")

# restart n3
subprocess.run(['setsid', 'nohup', 'nice', '-n', '10', ZNND, '--data', N3],
               stdout=open(f'{N3}/boot-restart.log', 'w'), stderr=subprocess.STDOUT,
               stdin=subprocess.DEVNULL, cwd=N3, env=ENV)
log('n3 restarted')

for i in range(40):
    time.sleep(30)
    try:
        if dev3_produced_recently():
            log('dev3 is producing again — sequence complete')
            sys.exit(0)
    except Exception as e:
        log(f'wait error: {e}')
log('dev3 not producing after 20 min — needs manual check')
sys.exit(1)
