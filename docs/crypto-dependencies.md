# Cryptographic Dependency Inventory — go-zenon @ e89f91f

**Date:** 2026-10-24
**Pinned commit:** `e89f91f68b922a65370b7ffd9cfe860045a99b29`
**Purpose:** Inventory all cryptographic primitives used in the pinned go-zenon code.
Facts with file:line references. No speculation.

---

## 1. Signature Scheme — Ed25519

Account blocks and momentums both use Ed25519 signatures.

| File | Line | Code | Description |
|------|------|------|-------------|
| `wallet/keypair.go` | 4 | `"crypto/ed25519"` | Import |
| `wallet/keypair.go` | 13-14 | `Public ed25519.PublicKey` / `Private ed25519.PrivateKey` | KeyPair struct fields |
| `wallet/keypair.go` | 21 | `ed25519.Sign(kp.Private, message)` | Sign — account block / momentum signature |
| `wallet/keypair.go` | 27-28 | `func (kp *KeyPair) Signer(...)` | Signer callback for `vm.SignFunc` |
| `wallet/crypto.go` | 84-88 | `func VerifySignature(...)` / `ed25519.Verify(...)` | Verify — used by transaction verifier |
| `verifier/account_block.go` | 458 | `wallet.VerifySignature(block.PublicKey, block.Hash.Bytes(), block.Signature)` | Account block signature verification |
| `verifier/momentum.go` | 411 | `wallet.VerifySignature(momentum.PublicKey, momentum.Hash.Bytes(), momentum.Signature)` | Momentum signature verification |
| `chain/nom/account_block.go` | 177 | `PublicKey ed25519.PublicKey` | Account block carries Ed25519 public key |
| `chain/nom/momentum.go` | 73 | `PublicKey ed25519.PublicKey` | Momentum carries Ed25519 public key |
| `wallet/derivation.go` | 41 | `ed25519.GenerateKey(bytes.NewReader(k.Key))` | Key generation from SLIP-0010 derived seed |

**Signing flow:** `pillar/worker_momentum.go:64` → `GenerateMomentum(detailed, w.coinbase.Signer)` → `vm/supervisor.go:383` → `signFunc(momentum.Hash.Bytes())` → `wallet/keypair.go:27` → `ed25519.Sign()`.

---

## 2. Hash Functions

All primary hash functions defined in `common/crypto/hash.go`:

| File | Line | Code | Description |
|------|------|------|-------------|
| `common/crypto/hash.go` | 21 | `"crypto/sha256"` | SHA-256 import |
| `common/crypto/hash.go` | 23 | `"golang.org/x/crypto/sha3"` | SHA3/Keccak import |
| `common/crypto/hash.go` | 29-34 | `func Hash(data ...[]byte)` / `sha3.New256()` | **SHA3-256 (FIPS-202)** — canonical protocol hash |
| `common/crypto/hash.go` | 41-46 | `func HashSHA256(data ...[]byte)` / `sha256.New()` | **SHA-256** — HTLC hash locks, swap |
| `common/crypto/hash.go` | 54-60 | `func Keccak256(data ...[]byte)` / `sha3.NewLegacyKeccak256()` | **Keccak-256** — Ethereum compat, devp2p, bridge EVM |

### Usage locations:

| File | Line | Hash Used | Purpose |
|------|------|-----------|---------|
| `common/types/hash.go` | 26-28 | SHA3-256 | `types.NewHash()` — all block/momentum hashes |
| `common/types/address.go` | 212 | SHA3-256 | `PubKeyToAddress()` — address derivation |
| `common/types/tokenstandard.go` | 46-47 | SHA3-256 | `NewZenonTokenStandard()` — ZTS token IDs |
| `common/db/patch.go` | 194-195 | SHA3-256 | `PatchHash()` — ChangesHash for blocks/momentums |
| `chain/nom/account_block.go` | 281-299 | SHA3-256 | `AccountBlock.ComputeHash()` |
| `chain/nom/momentum.go` | 104-122 | SHA3-256 | `Momentum.ComputeHash()` |
| `pow/pow.go` | 36 | SHA3-256 | PoW data hash |
| `pow/pow.go` | 61,118 | SHA3-256 | PoW nonce hashing |
| `vm/abi/abi.go` | 25 | SHA3-256 | Method selectors (NOT Keccak like Ethereum) |
| `vm/abi/method.go` | 51-53 | SHA3-256 | `Method.Id()` — 4-byte selector |
| `vm/embedded/implementation/htlc.go` | 313-316 | SHA3-256 + SHA-256 | HTLC hash lock verification (two types) |
| `vm/embedded/implementation/bridge.go` | 299 | Keccak-256 | `GetMessageToSignEvm()` — Ethereum personal-sign |
| `vm/embedded/implementation/bridge.go` | 307-312 | SHA3-256 / Keccak-256 | `HashByNetworkClass()` — TSS message hashing |
| `vm/embedded/implementation/swap_utils.go` | 50-53 | SHA-256 + RIPEMD-160 | `PubKeyToKeyId()` — Bitcoin-style key ID |
| `vm/embedded/implementation/swap_utils.go` | 109-110 | Double SHA-256 | `GetSwapMessage()` — legacy swap digest |
| `vm/embedded/definition/ptlc.go` | 263-271 | SHA3-256 | `GetPtlcUnlockMessage()` — PTLC domain-separated hash |
| `p2p/discover/udp.go` | 478,487,526,530 | Keccak-256 | devp2p node discovery packet signing/MAC |
| `p2p/discover/table.go` | 207,221 | Keccak-256 | Kademlia bucket SHA |
| `p2p/legacy/rlpx.go` | 256-262 | Keccak-256 | RLPx key derivation (AES key, MAC secret) |
| `wallet/derivation.go` | 98,120 | SHA-512 | HMAC-SHA512 for SLIP-0010 key derivation |

