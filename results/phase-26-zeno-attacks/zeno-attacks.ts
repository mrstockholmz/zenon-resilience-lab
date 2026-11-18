// Zeno settlement attack checks — resilience-lab phase 26 (mission M5).
//
// Runs negative ("attack") transactions against a live Zeno devnet and prints
// one line per check: PASS / FAIL / NOT TESTED. PASS means the chain rejected
// the attack (or the invariant holds); FAIL means the attack went through (or
// the check could not run to a verdict); NOT TESTED means the live substrate
// cannot exercise it and the branch's own unit tests are the authority.
//
// This file is the canonical copy in the lab repo. The runner stages it into
// the local zeno-devnet checkout (which holds the npm deps and src/devnet.js)
// and executes it with tsx. It is intentionally standalone: it borrows the
// patterns from zeno-devnet/flows/lib.ts but never touches lib's module-level
// flow state.
//
// Checks (invariants from the M1 review of digitalSloth/go-zenon
// feature/settlement @ 938fca3):
//   spork-gating-active      settlement spork (…0005) is active on this chain
//   deposit-while-spork-off  NOT TESTED (needs governance on the shared devnet)
//   fee-accounting-totals    burned + credited <= collected, rewards <= credited
//   custody-conservation     totalReleased + pendingReserve <= totalDeposited
//   deposit-plasma-limit     Deposit with plasmaLimit > MaxInputGas must be rejected
//   calll2-payload-limit     CallL2 with payload > MaxCallPayload must be rejected
//   batch-replay-executor    executor resubmits an old batch commitment -> reject
//   batch-replay-nonexecutor non-executor SubmitBatch -> reject
//   withdrawal-early-relay   RelayMessage before the batch finalizes -> reject
//   double-withdrawal        second RelayMessage of the same outboxId -> reject
//   release-rate-limit       two ReleaseWithdrawals in one momentum -> 2nd rejects

import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import {
  GatewayClient,
  NotFoundError,
  parseInterface,
  prepareCall,
  prepareDeploy,
  sha3,
  type ContractInterface,
  type ContractsProfile,
  type ReceiptResult,
} from "zeno-client"
import {
  AccountBlockTemplate,
  RPC_MAX_PAGE_SIZE,
  SettlementContract,
  SETTLEMENT_ADDRESS,
  ZNN_ZTS,
  settlement,
  type KeyPair,
} from "znn-typescript-sdk"
import {
  DOMAIN_ID,
  GATEWAY_URL,
  PILLAR_NAME,
  WITHDRAWAL_DELAY,
  connect,
  conservationFailures,
  keyPair,
  readConfiguration,
  sendAndWait,
  waitFor,
} from "../src/devnet.js"

const ONE_ZNN = 100_000_000n
const PLASMA_LIMIT = 10_000_000n // the input gas budget of an honest Deposit/CallL2
const MAX_INPUT_GAS = 100_000_000n // vm/constants/settlement.go MaxInputGas (100M since feature/settlement 2835c436; was 1e9 at 938fca3)
const MAX_CALL_PAYLOAD = 8192 // vm/constants/settlement.go MaxCallPayload
const SETTLEMENT_SPORK_ID = "0000000000000000000000000000000000000000000000000000000000000005"
const ZNN_ASSET = settlement.assetIdFromZts(ZNN_ZTS)
const BALANCE_LEAF_BYTES = 96n // quota model: 64 bytes per leaf + 32-byte value
const IMMUTABLE_METADATA = Buffer.from("0001" + "00".repeat(21), "hex")
const SOURCE_ROOT = process.env.ZENO_DEVNET_SRC ?? new URL("../../../", import.meta.url).pathname

const user = keyPair("user")
const admin = keyPair("admin")
const executorKp = keyPair("executor")

interface Ctx {
  gateway: GatewayClient
  contracts: ContractsProfile
}

const results: { name: string; status: "PASS" | "FAIL" | "NOT TESTED"; detail: string }[] = []

function report(name: string, status: "PASS" | "FAIL" | "NOT TESTED", detail: string): void {
  results.push({ name, status, detail })
  console.log(`${status} ${name}${detail === "" ? "" : ` — ${detail}`}`)
}

