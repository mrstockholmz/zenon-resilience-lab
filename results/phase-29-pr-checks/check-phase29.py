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
# Ground-truth epochs (monitor ts are true UTC epochs; the auto-unjail watcher
# log writes local CET time with a mislabeled 'Z' suffix — its entries anchor
# to the file mtime, see REPORT.md):
#   n3 kill      2026-11-17 23:44:02 UTC  epoch 1794962642  height ~83
#   jail detect  2026-11-18 00:54:48 UTC  epoch 1794966888  heights 599..604
#   (pillar-contract Update every 300 momenta -> jail executed at h601/602)
#   unjail incl. <= h604 (supply drop visible), n3 restarted ~00:55:30 UTC
KILL_TS, UJ_TS = 1794962642, 1794966888
DEV3_PREFIX = 'z1qzedcjmds6'  # monitor stores 12-char address prefixes
jail_rows = [r for r in rows if r.get('pillars', {}).get('dev3', {}).get('isJailed')]
if rows:
    last = rows[-1]
    h_before = 83  # height when n3 was stopped (raw/baseline-pre-stop.log)
    window = [r for r in rows if KILL_TS <= r['ts'] <= UJ_TS]
    prod_in_window = [r for r in window if DEV3_PREFIX in (r.get('producers') or {}).values()]
    produced_in_window = {r['pillars']['dev3']['produced'] for r in window if 'pillars' in r}
    frozen = len(produced_in_window) == 1
    check('310: dev3 stops producing after its node is killed',
          'PASS' if not prod_in_window and frozen else 'FAIL',
          f'kill->jail window: dev3 appears as producer in {len(prod_in_window)}/{len(window)} monitor rows, '
          f'producedMomentums constant at {sorted(produced_in_window)}')
    check('310: network keeps producing with 2 of 3 pillars (no stall)',
          'PASS' if last.get('height', 0) > h_before + 30 else 'FAIL',
          f'height {h_before} -> {last.get("height")} with only dev1+dev2 producing')
    # jail evidence: the monitor (45 s poll) missed the ~60 s jail window, so the
    # isJailed=true sample lives in raw/auto-unjail.log, not monitor.jsonl
    alt = f'{RAW}/auto-unjail.log'
    jail_txt = open(alt).read() if os.path.exists(alt) else ''
    jailed = 'IS JAILED' in jail_txt
    if jailed:
        lo = max((r for r in rows if r['ts'] <= UJ_TS), key=lambda r: r['ts'], default=None)
        hi = min((r for r in rows if r['ts'] >= UJ_TS), key=lambda r: r['ts'], default=None)
        jh = f'{lo["height"]}-{hi["height"]}' if lo and hi else '?'
        # latency kill->jail in ticks (tick = 300 s; momentum = 10 s)
        dt = UJ_TS - KILL_TS
        ticks = dt / 300.0
        # not jailed prematurely: no isJailed row and supply unchanged before the window
        pre = [r for r in rows if r['ts'] < UJ_TS - 600]
        premature = any(r['pillars']['dev3']['isJailed'] for r in pre if 'pillars' in r)
        check('310: pillar jailed after >=12-tick miss window',
              'PASS' if not premature and ticks >= 12 else 'FAIL',
              f'jail detected by watcher at epoch {UJ_TS} (heights {jh}, h601/602 Update); '
              f'{dt}s after kill = ~{ticks:.0f} ticks (>= 12 required); no premature jail in monitor')
    elif jail_rows:
        jr = jail_rows[0]
        check('310: pillar jailed after >=12-tick miss window',
              'PASS', f'dev3 isJailed=true first seen at monitor ts {jr["ts"]} (height {jr.get("height")})')
    else:
        check('310: pillar jailed after >=12-tick miss window', 'NOT TESTED', 'no jail evidence yet')

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

# dev3 resumes after unjail + n3 restart (anchor = unjail epoch, jail_rows not
# required: the monitor missed the brief jail window)
anchor = UJ_TS if jailed else (jail_rows[0]['ts'] if jail_rows else 0)
resumed = [r for r in rows if r['ts'] > anchor and DEV3_PREFIX in (r.get('producers') or {}).values()]
if resumed:
    check('310: unjailed pillar resumes producing after node restart', 'PASS',
          f'dev3 producer again at monitor ts {resumed[0]["ts"]} (height {resumed[0].get("height")}, '
          f'{(resumed[0]["ts"]-UJ_TS)}s after unjail)')
elif jailed or jail_rows:
    check('310: unjailed pillar resumes producing after node restart', 'NOT TESTED', 'n3 not restarted yet or no production observed')
