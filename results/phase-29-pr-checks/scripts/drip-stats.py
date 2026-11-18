#!/usr/bin/env python3
"""phase29 drip-liar window statistics extractor.

Reads the victim's zenon.log (ground truth, wall-clock) and writes
raw/drip-window-stats.json: per phase window — own-momentum production rate,
gap distribution, sync-session count/duty cycle, Block-import-failed count.

Windows (2026-11-18, local CET timestamps as logged by znnd):
  A2-dripv6   post-fix victim (dfbc164), drip liar v6 active   10:23:00-10:55:00
  B2          pre-fix victim (65240b1), liar never reconnected 10:56:00-19:19:00
  B2b-dripv6  pre-fix victim, drip liar v6 active (restarted)  19:19:00-end

Usage: python3 drip-stats.py [victim-zenon.log]
"""
import json, os, re, sys, datetime, statistics

RAW = os.path.dirname(os.path.abspath(__file__)) + '/../raw'
LOG = sys.argv[1] if len(sys.argv) > 1 else os.path.expanduser(
    '~/lab-devnet-p29/s306v/log/zenon.log')

WINDOWS = [
    ('A2-postfix-dripv6', '2026-11-18T10:23:00', '2026-11-18T10:55:00'),
    ('B2-prefix-disconnected', '2026-11-18T10:56:00', '2026-11-18T19:19:00'),
    ('B2b-prefix-dripv6', '2026-11-18T19:19:00', '2099-01-01T00:00:00'),
]

def ts(s):
    return datetime.datetime.fromisoformat(s).timestamp()

own = re.compile(r'^t=(\S+) lvl=info msg="creating own momentum" .*Height:(\d+)\}')
sync_start = re.compile(r'^t=(\S+) lvl=info msg="Synchronizing with the zenon network"')
sync_done = re.compile(r'^t=(\S+) lvl=info msg="Synchronization completed"')
import_fail = re.compile(r'^t=(\S+) lvl=info msg="Block import failed"')

stats = {}
for name, a, b in WINDOWS:
    stats[name] = {'own': [], 'sync_starts': 0, 'sync_done': 0, 'import_failed': 0}

with open(LOG, errors='replace') as f:
    for line in f:
        m = own.match(line)
        if m:
            tnorm = m.group(1).split('+')[0]
            for name, a, b in WINDOWS:
                if a <= tnorm <= b:
                    stats[name]['own'].append((ts(tnorm), int(m.group(2))))
            continue
        for pat, key in ((sync_start, 'sync_starts'), (sync_done, 'sync_done'),
                         (import_fail, 'import_failed')):
            m = pat.match(line)
            if m:
                tnorm = m.group(1).split('+')[0]
                for name, a, b in WINDOWS:
                    if a <= tnorm <= b:
                        stats[name][key] += 1
                break

out = {}
for name, s in stats.items():
    pts = s['own']
    if len(pts) >= 2:
        t0, t1 = pts[0][0], pts[-1][0]
        h0, h1 = pts[0][1], pts[-1][1]
        dt = t1 - t0
        gaps = sorted(round(pts[i+1][0] - pts[i][0]) for i in range(len(pts)-1))
        out[name] = {
            'height_from': h0, 'height_to': h1, 'span_s': round(dt),
            'mom_per_min': round((h1 - h0) / dt * 60, 3) if dt else 0,
            'n_own_momenta': len(pts),
            'gap_median_s': statistics.median(gaps) if gaps else None,
            'gap_max_s': max(gaps) if gaps else None,
            'sync_starts': s['sync_starts'], 'sync_done': s['sync_done'],
            'import_failed_too_far': s['import_failed'],
        }
    else:
        out[name] = {'n_own_momenta': len(pts), **{k: s[k] for k in
                     ('sync_starts', 'sync_done', 'import_failed_too_far')}}

path = os.path.join(RAW, 'drip-window-stats.json')
with open(path, 'w') as f:
    json.dump(out, f, indent=2)
print(json.dumps(out, indent=2))
print(f'\nwrote {path}')
