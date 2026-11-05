module zvmplasmaprobe

go 1.25.0

replace github.com/ethereum/go-ethereum => github.com/ethereum-optimism/op-geth v1.101702.2

require github.com/Zenoglyphs-VM/zvm-core v0.0.0

require (
	github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime v0.0.0-20251001021608-1fe7b43fc4d6 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/ethereum/go-ethereum v0.0.0-00010101000000-000000000000 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	golang.org/x/crypto v0.44.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/Zenoglyphs-VM/zvm-core => /home/mrstockholmez/zvm-core