---

## 3. Producer Keys — BIP-39 mnemonic → SLIP-0010 Ed25519 HD derivation

Producer keys are standard wallet `KeyPair`s derived from the node's encrypted keystore.

| File | Line | Code | Description |
|------|------|------|-------------|
| `node/config.go` | 307-348 | `func (c *Config) parseProducer(...)` | Loads producer key from wallet KeyFile |
| `node/config.go` | 313 | `walletManager.GetKeyFile(c.Producer.KeyFilePath)` | Reads encrypted key file |
| `node/config.go` | 317 | `walletManager.Unlock(...)` | Decrypts with Argon2id + AES-256-GCM |
| `node/config.go` | 338 | `keyStore.DeriveForIndexPath(c.Producer.Index)` | Derives Ed25519 key at BIP-44 path |
| `pillar/manager.go` | 27 | `coinbase *wallet.KeyPair` | Pillar manager holds the coinbase key |
| `pillar/manager.go` | 144-146 | `func (m *manager) SetCoinBase(...)` | Sets producer key for momentum signing |
| `pillar/worker_momentum.go` | 64 | `w.supervisor.GenerateMomentum(detailed, w.coinbase.Signer)` | Signs momentum with coinbase key |
| `wallet/derivation.go` | 23 | `ZenonAccountPathFormat = "m/44'/73404'/%d'"` | BIP-44 path, coin type 73404, all hardened |
| `wallet/derivation.go` | 28 | `seedModifier = "ed25519 seed"` | SLIP-0010 HMAC key |
| `wallet/derivation.go` | 97-108 | `newMasterKey()` / `hmac.New(sha512.New, []byte(seedModifier))` | SLIP-0010 master key derivation |
| `wallet/derivation.go` | 110-130 | `func (k *key) derive(i uint32)` | SLIP-0010 child key derivation (hardened only) |
| `wallet/keystore.go` | 7 | `"github.com/tyler-smith/go-bip39"` | BIP-39 library |
| `wallet/keystore.go` | 30 | `bip39.NewMnemonic(entropy)` | Mnemonic from entropy |
| `wallet/keystore.go` | 37 | `bip39.NewSeed(mnemonic, "")` | BIP-39 seed (no passphrase) |

**Key format on disk:** JSON KeyFile with AES-256-GCM encrypted BIP-39 entropy, Argon2id KDF.

---

## 4. Bridge Keys — secp256k1 ECDSA (TSS orchestrator key)

| File | Line | Code | Description |
|------|------|------|-------------|
| `vm/embedded/definition/bridge.go` | 335-336 | `CompressedTssECDSAPubKey` / `DecompressedTssECDSAPubKey` | Bridge stores TSS public key (base64 secp256k1) |
| `vm/embedded/implementation/bridge.go` | 15 | `"github.com/ethereum/go-ethereum/crypto/secp256k1"` | secp256k1 import |
| `vm/embedded/implementation/bridge.go` | 39-64 | `func CheckECDSASignature(...)` | Verifies recoverable secp256k1 sig over TSS-signed messages |
| `vm/embedded/implementation/bridge.go` | 56 | `secp256k1.RecoverPubkey(message, signature)` | ECDSA public key recovery |
| `vm/embedded/implementation/bridge.go` | 294-299 | `func GetMessageToSignEvm(...)` | Ethereum personal-sign envelope over Keccak256 |
| `vm/embedded/implementation/bridge.go` | 307-315 | `func HashByNetworkClass(...)` | NoM→SHA3-256, EVM→Keccak256 personal-sign |
| `vm/embedded/implementation/bridge.go` | 1443-1446 | `secp256k1.DecompressPubkey(pubKey)` | TSS key validation |
| `vm/embedded/implementation/bridge.go` | 1497 | `secp256k1.DecompressPubkey(pubKey)` | TSS key decompression |

