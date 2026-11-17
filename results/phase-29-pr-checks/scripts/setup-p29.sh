#!/bin/bash
# phase29 setup: fresh 3-pillar devnet from the go-zenon PR #310 head (bb1984fa)
# with the PillarJailSpork (id ...0008) pre-activated at genesis.
# Data dirs: ~/lab-devnet-p29/{n1,n2,n3}; chain id 70; RPC 47971-3; listen 56971-3.
set -euo pipefail

P29=~/lab-devnet-p29
RAW=~/zenon-resilience-lab/results/phase-29-pr-checks/raw
mkdir -p "$P29"/{n1,n2,n3}/wallet "$RAW"

# --- genesis ---
python3 - << 'EOF'
import json, time
g = json.load(open('/home/mrstockholmez/lab-devnet/genesis.json'))
g['ChainIdentifier'] = 70
g['ExtraData'] = 'LAB P29 PR310 pillar-jail'
g['GenesisTimestampSec'] = int(time.time()) - 30
g['SporkConfig']['Sporks'].append({
    'id': '0'*63 + '8',
    'name': 'pillar-jail',
    'description': 'PillarJailSpork (phase29 devnet, activated at genesis)',
    'activated': True,
    'enforcementHeight': 0,
})
out = '/home/mrstockholmez/zenon-resilience-lab/results/phase-29-pr-checks/raw/genesis-p29.json'
json.dump(g, open(out, 'w'), indent=1)
print('genesis written:', out)
EOF

# --- data dirs ---
declare -A KEY
KEY[n1]=z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq
KEY[n2]=z1qz9zr5c7a0p8qljvrwt2cy5j99w98c5myrn2un
KEY[n3]=z1qzedcjmds6cwuqu7wkrvl0dadwwauzaana6g8e
declare -A SRCW
SRCW[n1]=/home/mrstockholmez/lab-devnet/wallet
SRCW[n2]=/home/mrstockholmez/lab-devnet-pillar2/wallet
SRCW[n3]=/home/mrstockholmez/lab-devnet-pillar3/wallet
declare -A PORT=([n1]=1 [n2]=2 [n3]=3)

for n in n1 n2 n3; do
  d="$P29/$n"
  cp "$RAW/genesis-p29.json" "$d/genesis.json"
  cp "${SRCW[$n]}/${KEY[$n]}" "$d/wallet/"
  python3 - "$d" "${KEY[$n]}" "5697${PORT[$n]}" "4797${PORT[$n]}" << 'PYEOF'
import json, sys
d, addr, listen, rpc = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
p = f"{d}/wallet/{addr}"
k = json.load(open(p)); k["Path"] = p; json.dump(k, open(p, "w"), indent=4)
cfg = {
  "DataPath": d,
  "GenesisFile": f"{d}/genesis.json",
  "LogLevel": "info",
  "Name": f"lab-p29-{d.split('/')[-1]}",
  "Net": {
    "BootstrapPeers": [],
    "ListenHost": "127.0.0.1",
    "ListenPort": listen,
    "MinConnectedPeers": 0,
    "MinPeers": 0,
    "Seeders": [],
    "P2PBackend": "libp2p"
  },
  "Producer": {
    "Address": addr,
    "Index": 0,
    "KeyFilePath": addr,
    "Password": "devnet"
  },
  "RPC": {
    "EnableHTTP": True,
    "EnableWS": False,
    "Endpoints": [],
    "HTTPCors": [],
    "HTTPHost": "127.0.0.1",
    "HTTPPort": rpc,
    "HTTPVirtualHosts": ["localhost"],
    "WSHost": "127.0.0.1",
    "WSOrigins": [],
    "WSPort": rpc + 1000
  },
  "WalletPath": f"{d}/wallet"
}
json.dump(cfg, open(f"{d}/config.json", "w"), indent=4)
print("configured", d, listen, rpc)
PYEOF
done
echo "setup done"