async function check(name: string, fn: () => Promise<string>): Promise<void> {
  try {
    report(name, "PASS", await fn())
  } catch (e) {
    report(name, "FAIL", e instanceof Error ? e.message : String(e))
  }
}

const zenon = await connect()
const config = await readConfiguration(zenon)
if (config.missing.length > 0 || config.conflicts.length > 0) {
  throw new Error(`devnet configuration incomplete: ${[...config.conflicts, ...config.missing].join("; ")}`)
}
const gateway = await GatewayClient.connect(GATEWAY_URL, {
  batches: (d, b) => zenon.embedded.settlement.getBatch(d, b),
  sends: (hash) => zenon.ledger.getAccountBlockByHash(hash),
})
const head = await gateway.getHead(DOMAIN_ID)
if (head.status !== "OK") throw new Error(`gateway domain ${DOMAIN_ID} status is ${head.status}`)
const ctx: Ctx = { gateway, contracts: await gateway.contracts(DOMAIN_ID) }

async function conservation(where: string): Promise<void> {
  const failures = await conservationFailures(zenon)
  if (failures.length > 0) throw new Error(`conservation broken after ${where}: ${failures.join("; ")}`)
}

// sendAndWait resolves for accepted sends and for ReceiveBlock failures
// (accepted=false). Pool-level ValidateSendBlock rejections surface as thrown
// node errors; those are the verdict for the plasma/payload/permission checks.
async function send(kp: KeyPair, tx: AccountBlockTemplate, want: boolean): Promise<{ accepted: boolean; hash: string }> {
  const r = await sendAndWait(kp, tx)
  if (want && !r.accepted) throw new Error(`send ${r.hash} was rejected, want acceptance`)
  if (!want && r.accepted) throw new Error(`send ${r.hash} was accepted, want rejection`)
  return { accepted: r.accepted, hash: r.hash.toString() }
}

// A rejection is a thrown error (pool) or a landed block whose receive failed.
async function sendExpectReject(kp: KeyPair, tx: AccountBlockTemplate): Promise<string> {
  try {
    const { hash, accepted } = await sendAndWait(kp, tx)
    if (accepted) throw new Error(`attack send ${hash} was fully accepted`)
    return "rejected: block landed, embedded receive failed"
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    if (msg.includes("timed out")) throw new Error(`inconclusive (infra timeout): ${msg}`)
    return `rejected at pool: ${msg.slice(0, 160)}`
  }
}

async function receiptUntil(label: string, sendHash: string, tag: "latest" | "finalized", timeoutMs: number): Promise<ReceiptResult> {
  const order = { pending: 0, latest: 1, finalized: 2 }
  return waitFor(`${label} receipt at ${tag}`, timeoutMs, async () => {
    let r: ReceiptResult | undefined
    try {
      r = await gateway.getReceipt(sendHash)
    } catch (e) {
      if (!(e instanceof NotFoundError)) throw e
    }
    return r !== undefined && order[r.tag] >= order[tag] ? r : undefined
  })
}

// ---------------------------------------------------------------------------
// Read-only invariant checks
// ---------------------------------------------------------------------------

await check("spork-gating-active", async () => {
  const sporks = await zenon.embedded.spork.getAll()
  const spork = sporks.list.find((s) => s.id.toString() === SETTLEMENT_SPORK_ID)
  assert.ok(spork !== undefined, `settlement spork ${SETTLEMENT_SPORK_ID} not found on chain`)
  assert.ok(spork.activated, "settlement spork exists but is not activated")
  const domain = await zenon.embedded.settlement.getDomainInfo(DOMAIN_ID)
  assert.ok(domain !== null, "settlement domain missing although the spork is active")
  return `spork "${spork.name}" active; domain ${DOMAIN_ID} live`
})

report(
  "deposit-while-spork-off",
  "NOT TESTED",
  "deactivating the spork needs a pillar-signed spork tx on this shared devnet and would halt the executor; the negative side is covered by the branch unit test RejectedBeforeSpork (mutation-verified 2026-11-06)",
)