---

## 5. Address Derivation — SHA3-256(pubkey)[0:19] with class byte, bech32 encoded

| File | Line | Code | Description |
|------|------|------|-------------|
| `common/types/address.go` | 30 | `AddressPrefix = "z"` | bech32 HRP |
| `common/types/address.go` | 33 | `AddressSize = 1 + AddressCoreSize` | 20 bytes total |
| `common/types/address.go` | 37 | `AddressCoreSize = 19` | 19-byte core |
| `common/types/address.go` | 43-46 | `UserAddrByte = 0` / `ContractAddrByte = 1` | Class bytes |
| `common/types/address.go` | 211-218 | `func PubKeyToAddress(pubKey []byte) Address` | Address derivation: `UserAddrByte \|\| SHA3-256(pubkey)[0:19]` |
| `common/types/address.go` | 212 | `hash := sha3.Sum256(pubKey)` | SHA3-256 of public key |
| `common/types/address.go` | 22 | `"github.com/btcsuite/btcd/btcutil/bech32"` | bech32 library |
| `common/types/address.go` | 153-158 | `func (addr Address) String()` | bech32 encoding ("z1...") |
| `common/types/address.go` | 264-273 | `func parseBech32(...)` | bech32 decoding |

---

## 6. Proof of Work — SHA3-256 partial preimage search

| File | Line | Code | Description |
|------|------|------|-------------|
| `pow/pow.go` | 35-37 | `func GetAccountBlockHash(...)` | PoW data hash: SHA3-256(address \|\| prevHash) |
| `pow/pow.go` | 44-49 | `func CheckPoWNonce(...)` | Verification: checks nonce meets difficulty |
| `pow/pow.go` | 57-69 | `func GetPoWNonce(...)` | Mining: brute-force nonce search |
| `pow/pow.go` | 61 | `crypto.Hash(calc)` | SHA3-256 of nonce(8B) \|\| dataHash(32B) |
| `pow/pow.go` | 114-119 | `func hashWithNonce(...)` | Hash computation: `crypto.Hash(calc)[:8]` |
| `pow/pow.go` | 104-112 | `func GetThresholdByDifficulty(...)` | Threshold = 2^64 - (2^64 / difficulty) |
| `pow/pow.go` | 82-86 | `func Uint64ToByteArray(...)` | Little-endian uint64 encoding |

PoW checks the **first 8 bytes** of SHA3-256(nonce ‖ dataHash), read as little-endian uint64, against the threshold.

---

## 7. Merkle Trees / Proofs — None found

No Merkle tree implementation exists in the codebase. Grep for `merkle|Merkle` returned zero matches in Go source files.

---

## 8. RLP Encoding — go-ethereum/rlp

| File | Line | Code | Description |
|------|------|------|-------------|
| `go.mod` | 10 | `github.com/ethereum/go-ethereum v1.10.22` | RLP library source |
| `protocol/handler.go` | 25 | `"github.com/ethereum/go-ethereum/rlp"` | Import |
| `protocol/handler.go` | 239 | `func gatherBlocksForReply(msgStream *rlp.Stream, ...)` | RLP stream decoding of block requests |
| `protocol/handler.go` | 273 | `rlp.EncodeToBytes(block)` | RLP encoding for block replies |
| `protocol/handler.go` | 366,381,396,424 | `rlp.NewStream(msg.Payload, ...)` | RLP stream creation for message dispatch |
| `p2p/message.go` | 29 | `"github.com/ethereum/go-ethereum/rlp"` | Import |
| `p2p/message.go` | 96 | `rlp.EncodeToReader(data)` | RLP encoding for wire messages |
| `p2p/message.go` | 259 | `rlp.EncodeToBytes(content)` | RLP content encoding |
| `p2p/libp2p/stream_rw.go` | 10 | `"github.com/ethereum/go-ethereum/rlp"` | Import |
| `p2p/libp2p/stream_rw.go` | 167 | `func RlpEncode(...)` / `rlp.EncodeToReader(data)` | RLP encoding for libp2p wire format |
| `p2p/libp2p/peer.go` | 28 | `"github.com/ethereum/go-ethereum/rlp"` | Import — handshake |
| `p2p/legacy/rlpx.go` | 40 | `"github.com/ethereum/go-ethereum/rlp"` | Import — RLPx framing |
| `p2p/legacy/peer.go` | 28 | `"github.com/ethereum/go-ethereum/rlp"` | Import |
| `p2p/discover/udp.go` | 28 | `"github.com/ethereum/go-ethereum/rlp"` | Import — node discovery |
| `p2p/discover/database.go` | 33 | `"github.com/ethereum/go-ethereum/rlp"` | Import — node DB |
| `chain/nom/momentum.go` | 65,72,76-77 | `` `rlp:"-"` `` / `` `rlp:"optional"` `` | RLP struct tags on Momentum |
| `chain/account_pool.go` | 8 | `"github.com/ethereum/go-ethereum/rlp"` | Import — wire size validation |
| `pillar/content_selector.go` | 8 | `"github.com/ethereum/go-ethereum/rlp"` | Import — RLP size budget enforcement |

