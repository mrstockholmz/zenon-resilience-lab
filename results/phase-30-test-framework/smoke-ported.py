#!/usr/bin/env python3
"""Read-only smoke test for the docker devnet (docker-compose.yml at the repo root).

Checks that the five nodes are on one chain, that all three pillars produce on the
10 s cadence, that the genesis sporks activated, that every node reports sync done,
and that one getter per RPC namespace answers. With --sync it also starts nothing but
compares the late joiner (port 35999) against the RPC node.

Standard library only. Exit code 0 when every check passes, 1 otherwise.

  python3 test/devnet/smoke.py                 # wait for height 45, then check
  python3 test/devnet/smoke.py --height 100 --sync --out results.json
"""
import os
import argparse, json, subprocess, sys, time, urllib.request

NODES = ["pillar", "pillar2", "pillar3", "rpc", "observer"]
DEV = "z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e"  # devnet account, index 3
GETTERS = [
    ("embedded.pillar.getAll", [0, 10]),
    ("embedded.pillar.getQsrRegistrationCost", []),
    ("embedded.plasma.get", [DEV]),
    ("embedded.plasma.getVariables", []),
    ("embedded.token.getAll", [0, 10]),
    ("embedded.sentinel.getAllActive", [0, 10]),
    ("embedded.stake.getEntriesByAddress", [DEV, 0, 10]),
    ("embedded.accelerator.getAll", [0, 10]),
    ("embedded.swap.getLegacyPillars", []),
    ("embedded.spork.getAll", [0, 20]),
    ("embedded.bridge.getBridgeInfo", []),
    ("embedded.liquidity.getLiquidityInfo", []),
    ("ledger.getAccountInfoByAddress", [DEV]),
    ("ledger.getDetailedMomentumsByHeight", [1, 5]),
    ("ledger.getMomentumsByPage", [0, 5]),
    ("stats.processInfo", []),
    ("stats.osInfo", []),
]


def host(method, params=None, port=int(os.environ.get("SMOKE_RPC_PORT","35997"))):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params or []}).encode()
    req = urllib.request.Request(f"http://localhost:{port}", body, {"Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req, timeout=10))


def inside(node, method, params=None):
    """Call a node that publishes no host port, through docker exec."""
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params or []})
    out = subprocess.run(
        ["docker", "exec", f"znnd-devnet-{node}", "wget", "-qO-", "--post-data", body,
         "--header", "Content-Type: application/json", "http://localhost:35997"],
        capture_output=True, text=True, timeout=15)
    return json.loads(out.stdout) if out.stdout else {"error": out.stderr.strip()}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--height", type=int, default=45, help="wait for this frontier height first")
    ap.add_argument("--sync", action="store_true", help="also compare the late joiner on port 35999")
    ap.add_argument("--out", help="write results as JSON to this file")
    args = ap.parse_args()
    results = []

    def check(case, ok, detail):
        results.append({"case": case, "pass": bool(ok), "detail": detail})
        print("PASS" if ok else "FAIL", case, detail[:300])

    while True:
        h = host("ledger.getFrontierMomentum")["result"]["height"]
        if h >= args.height:
            break
        time.sleep(10)

    at = h - 2
    hashes = {}
    for n in NODES:
        lst = (inside(n, "ledger.getMomentumsByHeight", [at, 1]).get("result") or {}).get("list") or [{}]
        hashes[n] = lst[0].get("hash")
    check("one-chain", len(set(hashes.values())) == 1 and None not in hashes.values(), f"height {at}: {hashes}")

    ms = host("ledger.getMomentumsByHeight", [2, h - 1])["result"]["list"]
    producers = {}
    for m in ms:
        producers[m["producer"]] = producers.get(m["producer"], 0) + 1
    check("three-producers", len(producers) == 3, json.dumps(producers))
    gaps = [(a["height"], b["timestamp"] - a["timestamp"]) for a, b in zip(ms, ms[1:])]
    late = [g for g in gaps if g[1] != 10]
    check("cadence-10s", not late, f"{len(gaps)} intervals, not 10 s after heights: {late}")

    sporks = {s["name"]: s for s in host("embedded.spork.getAll", [0, 20])["result"]["list"]}
    versions = {}
    for m in ms:
        versions.setdefault(m["version"], []).append(m["height"])
    dp = sporks.get("dynamic-plasma", {}).get("enforcementHeight")
    v2 = min(versions.get(2, [0]))
    check("dynamic-plasma-version-2", dp is not None and v2 == dp + 1,
          f"enforcement {dp}, first version-2 momentum {v2}")
    for name in ("az", "htlc", "libp2p", "dynamic-plasma"):
        check(f"spork-{name}", sporks.get(name, {}).get("activated") is True, json.dumps(sporks.get(name)))

    for n in NODES:
        si = inside(n, "stats.syncInfo").get("result")
        ni = inside(n, "stats.networkInfo").get("result") or {}
        check(f"sync-{n}", bool(si) and si["state"] == 2 and ni.get("numPeers", 0) >= 2,
              f"{si} peers={ni.get('numPeers')}")

    for method, params in GETTERS:
        r = host(method, params)
        check("rpc:" + method, "error" not in r, json.dumps(r.get("error") or r.get("result"))[:200])

    if args.sync:
        si = host("stats.syncInfo", port=int(os.environ.get("SMOKE_SYNC_PORT","35999")))["result"]
        ni = host("stats.networkInfo", port=int(os.environ.get("SMOKE_SYNC_PORT","35999")))["result"]
        a = host("ledger.getMomentumsByHeight", [at, 1])["result"]["list"]
        b = host("ledger.getMomentumsByHeight", [at, 1], port=int(os.environ.get("SMOKE_SYNC_PORT","35999")))["result"]["list"]
        check("late-joiner-synced", si["state"] == 2 and ni["numPeers"] >= 1, f"{si} peers={ni['numPeers']}")
        check("late-joiner-same-chain", bool(a) and bool(b) and a[0]["hash"] == b[0]["hash"],
              f"height {at}: rpc {a[0]['hash'] if a else None} joiner {b[0]['hash'] if b else None}")

    failed = [r for r in results if not r["pass"]]
    version = host("stats.processInfo")["result"]
    print(f"\n{len(results) - len(failed)}/{len(results)} passed at height {h} on {version['version']}")
    if args.out:
        json.dump({"version": version, "height": h, "results": results}, open(args.out, "w"), indent=1)
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
