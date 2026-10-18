// powflood.go — flood the chain with pure-PoW blocks to move nextWorkPrice.
// Run: go run . powflood <numWorkers> <blocksPerWorker>
//
// Strategy:
//  1. Pre-compute numWorkers × blocksPerWorker pure-PoW self-send blocks
//     (difficulty 31,500,000 → 21,000 plasma each), in parallel across workers.
//  2. Publish them all back-to-back so they land in as few momenta as possible.
//  3. Read nextWorkPrice from the frontier momentum after inclusion.
//
// With 10 workers × 5 blocks = 50 blocks × 21,000 = 1,050,000 PoW plasma,
// exactly at PowPlasmaTarget. Should trigger a price change.
package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
)

type precomputedBlock struct {
	block  *nom.AccountBlock
	worker int
	idx    int
}

// rpcRaw2 is a helper that unmarshals the RPC result directly into out.
func rpcRaw2(method string, params interface{}, out interface{}) error {
	res, err := rpcRaw(method, params)
	if err != nil {
		return err
	}
	return json.Unmarshal(res, out)
}

// receiveWithPoW receives all pending blocks for an account, computing PoW if needed.
func receiveWithPoW(a account) int {
	var ul unreceivedList
	must("ledger.getUnreceivedBlocksByAddress", []interface{}{a.addrStr, 0, 50}, &ul)
	count := 0
	for _, ub := range ul.List {
		prevHash, height := frontierAccount(a.addrStr)
		fm := frontierMomentum()
		b := &nom.AccountBlock{
			Version:              1,
			ChainIdentifier:      chainID,
			BlockType:            nom.BlockTypeUserReceive,
			PreviousHash:         prevHash,
			Height:               height + 1,
			MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
			Address:              a.addr,
			FromBlockHash:        ub.Hash,
			Amount:               big.NewInt(0),
			TokenStandard:        types.ZeroTokenStandard,
		}
		req := required(a.addrStr, b.BlockType, nil, nil)
		if req.RequiredDifficulty > 0 {
			// PoW path: no fused plasma, difficulty covers the cost
			b.Difficulty = req.RequiredDifficulty
			dataHash := pow.GetAccountBlockHash(b)
			nonce := pow.GetPoWNonce(big.NewInt(int64(req.RequiredDifficulty)), dataHash)
			copy(b.Nonce.Data[:], nonce)
		} else {
			// Fused path
			b.FusedPlasma = req.BasePlasma
		}
		b.Hash = b.ComputeHash()
		b.PublicKey = a.kp.Public
		b.Signature = a.kp.Sign(b.Hash.Bytes())
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "receive publish err %s: %v\n", a.addrStr[:14], err)
			continue
		}
		// Wait for inclusion
		waitIncluded(a.addrStr, b.Height, 30*time.Second)
		count++
	}
	return count
}