---

## 9. Key Derivation / Wallet Encryption — Argon2id + AES-256-GCM + BIP-39 + SLIP-0010

| File | Line | Code | Description |
|------|------|------|-------------|
| `wallet/password.go` | 5 | `"golang.org/x/crypto/argon2"` | Argon2 import |
| `wallet/password.go` | 18 | `argon2.IDKey([]byte(password), h.salt, 1, 64*1024, 4, 32)` | **Argon2id**: time=1, memory=64MB, threads=4, keyLen=32 |
| `wallet/password.go` | 25 | `argon2.IDKey(...)` (same params) | Decryption key derivation |
| `wallet/crypto.go` | 19 | `"crypto/aes"` | AES import |
| `wallet/crypto.go` | 20 | `"crypto/cipher"` | GCM import |
| `wallet/crypto.go` | 32-46 | `func aesGCMEncrypt(...)` | **AES-256-GCM** encryption |
| `wallet/crypto.go` | 47-64 | `func aesGCMDecrypt(...)` | AES-256-GCM decryption |
| `wallet/crypto.go` | 29 | `gcmAdditionData = "zenon"` | GCM associated data |
| `wallet/crypto.go` | 42 | `GetEntropyCSPRNG(12)` | 12-byte GCM nonce |
| `wallet/crypto.go` | 71-78 | `func GetEntropyCSPRNG(n int)` | CSPRNG from `crypto/rand` |
| `wallet/keyfile.go` | 14 | `aesMode = "aes-256-gcm"` | Cipher identifier |
| `wallet/keyfile.go` | 15 | `argonName = "argon2.IDKey"` | KDF identifier |
| `wallet/keyfile.go` | 45-47 | `type argon2Params struct { Salt ... }` | Salt storage |
| `wallet/keyfile.go` | 98-110 | `func (kf *KeyFile) Decrypt(...)` | Full decryption pipeline |
| `wallet/keystore.go` | 113-138 | `func (ks *KeyStore) Encrypt(...)` | Full encryption pipeline |
| `wallet/derivation.go` | 23 | `ZenonAccountPathFormat = "m/44'/73404'/%d'"` | BIP-44 path |
| `wallet/derivation.go` | 97-108 | `func newMasterKey(seed)` | SLIP-0010 master: HMAC-SHA512("ed25519 seed", seed) |
| `wallet/derivation.go` | 110-130 | `func (k *key) derive(i)` | SLIP-0010 child: HMAC-SHA512(chainCode, 0x00 \|\| key \|\| index) |
| `wallet/keystore.go` | 30 | `bip39.NewMnemonic(entropy)` | BIP-39 mnemonic |
| `wallet/keystore.go` | 37 | `bip39.NewSeed(mnemonic, "")` | BIP-39 seed (PBKDF2-HMAC-SHA512 inside library) |

---

## 10. PTLC (Point Time-Locked Contract) — Three point types