await check("fee-accounting-totals", async () => {
  const fees = await zenon.embedded.settlement.getFeeInfo()
  assert.ok(
    fees.totalFeesBurned + fees.totalFeesCredited <= fees.totalFeesCollected,
    `burned ${fees.totalFeesBurned} + credited ${fees.totalFeesCredited} > collected ${fees.totalFeesCollected}`,
  )
  assert.ok(fees.totalRewardsCollected <= fees.totalFeesCredited, `rewards ${fees.totalRewardsCollected} > credited ${fees.totalFeesCredited}`)
  await conservation("fee read")
  return `burned=${fees.totalFeesBurned} credited=${fees.totalFeesCredited} collected=${fees.totalFeesCollected} rewards=${fees.totalRewardsCollected}`
})

await check("custody-conservation", async () => {
  const info = await zenon.embedded.settlement.getAssetInfo(DOMAIN_ID, ZNN_ASSET)
  assert.ok(info !== null, "ZNN asset info not found")
  assert.ok(
    info.totalReleased + info.pendingWithdrawalReserve <= info.totalDeposited,
    `released ${info.totalReleased} + reserve ${info.pendingWithdrawalReserve} > deposited ${info.totalDeposited}`,
  )
  return `deposited=${info.totalDeposited} released=${info.totalReleased} reserve=${info.pendingWithdrawalReserve}`
})

// ---------------------------------------------------------------------------
// ValidateSendBlock-layer attacks (pool rejections, no state touched)
// ---------------------------------------------------------------------------

await check("deposit-plasma-limit", async () => {
  const tx = AccountBlockTemplate.callContract(
    SETTLEMENT_ADDRESS,
    ZNN_ZTS,
    ONE_ZNN / 20n,
    SettlementContract.abi.encodeFunctionData("Deposit", [DOMAIN_ID, Buffer.alloc(32), MAX_INPUT_GAS + 1n]),
  )
  return sendExpectReject(user, tx)
})

await check("calll2-payload-limit", async () => {
  const tx = AccountBlockTemplate.callContract(
    SETTLEMENT_ADDRESS,
    ZNN_ZTS,
    0n,
    SettlementContract.abi.encodeFunctionData("CallL2", [DOMAIN_ID, Buffer.alloc(32), PLASMA_LIMIT, Buffer.alloc(MAX_CALL_PAYLOAD + 1)]),
  )
  return sendExpectReject(user, tx)
})

// ---------------------------------------------------------------------------
// Batch replay: rebuild the exact commitment of a seen batch and resubmit it.
// commitment layout: vm/embedded/implementation/settlement.go decodeBatchCommitment
// ---------------------------------------------------------------------------

function buildCommitment(
  b: {
    domainId: number
    batchId: number
    firstInputSeq: number
    lastInputSeq: number
    preStateRoot: { getBytes(): Buffer }
    postStateRoot: { getBytes(): Buffer }
    inputRoot: { getBytes(): Buffer }
    receiptRoot: { getBytes(): Buffer }
    eventRoot: { getBytes(): Buffer }
    outboxRoot: { getBytes(): Buffer }
    daHash: { getBytes(): Buffer }
    daMode: number
    assetFlowSummary: Buffer
  },
  executorAddress: Buffer,
): Buffer {
  assert.equal(executorAddress.length, 20, "executor address is 20 bytes")
  const summary: Buffer = b.assetFlowSummary
  const buf = Buffer.alloc(299 + summary.length + 4)
  for (let i = 0; i < 9; i++) buf.writeUInt16BE(1, 2 * i) // all nine commitment versions are 1 on this devnet
  buf.writeUInt32BE(b.domainId, 18)
  buf.writeBigUInt64BE(BigInt(b.batchId), 22)
  buf.writeBigUInt64BE(BigInt(b.firstInputSeq), 30)
  buf.writeBigUInt64BE(BigInt(b.lastInputSeq), 38)
  ;[b.preStateRoot, b.postStateRoot, b.inputRoot, b.receiptRoot, b.eventRoot, b.outboxRoot, b.daHash].forEach((h, i) =>
    h.getBytes().copy(buf, 46 + 32 * i),
  )
  buf.writeUInt8(b.daMode, 270)
  executorAddress.copy(buf, 271)
  buf.writeBigUInt64BE(0n, 291) // the wire submittedAtHeight must be 0
  summary.copy(buf, 299)
  buf.writeUInt32BE(0, 299 + summary.length) // zero proofData length terminator
  return buf
}