func runPowFlood(numWorkers, blocksPerWorker int) {
	a := agentAccount()

	// Step 0: check agent has enough ZNN for self-sends
	// Each self-send moves 1 ZNN, workers also need ZNN
	fmt.Fprintf(os.Stderr, "=== powflood: %d workers × %d blocks = %d pure-PoW blocks ===\n",
		numWorkers, blocksPerWorker, numWorkers*blocksPerWorker)
	fmt.Fprintf(os.Stderr, "target PoW plasma: %d (target=%d)\n",
		numWorkers*blocksPerWorker*21000, 1050000)

	fm0 := frontierMomentum()
	fmt.Fprintf(os.Stderr, "start: momentum=%d nextWorkPrice=%d nextFusionPrice=%d\n",
		fm0.Height, fm0.NextWorkPrice, fm0.NextFusionPrice)

	// Step 1: fund workers with ZNN (they need ZNN to self-send)
	// Each worker needs blocksPerWorker ZNN (1 per self-send)
	fmt.Fprintf(os.Stderr, "\n--- funding %d workers ---\n", numWorkers)
	for i := 1; i <= numWorkers; i++ {
		w := workerAccount(uint32(i))
		// Check if worker already has enough ZNN
		bal := znnBalance(w.addrStr)
		need := int64(blocksPerWorker)
		if bal >= need {
			fmt.Fprintf(os.Stderr, "worker %d already has %d ZNN (need %d), skipping fund\n", i, bal, need)
			continue
		}
		// Send ZNN from agent to worker
		for {
			b, _, err := buildSend(a, w.addr, znn, dec(need), nil, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agent starved funding worker %d, waiting 30s...\n", i)
				time.Sleep(30 * time.Second)
				continue
			}
			if err := publish(b); err != nil {
				fmt.Fprintf(os.Stderr, "publish fund worker %d: %v\n", i, err)
				time.Sleep(5 * time.Second)
				continue
			}
			_, _, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
			if !ok {
				fmt.Fprintf(os.Stderr, "fund worker %d not included, retrying...\n", i)
				continue
			}
			fmt.Fprintf(os.Stderr, "worker %d funded with %d ZNN\n", i, need)
			break
		}
	}

	// Step 2: pre-compute PoW blocks in parallel
	fmt.Fprintf(os.Stderr, "\n--- pre-computing %d PoW blocks ---\n", numWorkers*blocksPerWorker)
	t0 := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var allBlocks []precomputedBlock
	var computeErrs []error
	receiveCount := make(map[int]int)

	for wIdx := 1; wIdx <= numWorkers; wIdx++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			w := workerAccount(uint32(workerIdx))

			// Worker must receive any pending blocks first (with PoW if needed)
			received := receiveWithPoW(w)
			receiveCount[workerIdx] = received
			if received > 0 {
				fmt.Fprintf(os.Stderr, "worker %d received %d pending blocks\n", workerIdx, received)
			}

			// Get initial frontier for chaining
			prevHash, height := frontierAccount(w.addrStr)
			fm := frontierMomentum()

			var workerBlocks []precomputedBlock
			for bIdx := 0; bIdx < blocksPerWorker; bIdx++ {
				// Build a pure-PoW self-send: 0 fused plasma, difficulty 31,500,000
				b := &nom.AccountBlock{
					Version:              1,
					ChainIdentifier:      chainID,
					BlockType:            nom.BlockTypeUserSend,
					PreviousHash:         prevHash,
					Height:               height + 1,
					MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
					Address:              w.addr,
					ToAddress:            w.addr,
					Amount:               dec(1), // 1 ZNN self-transfer
					TokenStandard:        znn,
					FusedPlasma:          0,
					Difficulty:           31500000, // 21,000 plasma
				}

				// Compute PoW
				tp := time.Now()
				dataHash := pow.GetAccountBlockHash(b)
				nonce := pow.GetPoWNonce(big.NewInt(31500000), dataHash)
				copy(b.Nonce.Data[:], nonce)
				powMs := time.Since(tp).Milliseconds()

				b.Hash = b.ComputeHash()
				b.PublicKey = w.kp.Public
				b.Signature = w.kp.Sign(b.Hash.Bytes())

				// Chain next block locally
				prevHash = b.Hash
				height = b.Height

				workerBlocks = append(workerBlocks, precomputedBlock{block: b, worker: workerIdx, idx: bIdx})
				fmt.Fprintf(os.Stderr, "worker %d block %d: height=%d pow=%dms hash=%s...\n",
					workerIdx, bIdx, b.Height, powMs, b.Hash.String()[:16])
			}

			mu.Lock()
			allBlocks = append(allBlocks, workerBlocks...)
			mu.Unlock()
		}(wIdx)
	}
	wg.Wait()

	elapsed := time.Since(t0)
	fmt.Fprintf(os.Stderr, "pre-computed %d blocks in %v\n", len(allBlocks), elapsed)
	if len(computeErrs) > 0 {
		fmt.Fprintf(os.Stderr, "compute errors: %v\n", computeErrs)
	}

	// Sort blocks by worker, then idx for deterministic publishing
	// (not strictly needed since they're independent chains)
	fmt.Fprintf(os.Stderr, "\n--- publishing %d blocks ---\n", len(allBlocks))
	fmBefore := frontierMomentum()
	fmt.Fprintf(os.Stderr, "momentum before publish: %d\n", fmBefore.Height)

	publishOk := 0
	publishErr := 0
	for _, pb := range allBlocks {
		if err := publish(pb.block); err != nil {
			fmt.Fprintf(os.Stderr, "publish worker %d block %d: %v\n", pb.worker, pb.idx, err)
			publishErr++
		} else {
			publishOk++
		}
	}
	fmt.Fprintf(os.Stderr, "published: %d ok, %d errors\n", publishOk, publishErr)

	// Step 3: poll for inclusion and catch the price change
	fmt.Fprintf(os.Stderr, "\n--- polling for inclusion and price change ---\n")
	publishMomentum := fmBefore.Height
	var inclusionMomentum uint64
	var peakWorkPrice uint64
	var peakFusionPrice uint64

	// Poll every 2 seconds for up to 60 seconds
	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)
		fm := frontierMomentum()

		// Check if workers' blocks are included
		allIncluded := true
		for wIdx := 1; wIdx <= numWorkers; wIdx++ {
			w := workerAccount(uint32(wIdx))
			_, h := frontierAccount(w.addrStr)
			expectedHeight := uint64(blocksPerWorker) + uint64(receiveCount[wIdx])
			if h < expectedHeight {
				allIncluded = false
				break
			}
		}

		// Track peak prices
		if fm.NextWorkPrice > peakWorkPrice {
			peakWorkPrice = fm.NextWorkPrice
		}
		if fm.NextFusionPrice > peakFusionPrice {
			peakFusionPrice = fm.NextFusionPrice
		}

		// If all included and we've passed the inclusion momentum, record it
		if allIncluded && inclusionMomentum == 0 {
			inclusionMomentum = fm.Height
			fmt.Fprintf(os.Stderr, "all blocks included by momentum %d\n", fm.Height)
			fmt.Fprintf(os.Stderr, "frontier: momentum=%d nextWorkPrice=%d nextFusionPrice=%d\n",
				fm.Height, fm.NextWorkPrice, fm.NextFusionPrice)
		}

		// Stop if we've gone past the inclusion momentum and price has fallen back
		if inclusionMomentum > 0 && fm.Height > inclusionMomentum+2 {
			fmt.Fprintf(os.Stderr, "price fell back after momentum %d\n", fm.Height)
			break
		}
	}

	// Also check confirmationDetail for exact inclusion momentum
	for wIdx := 1; wIdx <= numWorkers; wIdx++ {
		w := workerAccount(uint32(wIdx))
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			continue
		}
		var blocks struct {
			List []struct {
				ConfirmationDetail struct {
					MomentumHeight uint64 `json:"momentumHeight"`
				} `json:"confirmationDetail"`
			} `json:"list"`
		}
		rpcRaw2("ledger.getAccountBlocksByHeight", []interface{}{w.addrStr, h, 1}, &blocks)
		if len(blocks.List) > 0 {
			mh := blocks.List[0].ConfirmationDetail.MomentumHeight
			fmt.Fprintf(os.Stderr, "worker %d last block included in momentum %d\n", wIdx, mh)
		}
	}

	// Step 4: final read
	fmAfter := frontierMomentum()
	fmt.Fprintf(os.Stderr, "\n=== RESULTS ===\n")
	fmt.Fprintf(os.Stderr, "publish momentum: %d\n", publishMomentum)
	fmt.Fprintf(os.Stderr, "inclusion momentum: %d\n", inclusionMomentum)
	fmt.Fprintf(os.Stderr, "peak nextWorkPrice: %d (floor=%d)\n", peakWorkPrice, fm0.NextWorkPrice)
	fmt.Fprintf(os.Stderr, "peak nextFusionPrice: %d (floor=%d)\n", peakFusionPrice, fm0.NextFusionPrice)
	fmt.Fprintf(os.Stderr, "final: momentum=%d nextWorkPrice=%d nextFusionPrice=%d\n",
		fmAfter.Height, fmAfter.NextWorkPrice, fmAfter.NextFusionPrice)

	// CSV output
	fmt.Fprintln(os.Stdout, "metric,value")
	fmt.Fprintf(os.Stdout, "workers,%d\n", numWorkers)
	fmt.Fprintf(os.Stdout, "blocks_per_worker,%d\n", blocksPerWorker)
	fmt.Fprintf(os.Stdout, "total_blocks,%d\n", len(allBlocks))
	fmt.Fprintf(os.Stdout, "publish_ok,%d\n", publishOk)
	fmt.Fprintf(os.Stdout, "publish_err,%d\n", publishErr)
	fmt.Fprintf(os.Stdout, "precompute_ms,%d\n", elapsed.Milliseconds())
	fmt.Fprintf(os.Stdout, "momentum_before,%d\n", fmBefore.Height)
	fmt.Fprintf(os.Stdout, "inclusion_momentum,%d\n", inclusionMomentum)
	fmt.Fprintf(os.Stdout, "momentum_after,%d\n", fmAfter.Height)
	fmt.Fprintf(os.Stdout, "nextWorkPrice_before,%d\n", fm0.NextWorkPrice)
	fmt.Fprintf(os.Stdout, "nextWorkPrice_peak,%d\n", peakWorkPrice)
	fmt.Fprintf(os.Stdout, "nextWorkPrice_after,%d\n", fmAfter.NextWorkPrice)
	fmt.Fprintf(os.Stdout, "nextFusionPrice_before,%d\n", fm0.NextFusionPrice)
	fmt.Fprintf(os.Stdout, "nextFusionPrice_peak,%d\n", peakFusionPrice)
	fmt.Fprintf(os.Stdout, "nextFusionPrice_after,%d\n", fmAfter.NextFusionPrice)
}

func znnBalance(addrStr string) int64 {
	var res struct {
		BalanceInfoMap map[string]*struct {
			Balance string `json:"balance"`
		} `json:"balanceInfoMap"`
	}
	must("ledger.getAccountInfoByAddress", []interface{}{addrStr}, &res)
	if info, ok := res.BalanceInfoMap[znn.String()]; ok && info.Balance != "" {
		bal, _ := new(big.Int).SetString(info.Balance, 10)
		if bal != nil {
			return bal.Int64() / 100000000 // 1 ZNN = 1e8
		}
	}
	return 0
}

func powFloodCmd() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: powflood <numWorkers> <blocksPerWorker>\n")
		os.Exit(1)
	}
	nw, _ := strconv.Atoi(os.Args[2])
	bpw, _ := strconv.Atoi(os.Args[3])
	runPowFlood(nw, bpw)
}