| File | Line | Code | Description |
|------|------|------|-------------|
| `vm/embedded/implementation/ptlc.go` | 9 | `"filippo.io/edwards25519"` | Edwards25519 curve ops |
| `vm/embedded/implementation/ptlc.go` | 10-11 | `"github.com/btcsuite/btcd/btcec/v2"` / `".../schnorr"` | secp256k1 + BIP-340 Schnorr |
| `vm/embedded/implementation/ptlc.go` | 57-63 | `edwards25519.Point.SetBytes()` / `MultByCofactor()` | Ed25519 small-order point check |
| `vm/embedded/implementation/ptlc.go` | 67-80 | `func verifyBIP340Signature(...)` | BIP-340 Schnorr signature verification |
| `vm/embedded/implementation/ptlc.go` | 82-97 | `func verifyPointScalar(...)` | secp256k1 scalar x G = point verification |
| `vm/embedded/implementation/ptlc.go` | 191-200 | `PointTypeED25519` case | Ed25519 signature PTLC unlock |
| `vm/embedded/implementation/ptlc.go` | 201-203 | `PointTypeBIP340` case | BIP-340 Schnorr PTLC unlock |
| `vm/embedded/implementation/ptlc.go` | 204-205 | `PointTypeSecp256k1Point` case | Hash-lock style PTLC (scalar preimage) |

---

## 11. P2P Transport Crypto

| File | Line | Code | Description |
|------|------|------|-------------|
| `p2p/legacy/rlpx.go` | 38 | `"github.com/ethereum/go-ethereum/crypto/ecies"` | **ECIES** encryption (legacy RLPx handshake) |
| `p2p/legacy/rlpx.go` | 256-262 | `crypto.Keccak256(...)` for AES key, MAC secret | RLPx key derivation via Keccak256 |
| `p2p/legacy/rlpx.go` | 266-269 | `sha3.New256()` for MACs | RLPx frame MAC |
| `p2p/libp2p/server.go` | 47 | `"github.com/libp2p/go-libp2p/p2p/security/noise"` | **Noise protocol** (XX pattern) for libp2p transport |
| `p2p/config.go` | 205 | `privateKey *ecdsa.PrivateKey` | Node identity: **secp256k1 ECDSA** key |
| `p2p/libp2p/identity.go` | 16-27 | `func ECDSAToLibp2pPrivKey(...)` | Converts secp256k1 ECDSA to libp2p key |

---

## 12. HTLC (Hash Time-Locked Contract)

| File | Line | Code | Description |
|------|------|------|-------------|
| `vm/embedded/definition/htlc.go` | 89-92 | `HashTypeSHA3 = 0` / `HashTypeSHA256 = 1` | Two hash lock types |
| `vm/embedded/implementation/htlc.go` | 313-316 | `crypto.Hash()` / `crypto.HashSHA256()` | Hash lock verification |

---

## 13. Legacy Swap (secp256k1)

| File | Line | Code | Description |
|------|------|------|-------------|
| `vm/embedded/implementation/swap_utils.go` | 10 | `"github.com/ethereum/go-ethereum/crypto/secp256k1"` | secp256k1 import |
| `vm/embedded/implementation/swap_utils.go` | 46-54 | `func PubKeyToKeyId(...)` | RIPEMD-160(SHA-256(compressed secp256k1 pubkey)) |
| `vm/embedded/implementation/swap_utils.go` | 73,86 | `secp256k1.Sign(message, prv)` | Swap message signing |
| `vm/embedded/implementation/swap_utils.go` | 167 | `secp256k1.RecoverPubkey(message, sig)` | Swap signature recovery |
| `vm/embedded/implementation/swap_utils.go` | 105-112 | `func GetSwapMessage(...)` | Double-SHA-256 of length-prefixed strings |

---

## go.mod — Direct Crypto-Relevant Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `golang.org/x/crypto` | v0.31.0 | SHA3-256, Keccak-256, Argon2id, RIPEMD-160 |
| `filippo.io/edwards25519` | v1.1.0 | Edwards25519 point ops (PTLC small-order check) |
| `github.com/btcsuite/btcd/btcec/v2` | v2.2.0 | secp256k1 curve, BIP-340 Schnorr (PTLC) |
| `github.com/btcsuite/btcd/btcutil` | v1.1.3 | bech32 encoding (addresses) |
| `github.com/ethereum/go-ethereum` | v1.10.22 | RLP, secp256k1 signing/recovery, ECIES, ABI |
| `github.com/tyler-smith/go-bip39` | v1.1.0 | BIP-39 mnemonic generation |
| `github.com/decred/dcrd/dcrec/secp256k1/v4` | v4.2.0 (indirect) | secp256k1 via btcec |
| `github.com/decred/dcrd/crypto/blake256` | v1.0.1 (indirect) | Blake-256 (btcec dep) |
| `lukechampine.com/blake3` | v1.2.1 (indirect) | BLAKE3 (libp2p dep, unused in go-zenon code) |
| `github.com/minio/sha256-simd` | v1.0.1 (indirect) | SHA-256 SIMD (libp2p dep) |