async function latestBatch() {
  const batches = await zenon.embedded.settlement.getBatches(DOMAIN_ID, 0, RPC_MAX_PAGE_SIZE)
  assert.ok(batches.list.length > 0, "no batches on the domain")
  return batches.list.reduce((a, c) => (c.batchId > a.batchId ? c : a))
}

async function executorAddress(): Promise<Buffer> {
  const set = await zenon.embedded.settlement.getExecutorSet()
  const entry = set.list.find((e) => e.pillarName === PILLAR_NAME)
  assert.ok(entry !== undefined, `no executor registered for pillar ${PILLAR_NAME}`)
  return entry.executorAddress.getBytes()
}

function submitBatchTx(commitment: Buffer): AccountBlockTemplate {
  return AccountBlockTemplate.callContract(
    SETTLEMENT_ADDRESS,
    ZNN_ZTS,
    0n,
    SettlementContract.abi.encodeFunctionData("SubmitBatch", [commitment]),
  )
}

// ---------------------------------------------------------------------------
// Withdrawal lifecycle attacks: deploy a vault, deposit, withdraw, then try
// to cheat the delay, replay the relay and beat the release rate limit.
//
// The batch-replay attacks below need at least one existing batch on the
// domain, so they run after this flow has produced one (on a fresh chain
// there are no batches until the first withdrawal is submitted).
// ---------------------------------------------------------------------------

interface Vault {
  iface: ContractInterface
  contractId: HashLike
}

interface HashLike {
  getBytes(): Buffer
  toString(): string
}

async function deployVault(): Promise<Vault> {
  const api = zenon.embedded.settlement
  const dir = `${SOURCE_ROOT}/zeno/zeno-contract-sdk-rs/examples/vault/artifacts`
  const read = (suffix: string) => readFile(`${dir}/vault-testhook.${suffix}`)
  const module = await read("wasm")
  const iface = parseInterface(JSON.parse((await read("interface.json")).toString()))
  prepareDeploy(module, iface, { interfaceEncodings: ctx.contracts.interfaceEncodings })
  const prices = await api.getPeripheryConfig()
  if (prices === null) throw new Error("periphery prices not set")
  const plan = settlement.planDeploy(module, IMMUTABLE_METADATA, { znnPerByteOfBytecode: prices.znnPerByteOfBytecode }, 0)

  const record = () => api.getDeployRecord(DOMAIN_ID, user.address, plan.codeHash)
  if ((await record()) === null) {
    const start = await sendAndWait(user, api.deployContractStart(DOMAIN_ID, plan.codeHash, plan.metadataHash, plan.totalSize, plan.chunkCount, IMMUTABLE_METADATA))
    if (!start.accepted) throw new Error(`deploy start rejected at ${start.hash}`)
  }
  for (const chunk of plan.chunks) {
    const r = await sendAndWait(user, api.deployContractChunk(DOMAIN_ID, plan.codeHash, chunk.index, chunk.chunkHash, chunk.data))
    if (r.accepted) console.log(`ok deploy chunk ${chunk.index} held`)
  }
  const live = await record()
  assert.ok(live !== null && !live.isExpired && live.receivedCount === plan.chunkCount, `deploy record incomplete (received ${live?.receivedCount})`)
  const fin = await sendAndWait(user, api.deployContractFinalize(DOMAIN_ID, plan.codeHash, plan.fee))
  if (!fin.accepted) throw new Error(`deploy finalize rejected at ${fin.hash}`)
  const finalize = await receiptUntil("deploy finalize", fin.hash.toString(), "latest", 300_000)
  assert.equal(finalize.receipt.status, "SUCCESS", "deploy finalize receipt")
  const contractId = finalize.receipt.contractId
  assert.notEqual(contractId, "00".repeat(32), "deploy contractId")
  await conservation("vault deploy")
  console.log(`ok vault-testhook deployed as ${contractId} (code hash ${Buffer.from(sha3(module)).toString("hex").slice(0, 12)}…)`)
  const id: HashLike = { getBytes: () => Buffer.from(contractId, "hex"), toString: () => contractId }
  return { iface, contractId: id }
}

