#!/bin/bash
# Reproduce the DP devnet test run (chain 100).
# Requires: znnd built from ~/go-zenon (DP-capable), datadir ~/znn-dp, dp-test binary.
set -e
RPC=http://127.0.0.1:47997
ADDR="${LAB_ADDR:?set LAB_ADDR}" LAB_ADDR to the devnet test address}
rpc(){ curl -s -m 8 -X POST $RPC -H 'Content-Type: application/json' -d "$1"; echo; }

echo "== DP plasma variables =="
rpc '{"jsonrpc":"2.0","id":1,"method":"embedded.plasma.getVariables","params":[]}'
echo "== frontier momentum (version 2 + next*Price are DP-only fields) =="
rpc '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}'
echo "== plasma pool =="
rpc "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"embedded.plasma.get\",\"params\":[\"$ADDR\"]}"

echo "== T1: fuse (contract call, EmbeddedSimple) =="
./dp-test fuse 500
echo "== T2: 3x plain send (base plasma only) =="
./dp-test burst 3 1
echo "== T3: 3x CancelFuse contract call (EmbeddedWResponse) =="
./dp-test calldata 3

echo "== authoritative per-block plasma =="
rpc "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ledger.getAccountBlocksByPage\",\"params\":[\"$ADDR\",0,20]}" | python3 -c "
import json,sys
d=json.load(sys.stdin)['result']['list']
print(f\"{'h':>3} {'type':>4} {'fused':>8} {'base':>8} {'used':>8} {'difficulty':>11}  hash\")
for b in sorted(d,key=lambda x:x['height']):
    print(f\"{b['height']:>3} {b['blockType']:>4} {b['fusedPlasma']:>8} {b['basePlasma']:>8} {b['usedPlasma']:>8} {b['difficulty']:>11}  {b['hash'][:16]}\")
"