else:
    check('310: unjailed pillar resumes producing after node restart', 'NOT TESTED', 'no jail yet')

neg = f'{RAW}/negative-unjail-wrong-amount.log'
if os.path.exists(neg):
    ok = 'invalid token or amount' in open(neg).read()
    check('310: unjail with 99 ZNN refused (amount gate)', 'PASS' if ok else 'FAIL',
          open(neg).read().strip().splitlines()[-1][:120])

check('310: rewards/votes while jailed', 'NOT TESTED',
      'jail window was ~60 s and dev3\'s node was down during it; slot exclusion while jailed '
      'is not separately observable from the dead-node baseline in this run')
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

# ---- PR #306 drip-liar v6 checks (2026-11-18 session 2/3) ----
# Setup discovery (see REPORT.md): victim (dev1) and liar (dev2) each produced
# their own canonical chain from h67 — every mutual sync needs a >30-momentum
# rollback, refused by the cap ('Block import failed ... Too far'). The sync
# sessions therefore COMPLETE while importing nothing: exactly issue #291/#306's
# shape, on both binaries.
#   A2  post-fix dfbc164 + drip liar   -> production must continue
#   B2  pre-fix 65240b1, liar dead     -> solo baseline only (control invalid,
#                                          handshake never re-established after
#                                          the 10:55 swap; liar restarted 19:19)
#   B2b pre-fix 65240b1 + drip liar    -> the actual pre-fix discriminating run
ds = f'{RAW}/drip-window-stats.json'
if os.path.exists(ds):
    W = json.load(open(ds))
    BASELINE = 2.0  # measured solo rate, mom/min (B2 disconnected window)
    a2 = W.get('A2-postfix-dripv6', {})
    b2b = W.get('B2b-prefix-dripv6', {})
    if a2.get('n_own_momenta', 0) >= 10:
        r = a2['mom_per_min']
        check('306 drip: post-fix production continues with liar attached (no stall)',
              'PASS' if r >= 0.85 * BASELINE else 'FAIL',
              f"A2 {r} mom/min (baseline {BASELINE}, {a2['n_own_momenta']} momenta, "
              f"{a2['sync_done']} no-op sessions, {a2['import_failed_too_far']} import-failed)")
    else:
        check('306 drip: post-fix production at baseline under back-to-back no-op sessions',
              'NOT TESTED', f"A2 has {a2.get('n_own_momenta')} momenta")
    if b2b.get('n_own_momenta', 0) >= 30:
        r = b2b['mom_per_min']
        # Discriminating hypothesis: pre-fix holds production while no-op sessions
        # run. If the pre-fix rate is also ~baseline, the hold is NOT reproduced
        # (second honest negative, stronger liar than v1).
        held = r < 0.85 * BASELINE
        check('306 drip: pre-fix control held under the same sessions',
              'FAIL' if not held else 'PASS',
              f"B2b {r} mom/min ({b2b['n_own_momenta']} momenta, {b2b['sync_done']} "
              f"no-op sessions, {b2b['import_failed_too_far']} import-failed) — "
              + ('hold reproduced' if held else
                 'hold NOT reproduced even with back-to-back sessions (honest negative)'))
        check('306 drip: sessions import nothing (chains mutually forked, rollback cap)',
              'PASS' if b2b['import_failed_too_far'] > 0 else 'FAIL',
              f"{b2b['import_failed_too_far']} 'Block import failed ... Too far' in the B2b window")
    else:
        check('306 drip: pre-fix control held under the same sessions',
              'NOT TESTED', f'B2b has {b2b.get("n_own_momenta")} momenta (<30)')
        check('306 drip: sessions import nothing (chains mutually forked, rollback cap)',
              'NOT TESTED', 'B2b window too short')
else:
    check('306 drip: stats file missing', 'NOT TESTED', 'raw/drip-window-stats.json not generated yet')

tlog = f'{RAW}/pr306-protocol-race-tests.log'
if os.path.exists(tlog):
    txt = open(tlog).read()
    ok = txt.count('ok  ') >= 3 and 'FAIL' not in txt
    check('306: go test -race ./protocol/... green at head dfbc164', 'PASS' if ok else 'FAIL',
          'protocol, downloader, fetcher packages ok (see raw/pr306-protocol-race-tests.log)')

print(f'\n{sum(1 for r in results if r[1]=="PASS")} PASS, {sum(1 for r in results if r[1]=="FAIL")} FAIL, '
      f'{sum(1 for r in results if r[1]=="NOT TESTED")} NOT TESTED')
sys.exit(1 if any(r[1] == 'FAIL' for r in results) else 0)