let vault: Vault | undefined
try {
  vault = await deployVault()
  const api = zenon.embedded.settlement

  // buy storage + deposit 1 ZNN (positive control for the deposit path)
  const prices = await api.getPeripheryConfig()
  if (prices === null) throw new Error("periphery prices not set")
  await send(user, api.buyStorage(DOMAIN_ID, vault.contractId as never, BALANCE_LEAF_BYTES, BALANCE_LEAF_BYTES * prices.znnPerByteOfState), true)
  const dep = await sendAndWait(user, api.deposit(DOMAIN_ID, vault.contractId as never, PLASMA_LIMIT, ZNN_ZTS, ONE_ZNN))
  if (!dep.accepted) throw new Error(`deposit rejected at ${dep.hash}`)
  const depReceipt = await receiptUntil("deposit", dep.hash.toString(), "latest", 300_000)
  assert.equal(depReceipt.receipt.status, "SUCCESS", "deposit receipt")
  await conservation("deposit")
  console.log(`ok deposit held: ${dep.hash}`)

  // trigger a withdrawal of 0.4 ZNN to the recipient account
  const recipient = keyPair("recipient").address
  const amount = (2n * ONE_ZNN) / 5n
  const withdrawTx = api.callL2(
    DOMAIN_ID,
    vault.contractId as never,
    PLASMA_LIMIT,
    prepareCall(vault.iface, "withdraw", [ZNN_ASSET.getBytes(), amount, recipient.toString()]),
  )
  const wd = await sendAndWait(user, withdrawTx)
  if (!wd.accepted) throw new Error(`withdraw call rejected at ${wd.hash}`)
  const wdReceipt = await receiptUntil("withdraw", wd.hash.toString(), "latest", 600_000)
  assert.equal(wdReceipt.receipt.status, "SUCCESS", "withdraw receipt")
  assert.equal(wdReceipt.outbox.length, 1, "withdraw outbox items")
  const outboxId = wdReceipt.outbox[0].outboxId
  const batchId = wdReceipt.batchId
  await conservation("withdraw call")
  console.log(`ok withdrawal in flight: outbox ${outboxId} in batch ${batchId}`)

  const outboxIdHash: HashLike = { getBytes: () => Buffer.from(outboxId, "hex"), toString: () => outboxId }
  const relayOf = async (): Promise<AccountBlockTemplate> => {
    const proof = await gateway.getOutboxProof(outboxId)
    const message = Buffer.from(proof.message, "hex")
    const inclusionProof = Buffer.from(proof.inclusionProof, "hex")
    settlement.decodeOutboxWithdrawal(message)
    const root = settlement.outboxProofRoot(message, inclusionProof).toString()
    assert.equal(root, proof.outboxRoot, "rebuilt outbox root")
    const batch = await api.getBatch(DOMAIN_ID, batchId)
    assert.equal(root, batch?.outboxRoot.toString(), "settlement outbox root")
    return api.relayMessage(DOMAIN_ID, BigInt(batchId), message, inclusionProof)
  }

  await check("withdrawal-early-relay", async () => {
    const batch = await api.getBatch(DOMAIN_ID, batchId)
    assert.ok(batch !== null && !batch.isFinalized, `batch ${batchId} already finalized; cannot exercise the delay`)
    return sendExpectReject(user, await relayOf())
  })

  await waitFor(`batch ${batchId} to finalize`, Number(WITHDRAWAL_DELAY) * 10_000 + 300_000, async () =>
    (await api.getBatch(DOMAIN_ID, batchId))?.isFinalized ? true : undefined,
  )
  console.log(`ok batch ${batchId} finalized`)

  await check("double-withdrawal", async () => {
    const first = await send(user, await relayOf(), true)
    assert.ok(await api.isOutboxProcessed(outboxIdHash as never), "outbox not processed after the first relay")
    await conservation("first relay")
    const second = await sendExpectReject(user, await relayOf())
    return `first relay ${first.hash} accepted; second relay ${second}`
  })

  // manual reporting: the rate-limit verdict depends on whether the second
  // call lands in the same momentum as the first (ReleaseIntervalMomentums=1)
  try {
    const state = async (): Promise<string> => {
      if (!(await api.isOutboxProcessed(outboxIdHash as never))) return "unprocessed"
      for (let page = 0; ; page++) {
        const queue = await api.getReleaseQueue(DOMAIN_ID, page, RPC_MAX_PAGE_SIZE)
        if (queue.list.some((e) => e.outboxId.toString() === outboxId)) return "queued"
        if (queue.list.length < RPC_MAX_PAGE_SIZE) return "released"
      }
    }
    if ((await state()) !== "queued") throw new Error("withdrawal not queued after relay")
    const r1 = await sendAndWait(user, api.releaseWithdrawals(DOMAIN_ID))
    if (!r1.accepted) throw new Error(`first releaseWithdrawals rejected at ${r1.hash}`)
    const h1 = r1.receiveHeight
    await waitFor("withdrawal released", 180_000, async () => ((await state()) === "released" ? true : undefined))
    await conservation("first release")
    // second call from a different account so both race the same momentum window
    const r2 = await sendAndWait(admin, api.releaseWithdrawals(DOMAIN_ID))
    if (!r2.accepted && r2.receiveHeight === h1) {
      report("release-rate-limit", "PASS", `second release rejected in the same momentum ${h1}`)
    } else if (r2.accepted && r2.receiveHeight === h1) {
      report("release-rate-limit", "FAIL", `second release ACCEPTED in the same momentum ${h1} — rate limit not enforced`)
    } else {
      report("release-rate-limit", "NOT TESTED", `second release landed in momentum ${r2.receiveHeight}, first in ${h1}; the 1-momentum window had passed (unit tests cover the guard)`)
    }
  } catch (e) {
    report("release-rate-limit", "FAIL", e instanceof Error ? e.message : String(e))
  }
} catch (e) {
  const msg = e instanceof Error ? e.message : String(e)
  for (const pending of ["withdrawal-early-relay", "double-withdrawal", "release-rate-limit", "batch-replay-executor", "batch-replay-nonexecutor"]) {
    if (!results.some((r) => r.name === pending)) report(pending, "FAIL", `setup aborted: ${msg}`)
  }
}

