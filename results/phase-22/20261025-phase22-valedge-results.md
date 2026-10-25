# Phase 22: Transaction Validation Edge Cases

**Date:** 2026-10-25
**Devnet:** chain 69, go-zenon `e89f91f` (pinned), single producer, 3 pillars / 1 active
**Method:** `labctl valedge <idx> <qsr>` — sends 14 intentionally malformed transactions, records exact error responses
**Seeds:** 3 (accounts 80, 82, 83; 100 QSR fused each)

## Question

Does the go-zenon validation pipeline correctly reject malformed transactions with informative error messages? Are there any gaps where invalid transactions are accepted?

## Test Cases

### Send-block validation (11 cases)

| # | Name | Field Modified | Expected Error | Result |
|---|------|---------------|----------------|--------|
| 1 | `zero_amount` | Amount = 0 | (accepted — boundary) | ✅ ACCEPTED |
| 2 | `negative_amount` | Amount = -1 | "amount can't be negative" | ✅ REJECTED |
| 3 | `amount_too_big` | Amount = 2^256 | "amount is too big" | ✅ REJECTED |
| 4 | `zero_zts_nonzero_amount` | TokenStandard = zero, Amount = 1 | "zts is missing" | ✅ REJECTED |
| 5 | `nonzero_from_block_hash` | FromBlockHash = random | "from-block-hash must be zero" | ✅ REJECTED |
| 6 | `version_zero` | Version = 0 | "version is missing" | ✅ REJECTED |
| 7 | `version_two` | Version = 2 | "version is invalid" | ✅ REJECTED |
| 8 | `wrong_chain_id` | ChainIdentifier = 999 | "different network Id" | ✅ REJECTED |
| 9 | `block_type_zero` | BlockType = 0 | "type is missing" | ✅ REJECTED |
| 10 | `genesis_receive_type` | BlockType = GenesisReceive | "type must not be genesis" | ✅ REJECTED |
| 11 | `contract_receive_for_user` | BlockType = ContractReceive | "type is not suitable" | ✅ REJECTED |

### Receive-block validation (3 cases)

| # | Name | Field Modified | Expected Error | Result |
|---|------|---------------|----------------|--------|
| 12 | `receive_nonzero_amount` | Amount = 1 | "amount must be zero" | ✅ REJECTED |
| 13 | `receive_nonzero_zts` | TokenStandard = ZNN | "zts must be zero" | ✅ REJECTED |
| 14 | `receive_nonzero_to` | ToAddress = target | "to-address must be zero" | ✅ REJECTED |

## Results (3 seeds)

| Seed | Account | Total | Rejected | Accepted | Error Matches |
|------|---------|-------|----------|----------|---------------|
| 1 | 80 | 14 | 13 | 1 | 13/13 |
| 2 | 82 | 14 | 13 | 1 | 13/13 |
| 3 | 83 | 14 | 13 | 1 | 13/13 |

**100% reproducible across all 3 seeds.**

## Key Findings

1. **All 13 negative test cases are correctly rejected** with informative error messages that match the go-zenon verifier source code exactly. The validation pipeline is robust against malformed transactions.

2. **Zero-amount sends are valid** (boundary case accepted). This is correct behavior — a send with Amount = 0 is technically valid and can be used for contract calls or data-only transactions.

3. **Error messages are descriptive** — each rejection includes the specific field and reason (e.g., "account-block amount can't be negative", "account-block from-block-hash must be zero"). This is good for developer experience.

4. **Chain ID mismatch is caught early** at the RPC layer (`ledger.publishRawTransaction`) before reaching the verifier, with a clear message: "the block has a different network Id (999) from the node (69)".

5. **No validation gaps found** — the verifier's `all()` check sequence (version → chainIdentifier → blockType → amounts → pow → previous → momentumAcknowledged → fromHash → sequencer) correctly rejects all tested malformed inputs.

## Implementation Notes

- **Bug found and fixed during development:** The initial implementation used `rand.Read(randHash.Bytes())` to generate a random hash, but `Bytes()` has a value receiver, so the random bytes were written to a copy, not the original. Fixed to `rand.Read(randHash[:])`.
- The `wrong_chain_id` expected error was updated from "chain-identifier mismatch" (verifier-level) to "different network Id" (RPC-level), reflecting that the RPC layer catches this before the verifier.

## Verifier Source Cross-Reference

All error messages match the go-zenon verifier at the pinned commit `e89f91f`:

| Test Case | Verifier Function | Error Constant |
|-----------|-------------------|----------------|
| negative_amount | `amounts()` | `ErrABAmountNegative` |
| amount_too_big | `amounts()` | `ErrABAmountTooBig` |
| zero_zts_nonzero_amount | `amounts()` | `ErrABZtsMissing` |
| nonzero_from_block_hash | `amounts()` | `ErrABFromBlockHashMustBeZero` |
| version_zero | `version()` | `ErrABVersionMissing` |
| version_two | `version()` | `ErrABVersionInvalid` |
| wrong_chain_id | RPC pre-check | (custom RPC error) |
| block_type_zero | `blockType()` | `ErrABTypeMissing` |
| genesis_receive_type | `blockType()` | `ErrABTypeMustNotBeGenesis` |
| contract_receive_for_user | `blockType()` | `ErrABTypeMustBeUser` |
| receive_nonzero_amount | `amounts()` | `ErrABAmountMustBeZero` |
| receive_nonzero_zts | `amounts()` | `ErrABZtsMustBeZero` |
| receive_nonzero_to | `amounts()` | `ErrABToAddressMustBeZero` |

## Regression Assertion R19

**R19: validation edge cases — negative rejected, zero_amount accepted**

All 14 test cases must be present. All 13 negative cases must be rejected with non-empty error messages. `zero_amount` must be accepted. Error messages must match expected patterns.

## Raw Data

- `results/phase-22/20261025-phase22-valedge-seed1.json`
- `results/phase-22/20261025-phase22-valedge-seed2.json`
- `results/phase-22/20261025-phase22-valedge-seed3.json`
