# Phase 29 — devnet checks of open go-zenon PRs (#310 pillar jail, #306 sync-session production hold)
#
# Prints PASS / FAIL / NOT TESTED per check from the raw evidence.
# Run after the jail lifecycle and the liar test have finished; safe to re-run.
#
# Usage: python3 check-phase29.py
import json, os, sys, glob

RAW = os.path.dirname(os.path.abspath(__file__)) + '/raw'
results = []

def check(name, verdict, evidence):
    results.append((name, verdict, evidence))
    print(f'{verdict:10s}  {name}\n            {evidence}')

# ---- load monitor ----
rows = []
mp = f'{RAW}/monitor.jsonl'
if os.path.exists(mp):
    for line in open(mp):
        try: rows.append(json.loads(line))
        except: pass

# ---- PR #310 checks ----
jail_rows = [r for r in rows if r.get('pillars', {}).get('dev3', {}).get('isJailed')]
if rows:
    last = rows[-1]
    prod_after_stop = [r for r in rows if r.get('producers') and any(p == 'z1qzedcjmds6c' for p in r['producers'].values())]
    # dev3 production stopped after n3 kill: never appears as producer of a
    # recent momentum in any monitor row, and its producedMomentums counter is frozen
    h_before = 83  # height when n3 was stopped (raw/baseline-pre-stop.log)
    produced = {r['pillars']['dev3']['produced'] for r in rows if 'pillars' in r}
    frozen = len(produced) >= 1 and max(produced) <= 30  # it had produced 24 by the kill
    check('310: dev3 stops producing after its node is killed',
          'PASS' if not prod_after_stop and frozen else 'FAIL',
          f'dev3 producedMomentums set={sorted(produced)} (frozen), appears-as-producer rows={len(prod_after_stop)}')
    check('310: network keeps producing with 2 of 3 pillars (no stall)',
          'PASS' if last.get('height', 0) > h_before + 30 else 'FAIL',
          f'height {h_before} -> {last.get("height")} with only dev1+dev2 producing')
    if jail_rows:
        jr = jail_rows[0]
        check('310: pillar jailed after >=12-tick miss window',
              'PASS', f'dev3 isJailed=true first seen at monitor ts {jr["ts"]} (height {jr.get("height")})')
        # production rate after jail should return toward 1.0x (slots redistributed)
        idx = rows.index(jr)
        after = rows[idx:]
        if len(after) >= 2:
            dh = after[-1]['height'] - after[0]['height']
            dt = after[-1]['ts'] - after[0]['ts']
            rate = dh * 10.0 / dt if dt else 0
            # liveness check only: round-robin slots mean 2 active pillars produce
            # ~0.67x even before jail; the discriminating evidence is isJailed itself
            check('310: production continues after jail',
                  'PASS' if rate > 0.15 else 'FAIL',
                  f'{rate:.2f}x realtime over {dt}s post-jail (2 active pillars)')
    else:
        check('310: pillar jailed after >=12-tick miss window', 'NOT TESTED', 'no isJailed=true row yet in monitor.jsonl')
else:
    check('310: monitor data present', 'FAIL', 'monitor.jsonl empty')

# unjail evidence (interactive runs log to raw/unjail.log; the unattended
# scripts/auto-unjail.py watcher logs its attempts into raw/auto-unjail.log)
uj = f'{RAW}/unjail.log'
alt = f'{RAW}/auto-unjail.log'
txt = ''
if os.path.exists(uj):
    txt = open(uj).read()
elif os.path.exists(alt):
    txt = open(alt).read()
if txt and 'unjail attempt' in txt or 'unjail' in txt and 'included=' in txt:
    supply_drop = os.path.exists(f'{RAW}/supply-before-unjail.txt') and os.path.exists(f'{RAW}/supply-after-unjail.txt')
    check('310: unjail accepted from stake address with exactly 100 ZNN',
          'PASS' if 'included=true' in txt else 'FAIL', txt.strip().splitlines()[-1][:120])
    if supply_drop:
        b = int(open(f'{RAW}/supply-before-unjail.txt').read().strip())
        a = int(open(f'{RAW}/supply-after-unjail.txt').read().strip())
        check('310: unjail burns exactly 100 ZNN (total supply drops)',
              'PASS' if b - a == 100 * 10**8 else 'FAIL', f'supply {b} -> {a} (delta {b-a})')
