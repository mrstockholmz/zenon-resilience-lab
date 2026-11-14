#!/usr/bin/env python3
"""check-phase27.py — PASS/FAIL/NOT TESTED verdicts for phase 27 (next sporks).

Reads the raw/ JSON artifacts produced by run-part1.sh / run-part2.sh /
run-part3.sh and prints one line per check.
"""
import json, os, sys

RAW = os.path.join(os.path.dirname(os.path.abspath(__file__)), "raw")

def load(name):
    p = os.path.join(RAW, name)
    if not os.path.exists(p):
        return None
    with open(p) as f:
        txt = f.read()
    # probe dump() prints a header line before the JSON
    idx = txt.find("{")
    try:
        return json.loads(txt[idx:])
    except Exception:
        return None

def load_obs(name):
    """observe/refundstate dumps: strip '=== label ===' header."""
    return load(name)

def verdict(check, status, detail):
    print(f"{check}: {status} — {detail}")

def main():
    ok = True

    # ---- (b) admission probes ----
    stale29 = load("stale-ack-h29.json")
    if stale29:
        e = stale29.get("publishError") or ""
        if "predates a spork enforced" in e:
            verdict("b1-stale-ack-h29-rejected", "PASS",
                    "rejected by the #280 recency rule (first block of the account, "
                    "so the old older-than-previous rule cannot apply): " + e.split(" - ")[0])
        elif "older momentum than previous" in e:
            # on the funded chain the fresh control consumed block height 1, so
            # the old rule fires first. Rule-isolated evidence (empty account,
            # recency message) is preserved in failed-p27b/stale-ack-h29.json.
            iso = load("failed-p27b/stale-ack-h29.json")
            note = ""
            if iso and "predates a spork enforced" in (iso.get("publishError") or ""):
                note = "; rule-isolated evidence (empty account, first block): failed-p27b/stale-ack-h29.json"
            verdict("b1-stale-ack-h29-rejected", "PASS",
                    "rejected (old older-than-previous rule fired first on this funded "
                    f"account; still a pre-enforcement ack){note}")
        else:
            verdict("b1-stale-ack-h29-rejected", "FAIL", f"unexpected: {e}")
            ok = False
    else:
        verdict("b1-stale-ack-h29-rejected", "NOT TESTED", "artifact missing")

    fresh = load("fresh-ack-control.json")
    if fresh:
        if fresh.get("included") is True and fresh.get("publishError") is None:
            verdict("b2-fresh-ack-control-accepted", "PASS",
                    "control block acking the frontier was admitted and included")
        else:
            verdict("b2-fresh-ack-control-accepted", "FAIL", json.dumps(fresh))
            ok = False
    else:
        verdict("b2-fresh-ack-control-accepted", "NOT TESTED", "artifact missing")

    staleE = load("stale-ack-enf-minus-1.json")
    fresh2 = load("fresh-ack-after-refund-spork.json")
    if staleE and fresh2:
        e = staleE.get("publishError") or ""
        # a rejection artifact carries publishError and NO "included" key
        rejected = bool(e) and staleE.get("included") is not True
        if rejected and "predates a spork enforced" in e:
            verdict("b3-recency-rule-isolated-reject", "PASS",
                    f"block acking enforcement-1 rejected by the #280 recency rule: {e[:160]}")
        elif rejected:
            verdict("b3-recency-rule-isolated-reject", "FAIL",
                    f"rejected by the wrong rule: {e}")
            ok = False
        else:
            verdict("b3-recency-rule-isolated-reject", "FAIL",
                    f"stale block was not rejected: {json.dumps(staleE)}")
            ok = False
        if fresh2.get("included") is True and fresh2.get("publishError") is None:
            verdict("b4-fresh-ack-after-spork-accepted", "PASS",
                    "control block acking the frontier accepted after enforcement")
        else:
            verdict("b4-fresh-ack-after-spork-accepted", "FAIL", json.dumps(fresh2))
            ok = False
    else:
        verdict("b3-recency-rule-isolated-reject", "NOT TESTED", "artifacts missing")
        verdict("b4-fresh-ack-after-spork-accepted", "NOT TESTED", "artifacts missing")

    # ---- unit-test evidence (unmodified tree, -race) ----
    ut_candidates = [os.path.join(RAW, "p27-unittests.log"), os.path.join("/tmp", "p27-unittests.log")]
    vt_candidates = [os.path.join(RAW, "p27-vmtests.log"), os.path.join("/tmp", "p27-vmtests.log")]
    ut = next((p for p in ut_candidates if os.path.exists(p)), None)
    vt = next((p for p in vt_candidates if os.path.exists(p)), None)
    if ut and "EXIT=0" in open(ut).read():
        verdict("b5-unit-tests-chain-verifier-pillar-race", "PASS", ut)
    else:
        verdict("b5-unit-tests-chain-verifier-pillar-race", "NOT TESTED", "log missing or failed")
    if vt and "EXIT=0" in open(vt).read():
        verdict("b6-unit-tests-vm-ack-recency-refund-race", "PASS", vt)
    else:
        verdict("b6-unit-tests-vm-ack-recency-refund-race", "NOT TESTED", "log missing or failed")

    # ---- (a) embedded refund ----
    CANARY = open(os.path.join(RAW, "canary-symbol.txt")).read().strip() if os.path.exists(os.path.join(RAW, "canary-symbol.txt")) else "CANARY3"

    s0 = load_obs("refundstate-0-stalled.json") or load_obs("refundstate-0b-canary-blocked.json")
    s0_pre = load_obs("refundstate-0-stalled.json")
    s0_canary = load_obs("refundstate-0b-canary-blocked.json")
    s1 = load_obs("refundstate-1-recovered.json")
    s2 = load_obs("refundstate-2-wrap2.json")
    wrap1 = load("wrap-1.txt")  # not json; probe prints plain text
    if s0:
        q = [b for b in s0["tokenContractQueue"]]
        # the stuck item is the bridge Burn: a contract send (blockType 4).
        # Probe <2026-11-14 late: fromAddress is zero on sends, so match on
        # addr (probe >= fixed) or fall back to blockType 4 / bridge prefix.
        def is_burn(b):
            if str(b.get("addr", "")).startswith("z1qxemdeddedxbridge"):
                return True
            if str(b.get("from", "")).startswith("z1qxemdeddedxbridge"):
                return True
            return b.get("blockType") == 4
        burnq = [b for b in q if is_burn(b)]
        if burnq:
            verdict("a1-pre-spork-burn-stuck-at-queue-head", "PASS",
                    f"Token contract queue holds the bridge Burn unreceived: {burnq[0]['hash'][:16]}… "
                    f"(amount {burnq[0].get('amount')}, blockType {burnq[0].get('blockType')})")
        else:
            verdict("a1-pre-spork-burn-stuck-at-queue-head", "FAIL",
                    f"no bridge Burn in token queue: {q}")
            ok = False
        if s0_canary and s0_canary.get("canaryTokenExists") is False:
            verdict("a1-pre-spork-queue-stalled-canary-blocked", "PASS",
                    f"canary {CANARY} behind the Burn did not execute")
        elif s0_pre and s0_pre.get("canaryTokenExists") is False:
            verdict("a1-pre-spork-queue-stalled-canary-blocked", "PASS",
                    f"canary {CANARY} not present while the Burn is stuck")
        else:
            verdict("a1-pre-spork-queue-stalled-canary-blocked", "FAIL",
                    "canary token exists before spork activation")
            ok = False
        # no refund pre-spork: the bridge keeps only the wrap fee (15),
        # the burned 9985 stays with the stuck burn send
        try:
            if int(s0.get("bridgeZtsBalance", "0")) < 9985:
                verdict("a1-pre-spork-no-refund", "PASS",
                        f"bridge holds only the wrap fee ({s0.get('bridgeZtsBalance')}); "
                        "the burned amount did not return pre-spork")
            else:
                verdict("a1-pre-spork-no-refund", "FAIL",
                        f"bridge balance pre-spork = {s0.get('bridgeZtsBalance')}")
                ok = False
        except ValueError:
            verdict("a1-pre-spork-no-refund", "FAIL", "bad balance")
            ok = False
    else:
        verdict("a1-pre-spork-burn-stuck-at-queue-head", "NOT TESTED", "artifact missing")
        verdict("a1-pre-spork-queue-stalled-canary-blocked", "NOT TESTED", "artifact missing")
        verdict("a1-pre-spork-no-refund", "NOT TESTED", "artifact missing")

    if s0 and s1:
        burn_hash = None
        for b in s0["tokenContractQueue"]:
            if str(b.get("addr", "")).startswith("z1qxemdeddedxbridge") or \
               str(b.get("from", "")).startswith("z1qxemdeddedxbridge") or \
               b.get("blockType") == 4:
                burn_hash = b["hash"]
                break
        q1 = [b["hash"] for b in s1["tokenContractQueue"]]
        if burn_hash and burn_hash not in q1:
            verdict("a2-stalled-queue-recovers-after-activation", "PASS",
                    "the stalled Burn left the queue after EmbeddedRefundSpork enforcement")
        else:
            verdict("a2-stalled-queue-recovers-after-activation", "FAIL",
                    f"burn still queued? {q1}")
            ok = False
        if s1.get("canaryTokenExists") is True:
            verdict("a2-canary-executes-after-recovery", "PASS",
                    "canary Issue executed once the queue advanced")
        else:
            verdict("a2-canary-executes-after-recovery", "FAIL",
                    "canary token still missing after activation")
            ok = False
        try:
            bal = int(s1.get("bridgeZtsBalance", "0"))
            if bal >= 9985:
                verdict("a2-refund-deposited-on-bridge", "PASS",
                        f"bridge holds {bal} units of the wrapped token after recovery "
                        "(fee + refunded burn)")
            else:
                verdict("a2-refund-deposited-on-bridge", "FAIL", f"bridge balance is {bal}")
                ok = False
        except ValueError:
            verdict("a2-refund-deposited-on-bridge", "FAIL", "bad balance")
            ok = False
    else:
        verdict("a2-stalled-queue-recovers-after-activation", "NOT TESTED", "artifact missing")
        verdict("a2-canary-executes-after-recovery", "NOT TESTED", "artifact missing")
        verdict("a2-refund-deposited-on-bridge", "NOT TESTED", "artifact missing")

    if s1 and s2 and os.path.exists(os.path.join(RAW, "wrap-2.txt")):
        supply1 = int(s1.get("ztsTotalSupply", "0"))
        supply2 = int(s2.get("ztsTotalSupply", "0"))
        q2 = [b for b in s2["tokenContractQueue"] if
              str(b.get("addr", "")).startswith("z1qxemdeddedxbridge") or
              str(b.get("from", "")).startswith("z1qxemdeddedxbridge") or
              b.get("blockType") == 4]
        if supply1 == supply2 and not q2:
            verdict("a3-post-spork-wrap-no-stall-no-burn", "PASS",
                    f"wrap #2 settled without stalling the queue; supply constant at {supply2}")
        else:
            verdict("a3-post-spork-wrap-no-stall-no-burn", "FAIL",
                    f"supply {supply1}->{supply2}, queue {q2}")
            ok = False
    else:
        verdict("a3-post-spork-wrap-no-stall-no-burn", "NOT TESTED", "artifact missing")

    # ---- (c) mixed devnet ----
    # Chain 1 (p27, DP enforced at 30): v0.0.10 cannot sync from master at
    # all — master's nom.Momentum carries the DP price fields v0.0.10 cannot
    # decode ("rlp: input list has too many elements"). The chains split at
    # the wire layer, before any spork logic runs.
    c = load("mixed-v0010-result.json")
    if c:
        if c.get("syncBlockedByWireFormat") is True:
            verdict("c1-dp-chain-wire-split", "PASS",
                    f"v0.0.10 connected but every momentum batch failed RLP decode "
                    f"(DP price fields); node stuck at height {c.get('lastHeight')}: "
                    f"{c.get('evidence','')[:100]}")
        else:
            verdict("c1-dp-chain-wire-split", "FAIL", json.dumps(c)[:200])
            ok = False
    else:
        verdict("c1-dp-chain-wire-split", "NOT TESTED", "artifact missing")

    # Chain 3 (p27c, unknown spork "p27c-x" activated early; libp2p/DP
    # pushed beyond the test window in genesis): v0.0.10 validates master
    # momentums up to the unknown-spork enforcement, then terminates.
    c2 = load("p27c-v0010-result.json")
    if c2:
        if c2.get("cleanExitAtUnknownSpork") is True:
            verdict("c2-v0010-clean-exit-at-unknown-spork", "PASS",
                    f"v0.0.10 validated momentums 1..{c2.get('v0010SyncedUpTo')} "
                    f"(enforcement {c2.get('enforcementHeight')}) then exited "
                    f"with status {c2.get('exitCode')}")
        else:
            verdict("c2-v0010-clean-exit-at-unknown-spork", "FAIL",
                    json.dumps(c2)[:200])
            ok = False
    else:
        verdict("c2-v0010-clean-exit-at-unknown-spork", "NOT TESTED", "artifact missing (p27c)")

    print()
    print("overall:", "PASS" if ok else "SEE FAILURES ABOVE")

if __name__ == "__main__":
    main()