await conservation("final")

await check("batch-replay-executor", async () => {
  const batch = await latestBatch()
  const commitment = buildCommitment(batch, await executorAddress())
  const countBefore = (await zenon.embedded.settlement.getBatches(DOMAIN_ID, 0, RPC_MAX_PAGE_SIZE)).count
  const detail = await sendExpectReject(executorKp, submitBatchTx(commitment))
  const countAfter = (await zenon.embedded.settlement.getBatches(DOMAIN_ID, 0, RPC_MAX_PAGE_SIZE)).count
  assert.equal(countAfter, countBefore, `batch count changed ${countBefore} -> ${countAfter} on a rejected replay`)
  return `replayed batch ${batch.batchId}; ${detail}; batch count unchanged at ${countAfter}`
})

await check("batch-replay-nonexecutor", async () => {
  const batch = await latestBatch()
  const commitment = buildCommitment(batch, await executorAddress())
  const detail = await sendExpectReject(user, submitBatchTx(commitment))
  return `replayed batch ${batch.batchId} from ${user.address.toString()}; ${detail}`
})

console.log("ok conservation held at the end")

const pass = results.filter((r) => r.status === "PASS").length
const fail = results.filter((r) => r.status === "FAIL").length
const notTested = results.filter((r) => r.status === "NOT TESTED").length
console.log(`SUMMARY pass=${pass} fail=${fail} notTested=${notTested} total=${results.length}`)
process.exit(fail > 0 ? 1 : 0)