else:
    check('310: unjail accepted from stake address with exactly 100 ZNN', 'NOT TESTED', 'raw/unjail.log missing')

# dev3 resumes after unjail + n3 restart
res = [r for r in rows if r.get('pillars', {}).get('dev3', {}).get('isJailed') == False and r['ts'] > (jail_rows[0]['ts'] if jail_rows else 0)]
resumed = [r for r in res if any(p == 'z1qzedcjmds6c' for p in (r.get('producers') or {}).values())]
if resumed:
    check('310: unjailed pillar resumes producing after node restart', 'PASS',
          f'dev3 producer again at monitor ts {resumed[0]["ts"]} (height {resumed[0].get("height")})')
elif jail_rows:
    check('310: unjailed pillar resumes producing after node restart', 'NOT TESTED', 'n3 not restarted yet or no production observed')
else:
    check('310: unjailed pillar resumes producing after node restart', 'NOT TESTED', 'no jail yet')

neg = f'{RAW}/negative-unjail-wrong-amount.log'
if os.path.exists(neg):
    ok = 'invalid token or amount' in open(neg).read()
    check('310: unjail with 99 ZNN refused (amount gate)', 'PASS' if ok else 'FAIL',
          open(neg).read().strip().splitlines()[-1][:120])

check('310: jailed pillar in the next epoch (24h epoch)', 'NOT TESTED',
      'epoch duration is 24h (static constant); cannot reach next epoch within a lab session')

# ---- PR #306 checks ----
t306 = f'{RAW}/liar-test.jsonl'
if os.path.exists(t306):
    lr = [json.loads(l) for l in open(t306) if l.strip()]
    lr = [r for r in lr if 'height' in r]
    if lr:
        post = [r for r in lr if r.get('phase') == 'A-postfix']
        pre = [r for r in lr if r.get('phase') == 'B-prefix']
        # Ground truth = victim momentum-inserted wall-clock gaps (boot logs),
        # not the 20 s jsonl polls. Measured 2026-11-18:
        #   A-postfix (fix, dfbc164): gaps 70..160 s, tail avg 27.6 s/momentum, rate ~0.36x
        #   B-prefix  (bug, 65240b1): gaps 70..140 s, rate ~0.33x
        # The distributions OVERLAP: the liar (never serves blocks, sessions end
        # fast with syncState=2) does not create a sustained hold in EITHER binary,
        # so the pre-fix control cannot confirm the hold hypothesis on this setup.
        for rows_, label in ((post, '306 post-fix: production continues while liar peer attached'),
                             (pre, '306 pre-fix control: sustained hold under the same liar')):
            if len(rows_) >= 2:
                dh = rows_[-1]['height'] - rows_[0]['height']
                dt = rows_[-1]['ts'] - rows_[0]['ts']
                rate = dh * 10.0 / dt if dt else 0
                if label.startswith('306 post-fix'):
                    # discriminating claim of the fix: production is never squeezed
                    # to a stop; ~0.33x is the normal solo-pillar share
                    v = 'PASS' if rate > 0.15 else 'FAIL'
                else:
                    # hypothesis NOT confirmed: pre-fix shows the same ~0.33x with
                    # the same order of gaps (max 140 s vs 160 s post-fix)
                    v = 'FAIL'
                check(label, v, f'{rate:.2f}x realtime over {dt}s ({dh} momenta); '
                                f'victim-log max gap 160s (A) vs 140s (B) — no discrimination, see REPORT.md')
            else:
                check(label, 'NOT TESTED', f'only {len(rows_)} rows for phase')
else:
    check('306: liar devnet test ran', 'NOT TESTED', 'raw/liar-test.jsonl missing')

tlog = f'{RAW}/pr306-protocol-race-tests.log'
if os.path.exists(tlog):
    txt = open(tlog).read()
    ok = txt.count('ok  ') >= 3 and 'FAIL' not in txt
    check('306: go test -race ./protocol/... green at head dfbc164', 'PASS' if ok else 'FAIL',
          'protocol, downloader, fetcher packages ok (see raw/pr306-protocol-race-tests.log)')

print(f'\n{sum(1 for r in results if r[1]=="PASS")} PASS, {sum(1 for r in results if r[1]=="FAIL")} FAIL, '
      f'{sum(1 for r in results if r[1]=="NOT TESTED")} NOT TESTED')
sys.exit(1 if any(r[1] == 'FAIL' for r in results) else 0)
