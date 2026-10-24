// labctl — Zenon Resilience Lab workload generator and measurement tool.
//
// Uses only disposable devnet identities from the committed go-zenon devnet
// mnemonic. Never touches real wallets or shared networks.
//
// Commands:
//   info                              node state summary
//   ordinary <n> <amount>             sequential self-transfers (ordinary workload)
//   burst <n> <amount>                rapid-fire self-transfers without waiting (burst workload)
//   waitfor <height>                  block until momentum height >= target
//   probe <seconds>                   lightly-provisioned probe: periodic small sends
//   fuse <qsr>                        fuse QSR for plasma on the agent account
//   fund <index> <znn>                send ZNN from agent to derived account[index]
//   fusefor <index> <qsr>             fuse QSR with beneficiary=derived account[index]
//   matrix <case> <seed>              run one phase-3 matrix case
//   contend <numAccounts> <txPerAccount> <amount>  multi-sender contention experiment (phase 7)
//   powburst <numAccounts> <txPerAcct> <amount>    PoW repricing experiment (phase 9)
//   mixcontend <fused> <unfused> <txPerAcct> <amt>  mixed fused/unfused contention (phase 10)
//   mixsaturate <fused> <unfused> <txPerAcct> <amt> mixed fused/unfused saturation (phase 13)
package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/wallet"
)

const (
	rpcURL  = "http://127.0.0.1:57997"
	chainID = 69
)

var (
	znn = types.ZnnTokenStandard
	qsr = types.QsrTokenStandard
)

// ---- RPC helpers ----

func rpc(method string, params interface{}, out interface{}) error {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	resp, err := http.Post(rpcURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf, &envelope); err != nil {
		return fmt.Errorf("rpc %s: bad response: %s", method, buf)
	}
	if envelope.Error != nil {
		return fmt.Errorf("rpc %s error: %s", method, envelope.Error.Message)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("rpc %s decode: %v raw=%s", method, err, envelope.Result)
		}
	}
	return nil
}

func mustRPC(method string, params interface{}, out interface{}) {
	if err := rpc(method, params, out); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL rpc %s: %v\n", method, err)
		os.Exit(1)
	}
}

// ---- Node state ----

type momentumInfo struct {
	Hash            types.Hash `json:"hash"`
	Height          uint64     `json:"height"`
	Timestamp       uint64     `json:"timestamp"`
	NextFusionPrice uint64     `json:"nextFusionPrice"`
	NextWorkPrice   uint64     `json:"nextWorkPrice"`
}

func frontierMomentum() momentumInfo {
	var m momentumInfo
	mustRPC("ledger.getFrontierMomentum", []interface{}{}, &m)
	return m
}

type frontierBlock struct {
	Hash   types.Hash `json:"hash"`
	Height uint64     `json:"height"`
}

func frontierAccount(addr string) (types.Hash, uint64) {
	var fb *frontierBlock
	mustRPC("ledger.getFrontierAccountBlock", []string{addr}, &fb)
	if fb == nil {
		return types.ZeroHash, 0
	}
	return fb.Hash, fb.Height
}

type requiredPlasma struct {
	AvailablePlasma    uint64 `json:"availablePlasma"`
	BasePlasma         uint64 `json:"basePlasma"`
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
}

func requiredPoW(addrStr string, blockType uint64, to *types.Address, data []byte) requiredPlasma {
	param := map[string]interface{}{"address": addrStr, "blockType": blockType}
	if to != nil {
		param["toAddress"] = to.String()
	}
	if data != nil {
		param["data"] = data
	}
	var r requiredPlasma
	mustRPC("embedded.plasma.getRequiredPoWForAccountBlock", []interface{}{param}, &r)
	return r
}

func plasmaLevel(addrStr string) uint64 {
	var info struct {
		CurrentPlasma uint64 `json:"currentPlasma"`
	}
	mustRPC("embedded.plasma.get", []string{addrStr}, &info)
	return info.CurrentPlasma
}

// ---- Accounts ----

type account struct {
	addr    types.Address
	addrStr string
	kp      *wallet.KeyPair
	index   uint32
}

var cachedKS *wallet.KeyStore

func keystore() *wallet.KeyStore {
	if cachedKS != nil {
		return cachedKS
	}
	// Use the committed devnet keyfile (disposable, public mnemonic)
	kf, err := wallet.ReadKeyFile(os.Getenv("HOME") + "/lab-devnet/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL read keyfile: %v\n", err)
		os.Exit(1)
	}
	ks, err := kf.Decrypt("devnet")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL decrypt: %v\n", err)
		os.Exit(1)
	}
	cachedKS = ks
	return ks
}

var agentIdx uint32 = 3

func agentAccount() account {
	// Default: index 3 (z1qp3yph55...) — the general dev account with genesis funding.
	// Index 0 is the pillar producer and has no ZNN/QSR balance.
	// Override with LAB_AGENT_IDX env var for low-fused tests on other accounts.
	if v := os.Getenv("LAB_AGENT_IDX"); v != "" {
		if idx, err := strconv.ParseUint(v, 10, 32); err == nil {
			return derivedAccount(uint32(idx))
		}
	}
	return derivedAccount(agentIdx)
}

func derivedAccount(index uint32) account {
	ks := keystore()
	_, kp, err := ks.DeriveForIndexPath(index)
	if err != nil {
		panic(err)
	}
	return account{kp.Address, kp.Address.String(), kp, index}
}

// ---- Block building ----

func buildSend(a account, to types.Address, zts types.ZenonTokenStandard, amount *big.Int, data []byte, allowPoW bool) (*nom.AccountBlock, int64, error) {
	prevHash, height := frontierAccount(a.addrStr)
	fm := frontierMomentum()
	b := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      chainID,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         prevHash,
		Height:               height + 1,
		MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
		Address:              a.addr,
		ToAddress:            to,
		Amount:               amount,
		TokenStandard:        zts,
		Data:                 data,
	}
	req := requiredPoW(a.addrStr, b.BlockType, &to, data)
	if req.RequiredDifficulty > 0 {
		if !allowPoW {
			return nil, 0, fmt.Errorf("plasma starved (need difficulty %d)", req.RequiredDifficulty)
		}
		b.Difficulty = req.RequiredDifficulty
		t0 := time.Now()
		dataHash := pow.GetAccountBlockHash(b)
		nonce := pow.GetPoWNonce(big.NewInt(int64(req.RequiredDifficulty)), dataHash)
		copy(b.Nonce.Data[:], nonce)
		powMs := time.Since(t0).Milliseconds()
		b.Hash = b.ComputeHash()
		b.PublicKey = a.kp.Public
		b.Signature = a.kp.Sign(b.Hash.Bytes())
		return b, powMs, nil
	}
	b.FusedPlasma = req.BasePlasma
	b.Hash = b.ComputeHash()
	b.PublicKey = a.kp.Public
	b.Signature = a.kp.Sign(b.Hash.Bytes())
	return b, 0, nil
}

func publish(b *nom.AccountBlock) error {
	return rpc("ledger.publishRawTransaction", []interface{}{b.ToNomMarshalJson()}, nil)
}

func waitIncluded(addrStr string, height uint64, timeout time.Duration) (time.Duration, uint64, bool) {
	start := time.Now()
	for time.Since(start) < timeout {
		_, h := frontierAccount(addrStr)
		if h >= height {
			fm := frontierMomentum()
			return time.Since(start), fm.Height, true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return time.Since(start), 0, false
}

func dec(x int64) *big.Int { return big.NewInt(x * 100_000_000) }

// ---- Result types ----

type txResult struct {
	Idx            int    `json:"idx"`
	BlockHeight    uint64 `json:"block_height"`
	SubmitMomentum uint64 `json:"submit_momentum"`
	PoWMs          int64  `json:"pow_ms"`
	Included       bool   `json:"included"`
	LatencyMs      int64  `json:"latency_ms"`
	InclMomentum   uint64 `json:"included_momentum"`
	PlasmaAfter    uint64 `json:"plasma_after"`
	Error          string `json:"error,omitempty"`
}

type runSummary struct {
	Workload       string     `json:"workload"`
	PlasmaRegime   string     `json:"plasma_regime"`
	Attempted      int        `json:"attempted"`
	Submitted      int        `json:"submitted"`
	Included       int        `json:"included"`
	Rejected       int        `json:"rejected"`
	Unresolved     int        `json:"unresolved"`
	PoWCount       int        `json:"pow_count"`
	TotalPoWMs     int64      `json:"total_pow_ms"`
	LatenciesMs    []int64    `json:"latencies_ms"`
	P50Ms          int64      `json:"p50_ms"`
	P90Ms          int64      `json:"p90_ms"`
	P99Ms          int64      `json:"p99_ms"`
	StartMomentum  uint64     `json:"start_momentum"`
	EndMomentum    uint64     `json:"end_momentum"`
	DurationSec    float64    `json:"duration_sec"`
	GeneratorCPU   float64    `json:"generator_cpu_sec"`
	Results        []txResult `json:"results"`
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func summarize(workload string, results []txResult, startM, endM uint64, duration float64, cpuSec float64) runSummary {
	s := runSummary{
		Workload:      workload,
		Attempted:     len(results),
		StartMomentum: startM,
		EndMomentum:   endM,
		DurationSec:   duration,
		GeneratorCPU:  cpuSec,
		Results:       results,
	}
	if endM < 30 {
		s.PlasmaRegime = "static"
	} else {
		s.PlasmaRegime = "dynamic"
	}
	var lats []int64
	for _, r := range results {
		if r.Error != "" {
			s.Rejected++
			continue
		}
		s.Submitted++
		if r.Included {
			s.Included++
			lats = append(lats, r.LatencyMs)
		} else {
			s.Unresolved++
		}
		if r.PoWMs > 0 {
			s.PoWCount++
			s.TotalPoWMs += r.PoWMs
		}
	}
	s.LatenciesMs = lats
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	s.P50Ms = percentile(lats, 0.50)
	s.P90Ms = percentile(lats, 0.90)
	s.P99Ms = percentile(lats, 0.99)
	return s
}

func printSummaryJSON(s runSummary) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(s)
}

// ---- CPU measurement ----

// getCPUTime reads /proc/self/stat to get user+system CPU time in seconds
func getCPUTime() float64 {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return 0
	}
	utime, _ := strconv.ParseFloat(fields[13], 64)
	stime, _ := strconv.ParseFloat(fields[14], 64)
	// Convert from clock ticks to seconds (assume 100 Hz)
	return (utime + stime) / 100.0
}

// ---- Phase 12: Saturation ----

type saturateBreakdown struct {
	AccountIndex    int    `json:"account_index"`
	Address         string `json:"address"`
	FusedQSR        int64  `json:"fused_qsr"`
	PlasmaAvailable uint64 `json:"plasma_available"`
	Attempted       int    `json:"attempted"`
	Submitted       int    `json:"submitted"`
	Included        int    `json:"included"`
	Rejected        int    `json:"rejected"`
	Unresolved      int    `json:"unresolved"`
}

// runSaturate: saturation experiment (phase 12).
//
// N accounts with EQUAL plasma each send M self-transfers as fast as possible.
// Total = N*M, designed to exceed the 200 tx/momentum limit.
// Measures: per-momentum inclusion, queue drain rate, backlog clearance.
//
// Uses fresh derived accounts starting at index 100 to avoid contamination
// from previous phases.
func runSaturate(numAccounts int, txPerAccount int, qsrPerAccount int64, amt int64) {
	baseIdx := uint32(100)

	// ---- Setup: fund all accounts ----
	fmt.Fprintf(os.Stderr, "saturate setup: funding %d accounts (index %d-%d)...\n", numAccounts, baseIdx, baseIdx+uint32(numAccounts)-1)
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "  funding account %d (%s)...\n", idx, w.addrStr)
			runFund(idx, 100)
		}
	}

	// Wait for funding inclusion
	targetHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "saturate setup: waiting for funding inclusion (height %d)...\n", targetHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= targetHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// ---- Setup: fuse equal QSR for all accounts (before receive, so receive txs have plasma) ----
	fmt.Fprintf(os.Stderr, "saturate setup: fusing %d QSR for %d accounts...\n", qsrPerAccount, numAccounts)
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		fmt.Fprintf(os.Stderr, "  fusing %d QSR for account %d...\n", qsrPerAccount, idx)
		runFuseFor(idx, qsrPerAccount)
	}

	// Wait for fusion inclusion
	fuseHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "saturate setup: waiting for fusion inclusion (height %d)...\n", fuseHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= fuseHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Receive funded blocks (now accounts have plasma from fusion, no PoW needed)
	fmt.Fprintf(os.Stderr, "saturate setup: receiving funded blocks...\n")
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		receiveAll(w)
	}

	// Record plasma levels
	plasmaLevels := make(map[uint32]uint64)
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		plasmaLevels[idx] = plasmaLevel(w.addrStr)
		fmt.Fprintf(os.Stderr, "  account %d: plasma=%d\n", idx, plasmaLevels[idx])
	}

	totalTxs := numAccounts * txPerAccount
	fmt.Fprintf(os.Stderr, "saturate: starting saturation (%d accounts x %d txs = %d total, limit=200/momentum)...\n",
		numAccounts, txPerAccount, totalTxs)

	// ---- Saturation phase ----
	fmBefore := frontierMomentum()
	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Each account sends txPerAccount txs as fast as possible.
	// Accounts run in parallel. Fire-and-forget, verify at end.
	for i := 0; i < numAccounts; i++ {
		wg.Add(1)
		go func(idx uint32) {
			defer wg.Done()
			w := derivedAccount(idx)
			for j := 0; j < txPerAccount; j++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: int(idx)*100000 + j, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: int(idx)*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: int(idx)*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true, // optimistic; verified below
				})
				mu.Unlock()
			}
		}(baseIdx + uint32(i))
	}
	wg.Wait()

	// Wait for all transactions to be included (may take multiple momenta)
	fmt.Fprintf(os.Stderr, "saturate: waiting 60s for all inclusions (backlog drain)...\n")
	time.Sleep(60 * time.Second)

	// ---- Verify inclusion ----
	mu.Lock()
	for i := range allResults {
		r := &allResults[i]
		if r.Error != "" {
			r.Included = false
			continue
		}
		acctIdx := uint32(r.Idx / 100000)
		w := derivedAccount(acctIdx)
		_, h := frontierAccount(w.addrStr)
		if h >= r.BlockHeight {
			r.LatencyMs = 0
			r.InclMomentum = 0
		} else {
			r.Included = false
		}
	}
	mu.Unlock()

	fmAfter := frontierMomentum()
	endM := fmAfter.Height
	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart

	// ---- Per-momentum content tracking ----
	fmt.Fprintf(os.Stderr, "saturate: analyzing momentum contents (%d to %d)...\n", startM, endM)
	type momentumDetail struct {
		Height   uint64 `json:"momentum_height"`
		TotalTxs int    `json:"total_txs"`
		Accounts map[int]int `json:"accounts"`
	}
	var momentumDetails []momentumDetail
	for h := startM; h <= endM; h++ {
		var ml struct {
			List []struct {
				Height  uint64 `json:"height"`
				Content []struct {
					Address string `json:"address"`
				} `json:"content"`
			} `json:"list"`
		}
		if err := rpc("ledger.getMomentumsByHeight", []interface{}{h, 1}, &ml); err != nil {
			continue
		}
		if len(ml.List) == 0 {
			continue
		}
		acctCounts := make(map[int]int)
		for _, blk := range ml.List[0].Content {
			for i := 0; i < numAccounts; i++ {
				idx := int(baseIdx) + i
				w := derivedAccount(uint32(idx))
				if blk.Address == w.addrStr {
					acctCounts[idx]++
					break
				}
			}
		}
		if len(acctCounts) > 0 {
			momentumDetails = append(momentumDetails, momentumDetail{
				Height:   ml.List[0].Height,
				TotalTxs: len(ml.List[0].Content),
				Accounts: acctCounts,
			})
		}
	}

	// ---- Per-account breakdown ----
	var breakdowns []saturateBreakdown
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		bd := saturateBreakdown{
			AccountIndex:    int(idx),
			Address:         w.addrStr,
			FusedQSR:        qsrPerAccount,
			PlasmaAvailable: plasmaLevels[idx],
		}
		for _, r := range allResults {
			if uint32(r.Idx/100000) != idx {
				continue
			}
			bd.Attempted++
			if r.Error != "" {
				bd.Rejected++
				continue
			}
			bd.Submitted++
			if r.Included {
				bd.Included++
			} else {
				bd.Unresolved++
			}
		}
		breakdowns = append(breakdowns, bd)
	}

	// ---- Summary ----
	s := summarize("saturate", allResults, startM, endM, duration, cpuSec)

	type saturateOutput struct {
		runSummary
		NumAccounts      int                `json:"num_accounts"`
		TxPerAccount     int                `json:"tx_per_account"`
		QSRPerAccount    int64              `json:"qsr_per_account"`
		TotalTxs         int                `json:"total_txs"`
		MomentumLimit    int                `json:"momentum_limit"`
		MomentaUsed      int                `json:"momenta_used"`
		Breakdowns       []saturateBreakdown `json:"breakdowns"`
		MomentumDetails  []momentumDetail   `json:"momentum_details"`
	}

	out := saturateOutput{
		runSummary:      s,
		NumAccounts:     numAccounts,
		TxPerAccount:    txPerAccount,
		QSRPerAccount:   qsrPerAccount,
		TotalTxs:        totalTxs,
		MomentumLimit:   200,
		MomentaUsed:     int(endM - startM + 1),
		Breakdowns:      breakdowns,
		MomentumDetails: momentumDetails,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// ---- Main ----

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: labctl info|ordinary <n> <amt>|burst <n> <amt>|waitfor <height>|probe <sec>|fuse <qsr>|fund <idx> <znn>|fusefor <idx> <qsr>|matrix <case> <seed>|contend <n> <txPerAcct> <amt>|deplete <idx> <qsr> <maxTx>|powburst <n> <secs> <amt>|powlat <n> <amt>|mixcontend <fused> <unfused> <txPerAcct> <amt>|saturate <n> <txPerAcct> <qsr> <amt>|mixsaturate <fused> <unfused> <fusedTxPer> <unfusedTxPer> <amt>|overpay <groups> <accountsPerGroup> <txPerAcct> <amt>|regen <idx> <qsr>|txtype <idx> <qsr>|poolcap <idx> <qsr> <maxTx>|frontconf <idx> <qsr>")
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "info":
		fm := frontierMomentum()
		a := agentAccount()
		_, h := frontierAccount(a.addrStr)
		fmt.Printf("momentum_height: %d\n", fm.Height)
		fmt.Printf("momentum_hash: %s\n", fm.Hash)
		fmt.Printf("next_fusion_price: %d\n", fm.NextFusionPrice)
		fmt.Printf("next_work_price: %d\n", fm.NextWorkPrice)
		fmt.Printf("agent: %s\n", a.addrStr)
		fmt.Printf("agent_height: %d\n", h)
		fmt.Printf("agent_plasma: %d\n", plasmaLevel(a.addrStr))
		if fm.Height < 30 {
			fmt.Printf("plasma_regime: static (height < 30)\n")
		} else {
			fmt.Printf("plasma_regime: dynamic (height >= 30)\n")
		}

	case "waitfor":
		target, _ := strconv.ParseUint(os.Args[2], 10, 64)
		for {
			fm := frontierMomentum()
			if fm.Height >= target {
				fmt.Printf("reached height %d (target %d)\n", fm.Height, target)
				return
			}
			time.Sleep(5 * time.Second)
		}

	case "ordinary":
		n, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runOrdinary(n, amt)

	case "burst":
		n, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runBurst(n, amt)

	case "probe":
		secs, _ := strconv.Atoi(os.Args[2])
		runProbe(secs)

	case "fuse":
		amt, _ := strconv.ParseInt(os.Args[2], 10, 64)
		runFuse(amt)

	case "fund":
		idx, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runFund(uint32(idx), amt)

	case "fusefor":
		idx, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runFuseFor(uint32(idx), amt)

	case "receive":
		idx, _ := strconv.ParseUint(os.Args[2], 10, 32)
		runReceiveAll(uint32(idx))

	case "multiburst":
		numAccounts, _ := strconv.Atoi(os.Args[2])
		n, _ := strconv.Atoi(os.Args[3])
		amt, _ := strconv.ParseInt(os.Args[4], 10, 64)
		runMultiBurst(numAccounts, n, amt)

	case "matrix":
		caseName := os.Args[2]
		seed, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runMatrix(caseName, seed)

	case "hvburst":
		n, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runHVBurst(n, amt)

	case "sustain":
		secs, _ := strconv.Atoi(os.Args[2])
		numAccounts, _ := strconv.Atoi(os.Args[3])
		runSustain(secs, numAccounts)

	case "contend":
		numAccounts, _ := strconv.Atoi(os.Args[2])
		txPerAccount, _ := strconv.Atoi(os.Args[3])
		amt, _ := strconv.ParseInt(os.Args[4], 10, 64)
		runContend(numAccounts, txPerAccount, amt)

	case "deplete":
		idx, _ := strconv.Atoi(os.Args[2])
		qsrAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		maxTx, _ := strconv.Atoi(os.Args[4])
		runDeplete(uint32(idx), qsrAmt, maxTx)

	case "powburst":
		numAccounts, _ := strconv.Atoi(os.Args[2])
		durationSecs, _ := strconv.Atoi(os.Args[3])
		amt, _ := strconv.ParseInt(os.Args[4], 10, 64)
		runPowBurst(numAccounts, durationSecs, amt)

	case "powlat":
		numAccounts, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runPowLat(numAccounts, amt)

	case "mixcontend":
		numFused, _ := strconv.Atoi(os.Args[2])
		numUnfused, _ := strconv.Atoi(os.Args[3])
		txPerAccount, _ := strconv.Atoi(os.Args[4])
		amt, _ := strconv.ParseInt(os.Args[5], 10, 64)
		runMixContend(numFused, numUnfused, txPerAccount, amt)

	case "saturate":
		numAccounts, _ := strconv.Atoi(os.Args[2])
		txPerAccount, _ := strconv.Atoi(os.Args[3])
		qsrPerAccount, _ := strconv.ParseInt(os.Args[4], 10, 64)
		amt, _ := strconv.ParseInt(os.Args[5], 10, 64)
		runSaturate(numAccounts, txPerAccount, qsrPerAccount, amt)

	case "mixsaturate":
		numFused, _ := strconv.Atoi(os.Args[2])
		numUnfused, _ := strconv.Atoi(os.Args[3])
		fusedTxPer, _ := strconv.Atoi(os.Args[4])
		unfusedTxPer, _ := strconv.Atoi(os.Args[5])
		amt, _ := strconv.ParseInt(os.Args[6], 10, 64)
		runMixSaturate(numFused, numUnfused, fusedTxPer, unfusedTxPer, amt)

	case "overpay":
		groups, _ := strconv.Atoi(os.Args[2])
		accountsPerGroup, _ := strconv.Atoi(os.Args[3])
		txPerAcct, _ := strconv.Atoi(os.Args[4])
		amt, _ := strconv.ParseInt(os.Args[5], 10, 64)
		runOverpay(groups, accountsPerGroup, txPerAcct, amt)

	case "regen":
		idx, _ := strconv.Atoi(os.Args[2])
		qsrAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runRegen(uint32(idx), qsrAmt)

	case "txtype":
		idx, _ := strconv.Atoi(os.Args[2])
		qsrAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runTxType(uint32(idx), qsrAmt)

	case "poolcap":
		idx, _ := strconv.Atoi(os.Args[2])
		qsrAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		maxTx, _ := strconv.Atoi(os.Args[4])
		runPoolCap(uint32(idx), qsrAmt, maxTx)

	case "frontconf":
		idx, _ := strconv.Atoi(os.Args[2])
		qsrAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		runFrontConf(uint32(idx), qsrAmt)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

// runDeplete: plasma depletion experiment (phase 8).
//
// Uses a derived account, funds it, fuses a known QSR amount, then sends
// self-transfers until plasma is exhausted. Records plasma after each tx.
// After exhaustion, waits several momenta and re-checks plasma (regeneration
// test), then tries a PoW-fallback transaction.
//
// The go-zenon code (vm/plasma.go) shows plasma is a monotonically decreasing
// resource: available = fused - cumulative_used - unconfirmed. This experiment
// verifies that model empirically.
func runDeplete(idx uint32, qsrAmt int64, maxTx int) {
	agent := agentAccount()
	target := derivedAccount(idx)

	// Step 0: Record initial state
	initialPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "deplete: account[%d]=%s initial_plasma=%d\n", idx, target.addrStr, initialPlasma)

	// Step 1: Fund the target account with ZNN for self-transfers
	_, targetHeight := frontierAccount(target.addrStr)
	if targetHeight == 0 {
		fmt.Fprintf(os.Stderr, "deplete: funding account[%d] with 100 ZNN...\n", idx)
		b, _, err := buildSend(agent, target.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	// Step 2: Fuse QSR for plasma on the target account
	fmt.Fprintf(os.Stderr, "deplete: fusing %d QSR on account[%d]...\n", qsrAmt, idx)
	data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, target.addr)
	b, _, err := buildSend(agent, types.PlasmaContract, qsr, dec(qsrAmt), data, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL fuse: %v\n", err)
		os.Exit(1)
	}
	if err := publish(b); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish fuse: %v\n", err)
		os.Exit(1)
	}
	waitIncluded(agent.addrStr, b.Height, 120*time.Second)

	// Step 2b: Wait for funding to be visible as unreceived, then receive all pending blocks
	fmt.Fprintf(os.Stderr, "deplete: waiting for funding to arrive at account[%d]...\n", idx)
	for i := 0; i < 30; i++ {
		var ul unreceivedList
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{target.addrStr, 0, 50}, &ul)
		if len(ul.List) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	// Also wait for fuse to be included
	time.Sleep(5 * time.Second)
	fmt.Fprintf(os.Stderr, "deplete: receiving pending blocks on account[%d]...\n", idx)
	received := receiveAll(target)
	fmt.Fprintf(os.Stderr, "deplete: received %d blocks on account[%d]\n", received, idx)

	// Step 2c: Verify the account has ZNN balance
	var acctInfo struct {
		BalanceList []struct {
			Token   string `json:"token"`
			Balance string `json:"balance"`
		} `json:"balanceInfoList"`
	}
	mustRPC("ledger.getAccountInfoByAddress", []interface{}{target.addrStr}, &acctInfo)
	for _, bi := range acctInfo.BalanceList {
		fmt.Fprintf(os.Stderr, "deplete: account[%d] balance: %s = %s\n", idx, bi.Token, bi.Balance)
	}

	postFusionPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "deplete: post-fusion plasma=%d (fused %d QSR)\n", postFusionPlasma, qsrAmt)

	// Step 3: Send self-transfers until plasma exhausted
	type depleteResult struct {
		runSummary
		AccountIndex     uint64 `json:"account_index"`
		AccountAddress   string `json:"account_address"`
		QSRFused         int64  `json:"qsr_fused"`
		InitialPlasma    uint64 `json:"initial_plasma"`
		PostFusionPlasma uint64 `json:"post_fusion_plasma"`
		PlasmaPerTx      []uint64 `json:"plasma_per_tx"`
		ExpectedPlasmaPerTx uint64 `json:"expected_plasma_per_tx"`
		RegenTestPlasma  uint64 `json:"regen_test_plasma"`
		RegenWaitMomenta int    `json:"regen_wait_momenta"`
		PoWFallbackUsed  bool   `json:"pow_fallback_used"`
		PoWFallbackMs    int64  `json:"pow_fallback_ms"`
		PoWFallbackOK    bool   `json:"pow_fallback_ok"`
		DepletedAtTx     int    `json:"depleted_at_tx"`
	}

	startM := frontierMomentum().Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var results []txResult
	var plasmaLevels []uint64

	for i := 0; i < maxTx; i++ {
		// Check plasma before send
		plasmaBefore := plasmaLevel(target.addrStr)
		plasmaLevels = append(plasmaLevels, plasmaBefore)

		b, powMs, err := buildSend(target, target.addr, znn, dec(1), nil, true)
		if err != nil {
			results = append(results, txResult{
				Idx: i, Error: fmt.Sprintf("build: %v", err),
				PlasmaAfter: plasmaBefore,
			})
			fmt.Fprintf(os.Stderr, "deplete: tx %d BUILD FAILED: %v (plasma=%d)\n", i, err, plasmaBefore)
			break
		}

		subM := frontierMomentum().Height
		if err := publish(b); err != nil {
			results = append(results, txResult{
				Idx: i, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
				Error: fmt.Sprintf("publish: %v", err), PlasmaAfter: plasmaBefore,
			})
			fmt.Fprintf(os.Stderr, "deplete: tx %d PUBLISH FAILED: %v (plasma=%d)\n", i, err, plasmaBefore)
			break
		}

		lat, mh, ok := waitIncluded(target.addrStr, b.Height, 60*time.Second)
		plasmaAfter := plasmaLevel(target.addrStr)
		results = append(results, txResult{
			Idx: i, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
			Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
			PlasmaAfter: plasmaAfter,
		})
		fmt.Fprintf(os.Stderr, "deplete: tx %d included=%v plasma_before=%d plasma_after=%d pow_ms=%d\n",
			i, ok, plasmaBefore, plasmaAfter, powMs)

		if !ok {
			fmt.Fprintf(os.Stderr, "deplete: tx %d NOT INCLUDED, stopping\n", i)
			break
		}

		// If plasma dropped to 0 or near 0, try one more then stop
		if plasmaAfter == 0 && i > 0 {
			fmt.Fprintf(os.Stderr, "deplete: plasma exhausted at tx %d\n", i)
			break
		}
	}

	depletedAtTx := len(results)

	// Step 4: Regeneration test — wait 5 momenta, check plasma
	fmt.Fprintf(os.Stderr, "deplete: waiting 5 momenta for regeneration test...\n")
	fmNow := frontierMomentum()
	targetHeight2 := fmNow.Height + 5
	for {
		fmCur := frontierMomentum()
		if fmCur.Height >= targetHeight2 {
			break
		}
		time.Sleep(3 * time.Second)
	}
	regenPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "deplete: after 5 momenta, plasma=%d (was %d at last tx)\n",
		regenPlasma, plasmaLevel(target.addrStr))

	// Step 5: PoW fallback test — try to send with PoW when plasma is 0
	fmt.Fprintf(os.Stderr, "deplete: testing PoW fallback...\n")
	powFallbackUsed := false
	powFallbackMs := int64(0)
	powFallbackOK := false
	if regenPlasma < 21000 {
		powFallbackUsed = true
		b, powMs, err := buildSend(target, target.addr, znn, dec(1), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "deplete: PoW fallback build failed: %v\n", err)
		} else {
			powFallbackMs = powMs
			if err := publish(b); err != nil {
				fmt.Fprintf(os.Stderr, "deplete: PoW fallback publish failed: %v\n", err)
			} else {
				_, _, ok := waitIncluded(target.addrStr, b.Height, 60*time.Second)
				powFallbackOK = ok
				fmt.Fprintf(os.Stderr, "deplete: PoW fallback included=%v pow_ms=%d\n", ok, powMs)
			}
		}
	}

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := frontierMomentum().Height
	s := summarize("deplete", results, startM, endM, duration, cpuSec)

	out := depleteResult{
		runSummary:          s,
		AccountIndex:        uint64(idx),
		AccountAddress:      target.addrStr,
		QSRFused:            qsrAmt,
		InitialPlasma:       initialPlasma,
		PostFusionPlasma:    postFusionPlasma,
		PlasmaPerTx:         plasmaLevels,
		ExpectedPlasmaPerTx: 21000,
		RegenTestPlasma:     regenPlasma,
		RegenWaitMomenta:    5,
		PoWFallbackUsed:     powFallbackUsed,
		PoWFallbackMs:       powFallbackMs,
		PoWFallbackOK:       powFallbackOK,
		DepletedAtTx:        depletedAtTx,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// runRegen: plasma regeneration rate measurement (phase 15).
//
// Phase 8 found that plasma regenerates after depletion but the rate was TBD.
// This experiment depletes plasma completely, then samples plasma levels at
// increasing momentum intervals (1, 3, 6, 10 momenta) to map the regeneration
// curve. Repeats with 3 seeds for reproducibility.
//
// The go-zenon code (vm/plasma.go) suggests regeneration happens on momentum
// confirmation via the two-counter system (committed/unconfirmed). This
// experiment measures the empirical rate.
func runRegen(idx uint32, qsrAmt int64) {
	agent := agentAccount()
	target := derivedAccount(idx)

	// Step 0: Record initial state
	initialPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "regen: account[%d]=%s initial_plasma=%d\n", idx, target.addrStr, initialPlasma)

	// Step 1: Fund the target account with ZNN for self-transfers
	_, targetHeight := frontierAccount(target.addrStr)
	if targetHeight == 0 {
		fmt.Fprintf(os.Stderr, "regen: funding account[%d] with 100 ZNN...\n", idx)
		b, _, err := buildSend(agent, target.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	// Step 2: Fuse QSR for plasma on the target account
	fmt.Fprintf(os.Stderr, "regen: fusing %d QSR on account[%d]...\n", qsrAmt, idx)
	data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, target.addr)
	b, _, err := buildSend(agent, types.PlasmaContract, qsr, dec(qsrAmt), data, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL fuse: %v\n", err)
		os.Exit(1)
	}
	if err := publish(b); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish fuse: %v\n", err)
		os.Exit(1)
	}
	waitIncluded(agent.addrStr, b.Height, 120*time.Second)

	// Step 2b: Wait for funding to arrive, then receive all pending blocks
	fmt.Fprintf(os.Stderr, "regen: waiting for funding to arrive at account[%d]...\n", idx)
	for i := 0; i < 30; i++ {
		var ul unreceivedList
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{target.addrStr, 0, 50}, &ul)
		if len(ul.List) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	time.Sleep(5 * time.Second)
	fmt.Fprintf(os.Stderr, "regen: receiving pending blocks on account[%d]...\n", idx)
	received := receiveAll(target)
	fmt.Fprintf(os.Stderr, "regen: received %d blocks on account[%d]\n", received, idx)

	// Step 2c: Wait for fusion to be confirmed and plasma available
	fmt.Fprintf(os.Stderr, "regen: waiting for fusion to confirm and plasma to become available...\n")
	for i := 0; i < 60; i++ {
		p := plasmaLevel(target.addrStr)
		if p >= uint64(qsrAmt*2100) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	postFusionPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "regen: post-fusion plasma=%d (fused %d QSR, expected %d)\n", postFusionPlasma, qsrAmt, qsrAmt*2100)

	if postFusionPlasma < 21000 {
		fmt.Fprintf(os.Stderr, "FATAL: plasma not available after fusion (plasma=%d)\n", postFusionPlasma)
		os.Exit(1)
	}

	// Step 3: Deplete plasma by sending self-transfers
	fmt.Fprintf(os.Stderr, "regen: depleting plasma...\n")
	var depleteResults []txResult
	maxTx := int(qsrAmt * 10) // 10 txs per QSR at 21000 plasma each
	for i := 0; i < maxTx; i++ {
		plasmaBefore := plasmaLevel(target.addrStr)
		if plasmaBefore < 21000 {
			fmt.Fprintf(os.Stderr, "regen: plasma depleted at tx %d (plasma=%d)\n", i, plasmaBefore)
			break
		}

		b, powMs, err := buildSend(target, target.addr, znn, dec(1), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "regen: tx %d BUILD FAILED: %v\n", i, err)
			break
		}
		subM := frontierMomentum().Height
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "regen: tx %d PUBLISH FAILED: %v\n", i, err)
			break
		}
		lat, mh, ok := waitIncluded(target.addrStr, b.Height, 60*time.Second)
		plasmaAfter := plasmaLevel(target.addrStr)
		depleteResults = append(depleteResults, txResult{
			Idx: i, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
			Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
			PlasmaAfter: plasmaAfter,
		})
		fmt.Fprintf(os.Stderr, "regen: deplete tx %d included=%v plasma=%d\n", i, ok, plasmaAfter)
		if !ok {
			break
		}
	}

	depletedPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "regen: depleted plasma=%d\n", depletedPlasma)

	// Step 4: Regeneration curve — sample plasma at increasing momentum intervals
	type regenSample struct {
		MomentaWaited int    `json:"momenta_waited"`
		Plasma        uint64 `json:"plasma"`
		MomentumHeight uint64 `json:"momentum_height"`
	}
	var regenCurve []regenSample

	// Wait intervals: 1, 2, 3, 4 more momenta (cumulative: 1, 3, 6, 10)
	intervals := []int{1, 2, 3, 4}
	cumulative := 0
	for _, interval := range intervals {
		cumulative += interval
		targetHeight := frontierMomentum().Height + uint64(interval)
		for {
			fmCur := frontierMomentum()
			if fmCur.Height >= targetHeight {
				break
			}
			time.Sleep(2 * time.Second)
		}
		p := plasmaLevel(target.addrStr)
		regenCurve = append(regenCurve, regenSample{
			MomentaWaited:  cumulative,
			Plasma:         p,
			MomentumHeight: frontierMomentum().Height,
		})
		fmt.Fprintf(os.Stderr, "regen: after %d momenta, plasma=%d\n", cumulative, p)
	}

	// Step 5: Also measure time-based regeneration (wait 20 seconds)
	fmt.Fprintf(os.Stderr, "regen: waiting 20 seconds for time-based sample...\n")
	time.Sleep(20 * time.Second)
	timeSample := plasmaLevel(target.addrStr)
	timeSampleHeight := frontierMomentum().Height
	fmt.Fprintf(os.Stderr, "regen: after 20s, plasma=%d (momentum=%d)\n", timeSample, timeSampleHeight)

	// Compute regeneration rate
	fusedPlasma := uint64(qsrAmt * 2100) // 2100 plasma per QSR
	type regenResult struct {
		runSummary
		AccountIndex       uint64        `json:"account_index"`
		AccountAddress     string        `json:"account_address"`
		QSRFused           int64         `json:"qsr_fused"`
		FusedPlasma        uint64        `json:"fused_plasma"`
		InitialPlasma      uint64        `json:"initial_plasma"`
		PostFusionPlasma   uint64        `json:"post_fusion_plasma"`
		DepletedPlasma     uint64        `json:"depleted_plasma"`
		DepletedAtTx       int           `json:"depleted_at_tx"`
		RegenCurve         []regenSample `json:"regen_curve"`
		TimeSamplePlasma   uint64        `json:"time_sample_plasma"`
		TimeSampleHeight   uint64        `json:"time_sample_height"`
		RegenPerMomentum   float64       `json:"regen_per_momentum"`
		RegenPerSecond     float64       `json:"regen_per_second"`
		FullRegenMomenta   int           `json:"full_regen_momenta"`
	}

	// Calculate per-momentum regeneration rate from curve
	var regenPerMomentum float64
	if len(regenCurve) >= 2 {
		last := regenCurve[len(regenCurve)-1]
		first := regenCurve[0]
		momentumDelta := last.MomentaWaited - first.MomentaWaited
		plasmaDelta := last.Plasma - first.Plasma
		if momentumDelta > 0 {
			regenPerMomentum = float64(plasmaDelta) / float64(momentumDelta)
		}
	}

	// Estimate full regeneration momenta
	fullRegenMomenta := 0
	if regenPerMomentum > 0 {
		fullRegenMomenta = int(float64(fusedPlasma) / regenPerMomentum)
	}

	out := regenResult{
		runSummary:        summarize("regen", depleteResults, 0, 0, 0, 0),
		AccountIndex:      uint64(idx),
		AccountAddress:    target.addrStr,
		QSRFused:          qsrAmt,
		FusedPlasma:       fusedPlasma,
		InitialPlasma:     initialPlasma,
		PostFusionPlasma:  postFusionPlasma,
		DepletedPlasma:    depletedPlasma,
		DepletedAtTx:      len(depleteResults),
		RegenCurve:        regenCurve,
		TimeSamplePlasma:  timeSample,
		TimeSampleHeight:  timeSampleHeight,
		RegenPerMomentum:  regenPerMomentum,
		RegenPerSecond:    float64(timeSample) / 20.0,
		FullRegenMomenta:  fullRegenMomenta,
	}
	// Fix summary fields
	out.runSummary.StartMomentum = depleteResults[0].SubmitMomentum
	out.runSummary.EndMomentum = frontierMomentum().Height
	out.runSummary.DurationSec = 0 // not meaningful for this experiment

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// runHVBurst: high-volume burst designed to trigger Dynamic Plasma repricing.
// Sends n transactions as fast as possible from the agent account, records
// nextFusionPrice/nextWorkPrice before and after each momentum to detect repricing.
func runHVBurst(n int, amt int64) {
	a := agentAccount()

	// Record prices before the burst
	fmBefore := frontierMomentum()
	fmt.Fprintf(os.Stderr, "pre-burst: momentum=%d nextFusionPrice=%d nextWorkPrice=%d plasma=%d\\n",
		fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice, plasmaLevel(a.addrStr))

	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	// Track prices per momentum during the burst
	type momentumPrice struct {
		Height          uint64 `json:"height"`
		NextFusionPrice uint64 `json:"nextFusionPrice"`
		NextWorkPrice   uint64 `json:"nextWorkPrice"`
	}
	var prices []momentumPrice
	var pricesMu sync.Mutex
	prices = append(prices, momentumPrice{fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice})

	// Momentum watcher: record price changes
	stopWatcher := make(chan struct{})
	var watcherWG sync.WaitGroup
	watcherWG.Add(1)
	go func() {
		defer watcherWG.Done()
		lastHeight := fmBefore.Height
		for {
			select {
			case <-stopWatcher:
				return
			default:
				fm := frontierMomentum()
				if fm.Height > lastHeight {
					pricesMu.Lock()
					prices = append(prices, momentumPrice{fm.Height, fm.NextFusionPrice, fm.NextWorkPrice})
					pricesMu.Unlock()
					lastHeight = fm.Height
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	// Send n transactions as fast as possible
	var results []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b, powMs, err := buildSend(a, a.addr, znn, dec(amt), nil, true)
			if err != nil {
				mu.Lock()
				results = append(results, txResult{Idx: idx, Error: err.Error()})
				mu.Unlock()
				return
			}
			subM := frontierMomentum().Height
			if err := publish(b); err != nil {
				mu.Lock()
				results = append(results, txResult{Idx: idx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: err.Error()})
				mu.Unlock()
				return
			}
			lat, mh, ok := waitIncluded(a.addrStr, b.Height, 120*time.Second)
			mu.Lock()
			results = append(results, txResult{
				Idx: idx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
				Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
				PlasmaAfter: plasmaLevel(a.addrStr),
			})
			mu.Unlock()
		}(i)
		// Minimal stagger — just enough to avoid RPC overload
		time.Sleep(2 * time.Millisecond)
	}
	wg.Wait()

	// Stop the watcher
	close(stopWatcher)
	watcherWG.Wait()

	// Record final prices
	fmAfter := frontierMomentum()
	pricesMu.Lock()
	// Deduplicate: only keep the last entry per height
	seen := make(map[uint64]int)
	var deduped []momentumPrice
	for _, p := range prices {
		if _, ok := seen[p.Height]; !ok {
			deduped = append(deduped, p)
			seen[p.Height] = len(deduped) - 1
		} else {
			deduped[seen[p.Height]] = p
		}
	}
	pricesMu.Unlock()

	fmt.Fprintf(os.Stderr, "post-burst: momentum=%d nextFusionPrice=%d nextWorkPrice=%d plasma=%d\\n",
		fmAfter.Height, fmAfter.NextFusionPrice, fmAfter.NextWorkPrice, plasmaLevel(a.addrStr))

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := fmAfter.Height
	s := summarize("hvburst", results, startM, endM, duration, cpuSec)

	// Add price tracking to output
	type hvOutput struct {
		runSummary
		MomentumPrices []momentumPrice `json:"momentum_prices"`
		PreFusionPrice  uint64          `json:"pre_fusion_price"`
		PreWorkPrice    uint64          `json:"pre_work_price"`
		PostFusionPrice uint64          `json:"post_fusion_price"`
		PostWorkPrice   uint64          `json:"post_work_price"`
	}
	out := hvOutput{
		runSummary:      s,
		MomentumPrices:  deduped,
		PreFusionPrice:  fmBefore.NextFusionPrice,
		PreWorkPrice:    fmBefore.NextWorkPrice,
		PostFusionPrice: fmAfter.NextFusionPrice,
		PostWorkPrice:   fmAfter.NextWorkPrice,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// ---- Phase 7: Contention ----

// momentumContent tracks how many transactions from each account were included in a momentum.
type momentumContent struct {
	Height   uint64 `json:"momentum_height"`
	Accounts map[int]int `json:"accounts"`
}

type accountBreakdown struct {
	AccountIndex    int    `json:"account_index"`
	Address         string `json:"address"`
	FusedQSR        int64  `json:"fused_qsr"`
	PlasmaAvailable uint64 `json:"plasma_available"`
	Attempted       int    `json:"attempted"`
	Submitted       int    `json:"submitted"`
	Included        int    `json:"included"`
	Rejected        int    `json:"rejected"`
	Unresolved      int    `json:"unresolved"`
	P50Ms           int64  `json:"p50_ms"`
	P90Ms           int64  `json:"p90_ms"`
}

// runContend: multi-sender contention experiment.
// Funds N derived accounts, fuses varying QSR amounts to create a plasma gradient,
// then has all accounts simultaneously send transactions to contend for inclusion.
//
// Setup: account i gets fused (i * 10) QSR, creating a plasma gradient.
// Contention: all N accounts fire txPerAccount self-transfers as fast as possible.
// Measurement: per-account stats and per-momentum content tracking.
func runContend(numAccounts int, txPerAccount int, amt int64) {
	// ---- Setup phase: fund + fuse (before timing) ----
	fmt.Fprintf(os.Stderr, "contend setup: funding %d accounts...\n", numAccounts)
	for i := 1; i <= numAccounts; i++ {
		w := derivedAccount(uint32(i))
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "  funding account %d (%s)...\n", i, w.addrStr)
			runFund(uint32(i), 100)
		}
	}

	// Wait for funding transactions to be included, then receive them
	targetHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "contend setup: waiting for funding inclusion (height %d)...\n", targetHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= targetHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "contend setup: receiving funded blocks...\n")
	for i := 1; i <= numAccounts; i++ {
		w := derivedAccount(uint32(i))
		count := receiveAll(w)
		if count > 0 {
			fmt.Fprintf(os.Stderr, "  account %d: received %d blocks\n", i, count)
		}
	}

	fmt.Fprintf(os.Stderr, "contend setup: fusing QSR for plasma gradient...\n")
	for i := 1; i <= numAccounts; i++ {
		fuseAmt := int64(i * 10)
		fmt.Fprintf(os.Stderr, "  fusing %d QSR for account %d...\n", fuseAmt, i)
		runFuseFor(uint32(i), fuseAmt)
	}

	// Wait for fusion transactions to be included
	fuseHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "contend setup: waiting for fusion inclusion (height %d)...\n", fuseHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= fuseHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Record plasma levels after setup
	plasmaLevels := make(map[int]uint64)
	for i := 1; i <= numAccounts; i++ {
		w := derivedAccount(uint32(i))
		plasmaLevels[i] = plasmaLevel(w.addrStr)
	}
	for i := 1; i <= numAccounts; i++ {
		fmt.Fprintf(os.Stderr, "  account %d: plasma=%d\n", i, plasmaLevels[i])
	}

	fmt.Fprintf(os.Stderr, "contend: starting contention phase (%d accounts x %d txs each)...\n", numAccounts, txPerAccount)

	// ---- Contention phase ----
	fmBefore := frontierMomentum()
	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	// Each account sends its txs sequentially, but accounts run in parallel.
	// Fire-and-forget for maximum contention, verify at the end.
	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 1; i <= numAccounts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := derivedAccount(uint32(idx))
			for j := 0; j < txPerAccount; j++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + j, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				// Fire-and-forget: record optimistically, verify later
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: idx*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true, // optimistic; verified below
				})
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// Wait for last transactions to be included
	fmt.Fprintf(os.Stderr, "contend: waiting 15s for final inclusions...\n")
	time.Sleep(15 * time.Second)

	// ---- Verify inclusion ----
	mu.Lock()
	for i := range allResults {
		r := &allResults[i]
		if r.Error != "" {
			r.Included = false
			continue
		}
		acctIdx := r.Idx / 100000
		w := derivedAccount(uint32(acctIdx))
		_, h := frontierAccount(w.addrStr)
		if h >= r.BlockHeight {
			r.LatencyMs = 0 // can't measure individual latency in fire-and-forget
			r.InclMomentum = 0
		} else {
			r.Included = false
		}
	}
	mu.Unlock()

	fmAfter := frontierMomentum()
	endM := fmAfter.Height
	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart

	// ---- Per-momentum content tracking ----
	// Query momentum details for each momentum in range and count txs per account.
	fmt.Fprintf(os.Stderr, "contend: analyzing momentum contents (%d to %d)...\n", startM, endM)
	momentumContents := make([]momentumContent, 0)
	for h := startM; h <= endM; h++ {
		var ml struct {
			List []struct {
				Height  uint64 `json:"height"`
				Content []struct {
					Address string `json:"address"`
				} `json:"content"`
			} `json:"list"`
		}
		if err := rpc("ledger.getMomentumsByHeight", []interface{}{h, 1}, &ml); err != nil {
			continue
		}
		if len(ml.List) == 0 {
			continue
		}
		acctCounts := make(map[int]int)
		for _, blk := range ml.List[0].Content {
			for i := 1; i <= numAccounts; i++ {
				w := derivedAccount(uint32(i))
				if blk.Address == w.addrStr {
					acctCounts[i]++
					break
				}
			}
		}
		if len(acctCounts) > 0 {
			momentumContents = append(momentumContents, momentumContent{
				Height:   ml.List[0].Height,
				Accounts: acctCounts,
			})
		}
	}

	// ---- Per-account breakdown ----
	breakdowns := make([]accountBreakdown, 0, numAccounts)
	for i := 1; i <= numAccounts; i++ {
		w := derivedAccount(uint32(i))
		bd := accountBreakdown{
			AccountIndex:    i,
			Address:         w.addrStr,
			FusedQSR:        int64(i * 10),
			PlasmaAvailable: plasmaLevels[i],
		}
		var lats []int64
		for _, r := range allResults {
			if r.Idx/100000 != i {
				continue
			}
			bd.Attempted++
			if r.Error != "" {
				bd.Rejected++
				continue
			}
			bd.Submitted++
			if r.Included {
				bd.Included++
				lats = append(lats, r.LatencyMs)
			} else {
				bd.Unresolved++
			}
		}
		sort.Slice(lats, func(a, b int) bool { return lats[a] < lats[b] })
		bd.P50Ms = percentile(lats, 0.50)
		bd.P90Ms = percentile(lats, 0.90)
		breakdowns = append(breakdowns, bd)
	}

	// ---- Build summary ----
	s := summarize("contend", allResults, startM, endM, duration, cpuSec)

	type contendOutput struct {
		runSummary
		AccountBreakdowns  []accountBreakdown  `json:"account_breakdowns"`
		MomentumContents   []momentumContent   `json:"momentum_contents"`
		TotalMomentumTxs   int                 `json:"total_momentum_txs"`
	}

	// Count total txs across all momenta
	totalMomentumTxs := 0
	for _, mc := range momentumContents {
		for _, c := range mc.Accounts {
			totalMomentumTxs += c
		}
	}

	out := contendOutput{
		runSummary:        s,
		AccountBreakdowns: breakdowns,
		MomentumContents:  momentumContents,
		TotalMomentumTxs:  totalMomentumTxs,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// runOrdinary: sequential self-transfers, waiting for each to be included
func runOrdinary(n int, amt int64) {
	a := agentAccount()
	startM := frontierMomentum().Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var results []txResult
	for i := 0; i < n; i++ {
		b, powMs, err := buildSend(a, a.addr, znn, dec(amt), nil, true)
		if err != nil {
			results = append(results, txResult{Idx: i, Error: err.Error()})
			continue
		}
		subM := frontierMomentum().Height
		if err := publish(b); err != nil {
			results = append(results, txResult{Idx: i, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: err.Error()})
			continue
		}
		lat, mh, ok := waitIncluded(a.addrStr, b.Height, 120*time.Second)
		results = append(results, txResult{
			Idx: i, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
			Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
			PlasmaAfter: plasmaLevel(a.addrStr),
		})
	}

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := frontierMomentum().Height
	s := summarize("ordinary", results, startM, endM, duration, cpuSec)
	printSummaryJSON(s)
}

// runBurst: rapid-fire self-transfers without waiting for inclusion
func runBurst(n int, amt int64) {
	a := agentAccount()
	startM := frontierMomentum().Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var results []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Submit all n transactions as fast as possible
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b, powMs, err := buildSend(a, a.addr, znn, dec(amt), nil, true)
			if err != nil {
				mu.Lock()
				results = append(results, txResult{Idx: idx, Error: err.Error()})
				mu.Unlock()
				return
			}
			subM := frontierMomentum().Height
			if err := publish(b); err != nil {
				mu.Lock()
				results = append(results, txResult{Idx: idx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: err.Error()})
				mu.Unlock()
				return
			}
			lat, mh, ok := waitIncluded(a.addrStr, b.Height, 120*time.Second)
			mu.Lock()
			results = append(results, txResult{
				Idx: idx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
				Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
				PlasmaAfter: plasmaLevel(a.addrStr),
			})
			mu.Unlock()
		}(i)
		// Small stagger to avoid RPC overload
		time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := frontierMomentum().Height
	s := summarize("burst", results, startM, endM, duration, cpuSec)
	printSummaryJSON(s)
}

// runProbe: lightly-provisioned account sends small transactions periodically
func runProbe(secs int) {
	// Use a derived account with no fused plasma as the probe
	probe := derivedAccount(3)
	agent := agentAccount()

	// Fund the probe if needed
	_, h := frontierAccount(probe.addrStr)
	if h == 0 {
		fmt.Fprintf(os.Stderr, "funding probe account %s...\n", probe.addrStr)
		b, _, err := buildSend(agent, probe.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund probe: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	deadline := time.Now().Add(time.Duration(secs) * time.Second)
	startM := frontierMomentum().Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var results []txResult
	idx := 0
	for time.Now().Before(deadline) {
		b, powMs, err := buildSend(probe, probe.addr, znn, dec(1), nil, true)
		if err != nil {
			results = append(results, txResult{Idx: idx, Error: err.Error()})
			idx++
			time.Sleep(5 * time.Second)
			continue
		}
		subM := frontierMomentum().Height
		if err := publish(b); err != nil {
			results = append(results, txResult{Idx: idx, Error: err.Error()})
			idx++
			time.Sleep(5 * time.Second)
			continue
		}
		lat, mh, ok := waitIncluded(probe.addrStr, b.Height, 60*time.Second)
		results = append(results, txResult{
			Idx: idx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
			Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
			PlasmaAfter: plasmaLevel(probe.addrStr),
		})
		idx++
		time.Sleep(10 * time.Second)
	}

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := frontierMomentum().Height
	s := summarize("probe", results, startM, endM, duration, cpuSec)
	printSummaryJSON(s)
}

// runFuse: fuse QSR for plasma on the agent account
func runFuse(amt int64) {
	a := agentAccount()
	data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, a.addr)
	b, powMs, err := buildSend(a, types.PlasmaContract, qsr, dec(amt), data, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	if err := publish(b); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish: %v\n", err)
		os.Exit(1)
	}
	lat, mh, ok := waitIncluded(a.addrStr, b.Height, 120*time.Second)
	fmt.Printf("fuse %d QSR: height=%d pow_ms=%d included=%v latency=%v momentum=%d plasma=%d\n",
		amt, b.Height, powMs, ok, lat, mh, plasmaLevel(a.addrStr))
}

// runFund: send ZNN from agent to a derived account
func runFund(idx uint32, amt int64) {
	agent := agentAccount()
	target := derivedAccount(idx)
	b, _, err := buildSend(agent, target.addr, znn, dec(amt), nil, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	if err := publish(b); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish: %v\n", err)
		os.Exit(1)
	}
	lat, mh, ok := waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	fmt.Printf("fund [%d] %s: %d ZNN height=%d included=%v latency=%v momentum=%d\n",
		idx, target.addrStr, amt, b.Height, ok, lat, mh)
}

// runFuseFor: fuse QSR with beneficiary = derived account
func runFuseFor(idx uint32, amt int64) {
	agent := agentAccount()
	target := derivedAccount(idx)
	data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, target.addr)
	b, _, err := buildSend(agent, types.PlasmaContract, qsr, dec(amt), data, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	if err := publish(b); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish: %v\n", err)
		os.Exit(1)
	}
	lat, mh, ok := waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	fmt.Printf("fusefor [%d] %s: %d QSR height=%d included=%v latency=%v momentum=%d target_plasma=%d\n",
		idx, target.addrStr, amt, b.Height, ok, lat, mh, plasmaLevel(target.addrStr))
}

// runMatrix: phase-3 matrix case runner
func runMatrix(caseName string, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	_ = rng // used for jitter in future variants

	switch caseName {
	case "low-steady-1":
		// Low fused plasma, steady (ordinary), 1 account
		runOrdinary(5, 1)
	case "low-burst-1":
		// Low fused plasma, burst, 1 account
		runBurst(10, 1)
	case "high-steady-1":
		// Higher fused plasma, steady, 1 account
		runFuse(100)
		runOrdinary(5, 1)
	case "high-burst-1":
		// Higher fused plasma, burst, 1 account
		runFuse(100)
		runBurst(10, 1)
	case "low-steady-multi":
		// Low fused, steady, several accounts
		runFund(1, 100)
		runFund(2, 100)
		runOrdinary(3, 1)
	case "probe-only":
		// Just the probe for a short time
		runProbe(60)
	default:
		fmt.Fprintf(os.Stderr, "unknown matrix case: %s\n", caseName)
		os.Exit(1)
	}
}

// runMultiBurst: burst from multiple accounts simultaneously
// Each account sends n txs in parallel; accounts don't conflict with each other.
func runMultiBurst(numAccounts int, n int, amt int64) {
	agent := agentAccount()
	
	// Fund accounts if needed
	for i := 1; i <= numAccounts; i++ {
		w := derivedAccount(uint32(i))
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "funding account %d...\n", i)
			b, _, err := buildSend(agent, w.addr, znn, dec(100), nil, true)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARN fund [%d]: %v\n", i, err)
				continue
			}
			if err := publish(b); err != nil {
				fmt.Fprintf(os.Stderr, "WARN publish fund [%d]: %v\n", i, err)
				continue
			}
			waitIncluded(agent.addrStr, b.Height, 120*time.Second)
		}
	}

	startM := frontierMomentum().Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 1; i <= numAccounts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := derivedAccount(uint32(idx))
			for j := 0; j < n; j++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*1000 + j, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*1000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				lat, mh, ok := waitIncluded(w.addrStr, b.Height, 120*time.Second)
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: idx*1000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: ok, LatencyMs: lat.Milliseconds(), InclMomentum: mh,
					PlasmaAfter: plasmaLevel(w.addrStr),
				})
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := frontierMomentum().Height
	s := summarize("multi-burst", allResults, startM, endM, duration, cpuSec)
	printSummaryJSON(s)
}

// runReceiveAll: receive all pending unreceived blocks for an account
func runReceiveAll(idx uint32) {
	a := derivedAccount(idx)
	count := receiveAll(a)
	fmt.Printf("account [%d] %s: received %d blocks\n", idx, a.addrStr, count)
}

type unreceivedList struct {
	List []*struct {
		Hash types.Hash `json:"hash"`
	} `json:"list"`
}

func receiveAll(a account) int {
	var ul unreceivedList
	mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{a.addrStr, 0, 50}, &ul)
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
		req := requiredPoW(a.addrStr, b.BlockType, nil, nil)
		if req.RequiredDifficulty > 0 {
			b.Difficulty = req.RequiredDifficulty
			dataHash := pow.GetAccountBlockHash(b)
			nonce := pow.GetPoWNonce(big.NewInt(int64(req.RequiredDifficulty)), dataHash)
			copy(b.Nonce.Data[:], nonce)
		} else {
			b.FusedPlasma = req.BasePlasma
		}
		b.Hash = b.ComputeHash()
		b.PublicKey = a.kp.Public
		b.Signature = a.kp.Sign(b.Hash.Bytes())
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "receive publish err %s: %v\n", a.addrStr[:14], err)
			continue
		}
		_, _, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
		if ok {
			count++
		}
	}
	return count
}

// runSustain: continuously send transactions from multiple accounts for N seconds.
// Designed to trigger sustained Dynamic Plasma repricing across multiple momenta.
func runSustain(secs int, numAccounts int) {
	fmBefore := frontierMomentum()
	fmt.Fprintf(os.Stderr, "pre-sustain: momentum=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice)

	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()
	deadline := tStart.Add(time.Duration(secs) * time.Second)

	// Momentum watcher: record price changes
	type momentumPrice struct {
		Height          uint64 `json:"height"`
		NextFusionPrice uint64 `json:"nextFusionPrice"`
		NextWorkPrice   uint64 `json:"nextWorkPrice"`
	}
	var prices []momentumPrice
	var pricesMu sync.Mutex
	prices = append(prices, momentumPrice{fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice})

	stopWatcher := make(chan struct{})
	var watcherWG sync.WaitGroup
	watcherWG.Add(1)
	go func() {
		defer watcherWG.Done()
		lastHeight := fmBefore.Height
		for {
			select {
			case <-stopWatcher:
				return
			default:
				fm := frontierMomentum()
				if fm.Height > lastHeight {
					pricesMu.Lock()
					prices = append(prices, momentumPrice{fm.Height, fm.NextFusionPrice, fm.NextWorkPrice})
					pricesMu.Unlock()
					lastHeight = fm.Height
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	// Send transactions from all accounts in parallel until deadline
	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 1; i <= numAccounts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := derivedAccount(uint32(idx))
			txIdx := 0
			for time.Now().Before(deadline) {
				b, powMs, err := buildSend(w, w.addr, znn, dec(1), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + txIdx, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					time.Sleep(100 * time.Millisecond)
					txIdx++
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + txIdx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("acct%d: %v", idx, err)})
					mu.Unlock()
					time.Sleep(100 * time.Millisecond)
					txIdx++
					continue
				}
				// Don't wait for inclusion — just fire and move on
				// Check inclusion asynchronously at the end
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: idx*100000 + txIdx, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true, // optimistic; will verify below
				})
				mu.Unlock()
				txIdx++
			}
		}(i)
	}
	wg.Wait()

	// Wait a bit for last transactions to be included
	time.Sleep(15 * time.Second)

	// Now verify inclusion for all
	mu.Lock()
	for i := range allResults {
		r := &allResults[i]
		if r.Error != "" {
			r.Included = false
			continue
		}
		// Check if the block height was reached
		acctIdx := r.Idx / 100000
		w := derivedAccount(uint32(acctIdx))
		_, h := frontierAccount(w.addrStr)
		if h >= r.BlockHeight {
			lat := int64(0) // can't measure individual latency in fire-and-forget
			r.LatencyMs = lat
			r.InclMomentum = 0
		} else {
			r.Included = false
		}
	}
	mu.Unlock()

	// Stop the watcher
	close(stopWatcher)
	watcherWG.Wait()

	fmAfter := frontierMomentum()
	pricesMu.Lock()
	seen := make(map[uint64]int)
	var deduped []momentumPrice
	for _, p := range prices {
		if _, ok := seen[p.Height]; !ok {
			deduped = append(deduped, p)
			seen[p.Height] = len(deduped) - 1
		} else {
			deduped[seen[p.Height]] = p
		}
	}
	pricesMu.Unlock()

	fmt.Fprintf(os.Stderr, "post-sustain: momentum=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fmAfter.Height, fmAfter.NextFusionPrice, fmAfter.NextWorkPrice)

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := fmAfter.Height
	s := summarize("sustain", allResults, startM, endM, duration, cpuSec)

	type sustainOutput struct {
		runSummary
		MomentumPrices  []momentumPrice `json:"momentum_prices"`
		PreFusionPrice  uint64          `json:"pre_fusion_price"`
		PreWorkPrice    uint64          `json:"pre_work_price"`
		PostFusionPrice uint64          `json:"post_fusion_price"`
		PostWorkPrice   uint64          `json:"post_work_price"`
	}
	out := sustainOutput{
		runSummary:      s,
		MomentumPrices:  deduped,
		PreFusionPrice:  fmBefore.NextFusionPrice,
		PreWorkPrice:    fmBefore.NextWorkPrice,
		PostFusionPrice: fmAfter.NextFusionPrice,
		PostWorkPrice:   fmAfter.NextWorkPrice,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}


// ---- Phase 9: PoW Repricing ----

// runPowBurst: PoW repricing experiment.
//
// Funds N fresh derived accounts with ZNN but NO QSR fusion. All accounts
// simultaneously send self-transfers with PoW (since they have no fused
// plasma). This drives PoW-covered plasma usage high enough to trigger
// workPrice repricing.
//
// The PoW plasma target is 1,050,000 (50 simple txs). When PoW-covered
// base plasma in a momentum exceeds this target, nextWorkPrice rises.
// We track nextWorkPrice per momentum and the required difficulty per
// transaction to observe the repricing effect.
//
// Key difference from hvburst (phase 6): hvburst used the fused agent
// account (fusion price repricing). powburst uses unfused accounts
// (work price repricing).
func runPowBurst(numAccounts int, durationSecs int, amt int64) {
	agent := agentAccount()

	// ---- Setup: fund accounts with ZNN (no fusion) ----
	fmt.Fprintf(os.Stderr, "powburst setup: funding %d accounts with ZNN (no fusion)...\n", numAccounts)
	baseIdx := uint32(200) // use high indices to avoid conflicts with prior phases
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		// Fund with enough ZNN for self-transfers
		b, _, err := buildSend(agent, w.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FUND FAIL acct %d: %v\n", idx, err)
			continue
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FUND PUBLISH FAIL acct %d: %v\n", idx, err)
			continue
		}
		// Wait for this funding tx to be included before sending the next one
		// (agent account-chain is sequential)
		waitIncluded(agent.addrStr, b.Height, 60*time.Second)
	}

	// Wait for all funding to be included, then receive
	targetHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "powburst setup: waiting for funding inclusion (height %d)...\n", targetHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= targetHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "powburst setup: receiving funded blocks...\n")
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		count := receiveAll(w)
		if count > 0 {
			fmt.Fprintf(os.Stderr, "  account %d: received %d blocks\n", idx, count)
		}
	}
	// Wait for receive blocks to be included
	recvHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "powburst setup: waiting for receive inclusion (height %d)...\n", recvHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= recvHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "powburst setup: funding complete\n")

	// Record prices before the burst
	fmBefore := frontierMomentum()
	fmt.Fprintf(os.Stderr, "pre-powburst: momentum=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice)

	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()
	deadline := tStart.Add(time.Duration(durationSecs) * time.Second)

	// Momentum watcher: record price changes
	type momentumPrice struct {
		Height          uint64 `json:"height"`
		NextFusionPrice uint64 `json:"nextFusionPrice"`
		NextWorkPrice   uint64 `json:"nextWorkPrice"`
	}
	var prices []momentumPrice
	var pricesMu sync.Mutex
	prices = append(prices, momentumPrice{fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice})

	stopWatcher := make(chan struct{})
	var watcherWG sync.WaitGroup
	watcherWG.Add(1)
	go func() {
		defer watcherWG.Done()
		lastHeight := fmBefore.Height
		for {
			select {
			case <-stopWatcher:
				return
			default:
				fm := frontierMomentum()
				if fm.Height > lastHeight {
					pricesMu.Lock()
					prices = append(prices, momentumPrice{fm.Height, fm.NextFusionPrice, fm.NextWorkPrice})
					pricesMu.Unlock()
					lastHeight = fm.Height
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	// Send transactions from all accounts in parallel until deadline
	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < numAccounts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			idx := baseIdx + uint32(i)
			w := derivedAccount(idx)
			txIdx := 0
			for time.Now().Before(deadline) {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{
						Idx: i*100000 + txIdx, Error: fmt.Sprintf("acct%d: %v", idx, err),
					})
					mu.Unlock()
					txIdx++
					continue
				}
				subM := frontierMomentum().Height
				reqDiff := b.Difficulty
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{
						Idx: i*100000 + txIdx, BlockHeight: b.Height, SubmitMomentum: subM,
						PoWMs: powMs, Error: fmt.Sprintf("acct%d: %v", idx, err),
					})
					mu.Unlock()
					txIdx++
					continue
				}
				lat, mh, ok := waitIncluded(w.addrStr, b.Height, 120*time.Second)
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: i*100000 + txIdx, BlockHeight: b.Height, SubmitMomentum: subM,
					PoWMs: powMs, Included: ok, LatencyMs: lat.Milliseconds(),
					InclMomentum: mh, PlasmaAfter: reqDiff, // reuse PlasmaAfter to store required difficulty
				})
				mu.Unlock()
				txIdx++
			}
		}(i)
	}
	wg.Wait()

	// Stop the watcher
	close(stopWatcher)
	watcherWG.Wait()

	// Record final prices
	fmAfter := frontierMomentum()
	pricesMu.Lock()
	seen := make(map[uint64]int)
	var deduped []momentumPrice
	for _, p := range prices {
		if _, ok := seen[p.Height]; !ok {
			deduped = append(deduped, p)
			seen[p.Height] = len(deduped) - 1
		} else {
			deduped[seen[p.Height]] = p
		}
	}
	pricesMu.Unlock()

	fmt.Fprintf(os.Stderr, "post-powburst: momentum=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fmAfter.Height, fmAfter.NextFusionPrice, fmAfter.NextWorkPrice)

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := fmAfter.Height
	s := summarize("powburst", allResults, startM, endM, duration, cpuSec)

	type powBurstOutput struct {
		runSummary
		MomentumPrices  []momentumPrice `json:"momentum_prices"`
		PreFusionPrice  uint64          `json:"pre_fusion_price"`
		PreWorkPrice    uint64          `json:"pre_work_price"`
		PostFusionPrice uint64          `json:"post_fusion_price"`
		PostWorkPrice   uint64          `json:"post_work_price"`
	}
	out := powBurstOutput{
		runSummary:      s,
		MomentumPrices:  deduped,
		PreFusionPrice:  fmBefore.NextFusionPrice,
		PreWorkPrice:    fmBefore.NextWorkPrice,
		PostFusionPrice: fmAfter.NextFusionPrice,
		PostWorkPrice:   fmAfter.NextWorkPrice,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}


// ---- Phase 9: PoW Latency Measurement ----

// runPowLat: measure PoW transaction latency for unfused accounts.
//
// Uses pre-funded unfused accounts (0 plasma). Each account sends one
// self-transfer with PoW. Measures:
//   - PoW computation time
//   - Total latency (build → publish → inclusion)
//   - Required difficulty (from RPC)
//
// This gives concrete numbers for the cost of using PoW vs fused plasma.
// On the lab hardware (i7-4770), PoW for a simple send takes ~30-60s.
func runPowLat(numAccounts int, amt int64) {
	baseIdx := uint32(200)

	// Verify accounts are funded
	fmt.Fprintf(os.Stderr, "powlat: verifying %d accounts are funded...\n", numAccounts)
	for i := 0; i < numAccounts; i++ {
		idx := baseIdx + uint32(i)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "powlat: account %d not funded, skipping\n", idx)
			continue
		}
	}

	fmBefore := frontierMomentum()
	fmt.Fprintf(os.Stderr, "pre-powlat: momentum=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fmBefore.Height, fmBefore.NextFusionPrice, fmBefore.NextWorkPrice)

	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	// Each account sends 1 tx with PoW, all in parallel
	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < numAccounts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			idx := baseIdx + uint32(i)
			w := derivedAccount(idx)

			// Check plasma level (should be 0)
			plasma := plasmaLevel(w.addrStr)
			if plasma > 0 {
				fmt.Fprintf(os.Stderr, "powlat: account %d has %d plasma, expected 0\n", idx, plasma)
			}

			// Query required difficulty
			req := requiredPoW(w.addrStr, nom.BlockTypeUserSend, &w.addr, nil)
			fmt.Fprintf(os.Stderr, "powlat: account %d requiredDifficulty=%d availablePlasma=%d basePlasma=%d\n",
				idx, req.RequiredDifficulty, req.AvailablePlasma, req.BasePlasma)

			// Build and send
			b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
			if err != nil {
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: i, Error: fmt.Sprintf("acct%d: %v", idx, err),
				})
				mu.Unlock()
				return
			}
			subM := frontierMomentum().Height
			reqDiff := b.Difficulty
			if err := publish(b); err != nil {
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: i, BlockHeight: b.Height, SubmitMomentum: subM,
					PoWMs: powMs, Error: fmt.Sprintf("acct%d: %v", idx, err),
				})
				mu.Unlock()
				return
			}
			lat, mh, ok := waitIncluded(w.addrStr, b.Height, 180*time.Second)
			mu.Lock()
			allResults = append(allResults, txResult{
				Idx: i, BlockHeight: b.Height, SubmitMomentum: subM,
				PoWMs: powMs, Included: ok, LatencyMs: lat.Milliseconds(),
				InclMomentum: mh, PlasmaAfter: reqDiff,
			})
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	fmAfter := frontierMomentum()
	fmt.Fprintf(os.Stderr, "post-powlat: momentum=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fmAfter.Height, fmAfter.NextFusionPrice, fmAfter.NextWorkPrice)

	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart
	endM := fmAfter.Height
	s := summarize("powlat", allResults, startM, endM, duration, cpuSec)

	type powLatOutput struct {
		runSummary
		PreFusionPrice  uint64 `json:"pre_fusion_price"`
		PreWorkPrice    uint64 `json:"pre_work_price"`
		PostFusionPrice uint64 `json:"post_fusion_price"`
		PostWorkPrice   uint64 `json:"post_work_price"`
	}
	out := powLatOutput{
		runSummary:      s,
		PreFusionPrice:  fmBefore.NextFusionPrice,
		PreWorkPrice:    fmBefore.NextWorkPrice,
		PostFusionPrice: fmAfter.NextFusionPrice,
		PostWorkPrice:   fmAfter.NextWorkPrice,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// ---- Phase 10: Mixed fused/unfused contention ----

type mixGroupBreakdown struct {
	Group           string `json:"group"` // "fused" or "unfused"
	AccountIndex    int    `json:"account_index"`
	Address         string `json:"address"`
	FusedQSR        int64  `json:"fused_qsr"`
	PlasmaAvailable uint64 `json:"plasma_available"`
	Attempted       int    `json:"attempted"`
	Submitted       int    `json:"submitted"`
	Included        int    `json:"included"`
	Rejected        int    `json:"rejected"`
	Unresolved      int    `json:"unresolved"`
	PoWCount        int    `json:"pow_count"`
	TotalPoWMs      int64  `json:"total_pow_ms"`
	P50Ms           int64  `json:"p50_ms"`
	P90Ms           int64  `json:"p90_ms"`
}

// runMixContend: mixed fused/unfused contention experiment (phase 10).
//
// Sets up two groups of accounts:
//   - "fused" accounts: funded + fused with varying QSR (like phase 7)
//   - "unfused" accounts: funded but 0 QSR fused (must use PoW for every tx)
//
// All accounts send self-transfers simultaneously, competing for momentum space.
// Measures whether fused plasma transactions get priority over PoW transactions
// when demand exceeds per-momentum capacity.
//
// Fused accounts use indices 1..numFused, unfused use indices 300..300+numUnfused-1.
func runMixContend(numFused int, numUnfused int, txPerAccount int, amt int64) {
	// ---- Setup: fund all accounts ----
	fmt.Fprintf(os.Stderr, "mixcontend setup: funding %d fused + %d unfused accounts...\n", numFused, numUnfused)
	for i := 1; i <= numFused; i++ {
		w := derivedAccount(uint32(i))
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "  funding fused account %d (%s)...\n", i, w.addrStr)
			runFund(uint32(i), 100)
		}
	}
	for j := 0; j < numUnfused; j++ {
		idx := uint32(300 + j)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "  funding unfused account %d (%s)...\n", idx, w.addrStr)
			runFund(idx, 100)
		}
	}

	// Wait for funding inclusion
	targetHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "mixcontend setup: waiting for funding inclusion (height %d)...\n", targetHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= targetHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Receive funded blocks
	fmt.Fprintf(os.Stderr, "mixcontend setup: receiving funded blocks...\n")
	for i := 1; i <= numFused; i++ {
		w := derivedAccount(uint32(i))
		receiveAll(w)
	}
	for j := 0; j < numUnfused; j++ {
		w := derivedAccount(uint32(300 + j))
		receiveAll(w)
	}

	// ---- Setup: fuse only the fused group ----
	fmt.Fprintf(os.Stderr, "mixcontend setup: fusing QSR for %d fused accounts...\n", numFused)
	for i := 1; i <= numFused; i++ {
		fuseAmt := int64(i * 20) // 20, 40, 60, ... QSR
		fmt.Fprintf(os.Stderr, "  fusing %d QSR for fused account %d...\n", fuseAmt, i)
		runFuseFor(uint32(i), fuseAmt)
	}

	// Wait for fusion inclusion
	fuseHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "mixcontend setup: waiting for fusion inclusion (height %d)...\n", fuseHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= fuseHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Record plasma levels
	fusedPlasma := make(map[int]uint64)
	for i := 1; i <= numFused; i++ {
		w := derivedAccount(uint32(i))
		fusedPlasma[i] = plasmaLevel(w.addrStr)
		fmt.Fprintf(os.Stderr, "  fused account %d: plasma=%d\n", i, fusedPlasma[i])
	}
	unfusedPlasma := make(map[int]uint64)
	for j := 0; j < numUnfused; j++ {
		idx := 300 + j
		w := derivedAccount(uint32(idx))
		unfusedPlasma[idx] = plasmaLevel(w.addrStr)
		fmt.Fprintf(os.Stderr, "  unfused account %d: plasma=%d\n", idx, unfusedPlasma[idx])
	}

	fmt.Fprintf(os.Stderr, "mixcontend: starting contention (%d fused x %d txs + %d unfused x %d txs)...\n",
		numFused, txPerAccount, numUnfused, txPerAccount)

	// ---- Contention phase ----
	fmBefore := frontierMomentum()
	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fused accounts send with plasma (no PoW needed)
	for i := 1; i <= numFused; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := derivedAccount(uint32(idx))
			for j := 0; j < txPerAccount; j++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + j, Error: fmt.Sprintf("fused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("fused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: idx*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true,
				})
				mu.Unlock()
			}
		}(i)
	}

	// Unfused accounts send with PoW
	for j := 0; j < numUnfused; j++ {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			idx := 300 + j
			w := derivedAccount(uint32(idx))
			for k := 0; k < txPerAccount; k++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + k, Error: fmt.Sprintf("unfused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: idx*100000 + k, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("unfused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: idx*100000 + k, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true,
				})
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()

	// Wait for last transactions to be included
	fmt.Fprintf(os.Stderr, "mixcontend: waiting 30s for final inclusions (PoW txs are slow)...\n")
	time.Sleep(30 * time.Second)

	// ---- Verify inclusion ----
	mu.Lock()
	for i := range allResults {
		r := &allResults[i]
		if r.Error != "" {
			r.Included = false
			continue
		}
		acctIdx := r.Idx / 100000
		w := derivedAccount(uint32(acctIdx))
		_, h := frontierAccount(w.addrStr)
		if h >= r.BlockHeight {
			r.LatencyMs = 0
			r.InclMomentum = 0
		} else {
			r.Included = false
		}
	}
	mu.Unlock()

	fmAfter := frontierMomentum()
	endM := fmAfter.Height
	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart

	// ---- Per-momentum content tracking ----
	fmt.Fprintf(os.Stderr, "mixcontend: analyzing momentum contents (%d to %d)...\n", startM, endM)
	momentumContents := make([]momentumContent, 0)
	for h := startM; h <= endM; h++ {
		var ml struct {
			List []struct {
				Height  uint64 `json:"height"`
				Content []struct {
					Address string `json:"address"`
				} `json:"content"`
			} `json:"list"`
		}
		if err := rpc("ledger.getMomentumsByHeight", []interface{}{h, 1}, &ml); err != nil {
			continue
		}
		if len(ml.List) == 0 {
			continue
		}
		acctCounts := make(map[int]int)
		for _, blk := range ml.List[0].Content {
			// Check fused accounts
			for i := 1; i <= numFused; i++ {
				w := derivedAccount(uint32(i))
				if blk.Address == w.addrStr {
					acctCounts[i]++
					break
				}
			}
			// Check unfused accounts
			for j := 0; j < numUnfused; j++ {
				idx := 300 + j
				w := derivedAccount(uint32(idx))
				if blk.Address == w.addrStr {
					acctCounts[idx]++
					break
				}
			}
		}
		if len(acctCounts) > 0 {
			momentumContents = append(momentumContents, momentumContent{
				Height:   ml.List[0].Height,
				Accounts: acctCounts,
			})
		}
	}

	// ---- Per-account breakdown ----
	var breakdowns []mixGroupBreakdown
	for i := 1; i <= numFused; i++ {
		w := derivedAccount(uint32(i))
		bd := mixGroupBreakdown{
			Group:           "fused",
			AccountIndex:    i,
			Address:         w.addrStr,
			FusedQSR:        int64(i * 20),
			PlasmaAvailable: fusedPlasma[i],
		}
		var lats []int64
		for _, r := range allResults {
			if r.Idx/100000 != i {
				continue
			}
			bd.Attempted++
			if r.Error != "" {
				bd.Rejected++
				continue
			}
			bd.Submitted++
			if r.Included {
				bd.Included++
				lats = append(lats, r.LatencyMs)
			} else {
				bd.Unresolved++
			}
			if r.PoWMs > 0 {
				bd.PoWCount++
				bd.TotalPoWMs += r.PoWMs
			}
		}
		sort.Slice(lats, func(a, b int) bool { return lats[a] < lats[b] })
		bd.P50Ms = percentile(lats, 0.50)
		bd.P90Ms = percentile(lats, 0.90)
		breakdowns = append(breakdowns, bd)
	}
	for j := 0; j < numUnfused; j++ {
		idx := 300 + j
		w := derivedAccount(uint32(idx))
		bd := mixGroupBreakdown{
			Group:           "unfused",
			AccountIndex:    idx,
			Address:         w.addrStr,
			FusedQSR:        0,
			PlasmaAvailable: unfusedPlasma[idx],
		}
		var lats []int64
		for _, r := range allResults {
			if r.Idx/100000 != idx {
				continue
			}
			bd.Attempted++
			if r.Error != "" {
				bd.Rejected++
				continue
			}
			bd.Submitted++
			if r.Included {
				bd.Included++
				lats = append(lats, r.LatencyMs)
			} else {
				bd.Unresolved++
			}
			if r.PoWMs > 0 {
				bd.PoWCount++
				bd.TotalPoWMs += r.PoWMs
			}
		}
		sort.Slice(lats, func(a, b int) bool { return lats[a] < lats[b] })
		bd.P50Ms = percentile(lats, 0.50)
		bd.P90Ms = percentile(lats, 0.90)
		breakdowns = append(breakdowns, bd)
	}

	// ---- Summary ----
	s := summarize("mixcontend", allResults, startM, endM, duration, cpuSec)

	// Count per-group totals
	var fusedAttempted, fusedIncluded, fusedRejected, fusedUnresolved int
	var unfusedAttempted, unfusedIncluded, unfusedRejected, unfusedUnresolved int
	for _, bd := range breakdowns {
		if bd.Group == "fused" {
			fusedAttempted += bd.Attempted
			fusedIncluded += bd.Included
			fusedRejected += bd.Rejected
			fusedUnresolved += bd.Unresolved
		} else {
			unfusedAttempted += bd.Attempted
			unfusedIncluded += bd.Included
			unfusedRejected += bd.Rejected
			unfusedUnresolved += bd.Unresolved
		}
	}

	totalMomentumTxs := 0
	for _, mc := range momentumContents {
		for _, c := range mc.Accounts {
			totalMomentumTxs += c
		}
	}

	type mixContendOutput struct {
		runSummary
		FusedAccounts         int                  `json:"fused_accounts"`
		UnfusedAccounts       int                  `json:"unfused_accounts"`
		FusedAttempted        int                  `json:"fused_attempted"`
		FusedIncluded         int                  `json:"fused_included"`
		FusedRejected         int                  `json:"fused_rejected"`
		FusedUnresolved       int                  `json:"fused_unresolved"`
		UnfusedAttempted      int                  `json:"unfused_attempted"`
		UnfusedIncluded       int                  `json:"unfused_included"`
		UnfusedRejected       int                  `json:"unfused_rejected"`
		UnfusedUnresolved     int                  `json:"unfused_unresolved"`
		Breakdowns            []mixGroupBreakdown  `json:"breakdowns"`
		MomentumContents      []momentumContent    `json:"momentum_contents"`
		TotalMomentumTxs      int                  `json:"total_momentum_txs"`
	}
	out := mixContendOutput{
		runSummary:        s,
		FusedAccounts:     numFused,
		UnfusedAccounts:   numUnfused,
		FusedAttempted:    fusedAttempted,
		FusedIncluded:     fusedIncluded,
		FusedRejected:     fusedRejected,
		FusedUnresolved:   fusedUnresolved,
		UnfusedAttempted:  unfusedAttempted,
		UnfusedIncluded:   unfusedIncluded,
		UnfusedRejected:   unfusedRejected,
		UnfusedUnresolved: unfusedUnresolved,
		Breakdowns:        breakdowns,
		MomentumContents:  momentumContents,
		TotalMomentumTxs:  totalMomentumTxs,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// ---- Phase 13: mixed fused/unfused saturation ----

type mixSaturateBreakdown struct {
	AccountIndex    int    `json:"account_index"`
	Address         string `json:"address"`
	Group           string `json:"group"` // "fused" or "unfused"
	FusedQSR        int64  `json:"fused_qsr"`
	PlasmaAvailable uint64 `json:"plasma_available"`
	Attempted       int    `json:"attempted"`
	Submitted       int    `json:"submitted"`
	Included        int    `json:"included"`
	Rejected        int    `json:"rejected"`
	Unresolved      int    `json:"unresolved"`
	PoWCount        int    `json:"pow_count"`
	TotalPoWMs      int64  `json:"total_pow_ms"`
	P50Ms           int64  `json:"p50_ms"`
	P90Ms           int64  `json:"p90_ms"`
}

// runMixSaturate: mixed fused/unfused saturation experiment (phase 13).
//
// N fused accounts (varying QSR) + N unfused accounts (0 QSR, PoW per tx)
// all send M self-transfers simultaneously. Total = 2*N*M, designed to
// exceed the 200 tx/momentum limit.
//
// Question: when the mempool is saturated, does plasma availability affect
// queuing priority? Or is it strictly FIFO regardless of plasma?
//
// Fused accounts use indices 100..100+N-1 (like saturate).
// Unfused accounts use indices 300..300+N-1 (like mixcontend).
func runMixSaturate(numFused int, numUnfused int, fusedTxPer int, unfusedTxPer int, amt int64) {
	fusedBase := uint32(100)
	unfusedBase := uint32(300)

	// ---- Setup: fund all accounts ----
	fmt.Fprintf(os.Stderr, "mixsaturate setup: funding %d fused + %d unfused accounts...\n", numFused, numUnfused)
	for i := 0; i < numFused; i++ {
		idx := fusedBase + uint32(i)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "  funding fused account %d (%s)...\n", idx, w.addrStr)
			runFund(idx, 100)
		}
	}
	for j := 0; j < numUnfused; j++ {
		idx := unfusedBase + uint32(j)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h == 0 {
			fmt.Fprintf(os.Stderr, "  funding unfused account %d (%s)...\n", idx, w.addrStr)
			runFund(idx, 100)
		}
	}

	// Wait for funding inclusion
	targetHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "mixsaturate setup: waiting for funding inclusion (height %d)...\n", targetHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= targetHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Receive funded blocks (skip accounts that already have height > 1, meaning
	// they already received in a prior run)
	fmt.Fprintf(os.Stderr, "mixsaturate setup: receiving funded blocks (skipping already-received)...\n")
	for i := 0; i < numFused; i++ {
		idx := fusedBase + uint32(i)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h <= 1 {
			receiveAll(w)
		}
	}
	for j := 0; j < numUnfused; j++ {
		idx := unfusedBase + uint32(j)
		w := derivedAccount(idx)
		_, h := frontierAccount(w.addrStr)
		if h <= 1 {
			receiveAll(w)
		}
	}

	// ---- Setup: fuse only the fused group (skip if already has plasma) ----
	fmt.Fprintf(os.Stderr, "mixsaturate setup: fusing QSR for %d fused accounts (skipping already-fused)...\n", numFused)
	for i := 0; i < numFused; i++ {
		idx := fusedBase + uint32(i)
		w := derivedAccount(idx)
		existingPlasma := plasmaLevel(w.addrStr)
		if existingPlasma > 0 {
			fmt.Fprintf(os.Stderr, "  skipping fused account %d (already has plasma=%d)\n", idx, existingPlasma)
			continue
		}
		fuseAmt := int64((i + 1) * 20) // 20, 40, 60, ... QSR
		fmt.Fprintf(os.Stderr, "  fusing %d QSR for fused account %d...\n", fuseAmt, idx)
		runFuseFor(idx, fuseAmt)
	}

	// Wait for fusion inclusion
	fuseHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "mixsaturate setup: waiting for fusion inclusion (height %d)...\n", fuseHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= fuseHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Record plasma levels
	fusedPlasma := make(map[uint32]uint64)
	for i := 0; i < numFused; i++ {
		idx := fusedBase + uint32(i)
		w := derivedAccount(idx)
		fusedPlasma[idx] = plasmaLevel(w.addrStr)
		fmt.Fprintf(os.Stderr, "  fused account %d: plasma=%d\n", idx, fusedPlasma[idx])
	}
	unfusedPlasma := make(map[uint32]uint64)
	for j := 0; j < numUnfused; j++ {
		idx := unfusedBase + uint32(j)
		w := derivedAccount(idx)
		unfusedPlasma[idx] = plasmaLevel(w.addrStr)
		fmt.Fprintf(os.Stderr, "  unfused account %d: plasma=%d\n", idx, unfusedPlasma[idx])
	}

	totalTxs := numFused*fusedTxPer + numUnfused*unfusedTxPer
	fmt.Fprintf(os.Stderr, "mixsaturate: starting saturation (%d fused x %d + %d unfused x %d = %d total, limit=200/momentum)...\n",
		numFused, fusedTxPer, numUnfused, unfusedTxPer, totalTxs)

	// ---- Saturation phase ----
	fmBefore := frontierMomentum()
	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fused accounts send with plasma (no PoW needed)
	for i := 0; i < numFused; i++ {
		wg.Add(1)
		go func(idx uint32) {
			defer wg.Done()
			w := derivedAccount(idx)
			for j := 0; j < fusedTxPer; j++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: int(idx)*100000 + j, Error: fmt.Sprintf("fused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: int(idx)*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("fused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: int(idx)*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true,
				})
				mu.Unlock()
			}
		}(fusedBase + uint32(i))
	}

	// Unfused accounts send with PoW
	for j := 0; j < numUnfused; j++ {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			idx := unfusedBase + uint32(j)
			w := derivedAccount(idx)
			for k := 0; k < unfusedTxPer; k++ {
				b, powMs, err := buildSend(w, w.addr, znn, dec(amt), nil, true)
				if err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: int(idx)*100000 + k, Error: fmt.Sprintf("unfused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				subM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					allResults = append(allResults, txResult{Idx: int(idx)*100000 + k, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs, Error: fmt.Sprintf("unfused%d: %v", idx, err)})
					mu.Unlock()
					continue
				}
				mu.Lock()
				allResults = append(allResults, txResult{
					Idx: int(idx)*100000 + k, BlockHeight: b.Height, SubmitMomentum: subM, PoWMs: powMs,
					Included: true,
				})
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()

	// Wait for all transactions to be included (may take multiple momenta)
	fmt.Fprintf(os.Stderr, "mixsaturate: waiting 90s for all inclusions (backlog drain + PoW delays)...\n")
	time.Sleep(90 * time.Second)

	// ---- Verify inclusion ----
	mu.Lock()
	for i := range allResults {
		r := &allResults[i]
		if r.Error != "" {
			r.Included = false
			continue
		}
		acctIdx := uint32(r.Idx / 100000)
		w := derivedAccount(acctIdx)
		_, h := frontierAccount(w.addrStr)
		if h >= r.BlockHeight {
			r.LatencyMs = 0
			r.InclMomentum = 0
		} else {
			r.Included = false
		}
	}
	mu.Unlock()

	fmAfter := frontierMomentum()
	endM := fmAfter.Height
	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart

	// ---- Per-momentum content tracking ----
	fmt.Fprintf(os.Stderr, "mixsaturate: analyzing momentum contents (%d to %d)...\n", startM, endM)
	momentumContents := make([]momentumContent, 0)
	for h := startM; h <= endM; h++ {
		var ml struct {
			List []struct {
				Height  uint64 `json:"height"`
				Content []struct {
					Address string `json:"address"`
				} `json:"content"`
			} `json:"list"`
		}
		if err := rpc("ledger.getMomentumsByHeight", []interface{}{h, 1}, &ml); err != nil {
			continue
		}
		if len(ml.List) == 0 {
			continue
		}
		acctCounts := make(map[int]int)
		for _, blk := range ml.List[0].Content {
			// Check fused accounts
			for i := 0; i < numFused; i++ {
				idx := int(fusedBase) + i
				w := derivedAccount(uint32(idx))
				if blk.Address == w.addrStr {
					acctCounts[idx]++
					break
				}
			}
			// Check unfused accounts
			for j := 0; j < numUnfused; j++ {
				idx := int(unfusedBase) + j
				w := derivedAccount(uint32(idx))
				if blk.Address == w.addrStr {
					acctCounts[idx]++
					break
				}
			}
		}
		if len(acctCounts) > 0 {
			momentumContents = append(momentumContents, momentumContent{
				Height:   ml.List[0].Height,
				Accounts: acctCounts,
			})
		}
	}

	// ---- Per-account breakdown ----
	var breakdowns []mixSaturateBreakdown
	for i := 0; i < numFused; i++ {
		idx := fusedBase + uint32(i)
		w := derivedAccount(idx)
		bd := mixSaturateBreakdown{
			AccountIndex:    int(idx),
			Address:         w.addrStr,
			Group:           "fused",
			FusedQSR:        int64((i + 1) * 20),
			PlasmaAvailable: fusedPlasma[idx],
		}
		var lats []int64
		for _, r := range allResults {
			if uint32(r.Idx/100000) != idx {
				continue
			}
			bd.Attempted++
			if r.Error != "" {
				bd.Rejected++
				continue
			}
			bd.Submitted++
			if r.Included {
				bd.Included++
				lats = append(lats, r.LatencyMs)
			} else {
				bd.Unresolved++
			}
			if r.PoWMs > 0 {
				bd.PoWCount++
				bd.TotalPoWMs += r.PoWMs
			}
		}
		sort.Slice(lats, func(a, b int) bool { return lats[a] < lats[b] })
		bd.P50Ms = percentile(lats, 0.50)
		bd.P90Ms = percentile(lats, 0.90)
		breakdowns = append(breakdowns, bd)
	}
	for j := 0; j < numUnfused; j++ {
		idx := unfusedBase + uint32(j)
		w := derivedAccount(idx)
		bd := mixSaturateBreakdown{
			AccountIndex:    int(idx),
			Address:         w.addrStr,
			Group:           "unfused",
			FusedQSR:        0,
			PlasmaAvailable: unfusedPlasma[idx],
		}
		var lats []int64
		for _, r := range allResults {
			if uint32(r.Idx/100000) != idx {
				continue
			}
			bd.Attempted++
			if r.Error != "" {
				bd.Rejected++
				continue
			}
			bd.Submitted++
			if r.Included {
				bd.Included++
				lats = append(lats, r.LatencyMs)
			} else {
				bd.Unresolved++
			}
			if r.PoWMs > 0 {
				bd.PoWCount++
				bd.TotalPoWMs += r.PoWMs
			}
		}
		sort.Slice(lats, func(a, b int) bool { return lats[a] < lats[b] })
		bd.P50Ms = percentile(lats, 0.50)
		bd.P90Ms = percentile(lats, 0.90)
		breakdowns = append(breakdowns, bd)
	}

	// ---- Summary ----
	s := summarize("mixsaturate", allResults, startM, endM, duration, cpuSec)

	// Count per-group totals
	var fusedAttempted, fusedIncluded, fusedRejected, fusedUnresolved int
	var unfusedAttempted, unfusedIncluded, unfusedRejected, unfusedUnresolved int
	for _, bd := range breakdowns {
		if bd.Group == "fused" {
			fusedAttempted += bd.Attempted
			fusedIncluded += bd.Included
			fusedRejected += bd.Rejected
			fusedUnresolved += bd.Unresolved
		} else {
			unfusedAttempted += bd.Attempted
			unfusedIncluded += bd.Included
			unfusedRejected += bd.Rejected
			unfusedUnresolved += bd.Unresolved
		}
	}

	totalMomentumTxs := 0
	for _, mc := range momentumContents {
		for _, c := range mc.Accounts {
			totalMomentumTxs += c
		}
	}

	type mixSaturateOutput struct {
		runSummary
		FusedAccounts     int                    `json:"fused_accounts"`
		UnfusedAccounts   int                    `json:"unfused_accounts"`
		FusedTxPer        int                    `json:"fused_tx_per"`
		UnfusedTxPer      int                    `json:"unfused_tx_per"`
		FusedAttempted    int                    `json:"fused_attempted"`
		FusedIncluded     int                    `json:"fused_included"`
		FusedRejected     int                    `json:"fused_rejected"`
		FusedUnresolved   int                    `json:"fused_unresolved"`
		UnfusedAttempted  int                    `json:"unfused_attempted"`
		UnfusedIncluded   int                    `json:"unfused_included"`
		UnfusedRejected   int                    `json:"unfused_rejected"`
		UnfusedUnresolved int                    `json:"unfused_unresolved"`
		Breakdowns        []mixSaturateBreakdown `json:"breakdowns"`
		MomentumContents  []momentumContent      `json:"momentum_contents"`
		TotalMomentumTxs  int                    `json:"total_momentum_txs"`
	}
	out := mixSaturateOutput{
		runSummary:        s,
		FusedAccounts:     numFused,
		UnfusedAccounts:   numUnfused,
		FusedTxPer:        fusedTxPer,
		UnfusedTxPer:      unfusedTxPer,
		FusedAttempted:    fusedAttempted,
		FusedIncluded:     fusedIncluded,
		FusedRejected:     fusedRejected,
		FusedUnresolved:   fusedUnresolved,
		UnfusedAttempted:  unfusedAttempted,
		UnfusedIncluded:   unfusedIncluded,
		UnfusedRejected:   unfusedRejected,
		UnfusedUnresolved: unfusedUnresolved,
		Breakdowns:        breakdowns,
		MomentumContents:  momentumContents,
		TotalMomentumTxs:  totalMomentumTxs,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}


// ---- Phase 14: Content Selector Price Priority ----

// buildSendOverpay creates a self-transfer with a custom FusedPlasma amount,
// allowing overpayment above the base plasma requirement. This changes the
// block's price-per-base-plasma ratio in the content selector.
func buildSendOverpay(a account, to types.Address, zts types.ZenonTokenStandard, amount *big.Int, data []byte, overpayMultiplier int) (*nom.AccountBlock, error) {
	prevHash, height := frontierAccount(a.addrStr)
	fm := frontierMomentum()
	b := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      chainID,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         prevHash,
		Height:               height + 1,
		MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
		Address:              a.addr,
		ToAddress:            to,
		Amount:               amount,
		TokenStandard:        zts,
		Data:                 data,
	}
	req := requiredPoW(a.addrStr, b.BlockType, &to, data)
	// Set FusedPlasma to overpayMultiplier × base plasma
	b.FusedPlasma = req.BasePlasma * uint64(overpayMultiplier)
	b.Hash = b.ComputeHash()
	b.PublicKey = a.kp.Public
	b.Signature = a.kp.Sign(b.Hash.Bytes())
	return b, nil
}

type overpayBreakdown struct {
	AccountIndex    int    `json:"account_index"`
	Address         string `json:"address"`
	Group           string `json:"group"`
	OverpayMult     int    `json:"overpay_multiplier"`
	FusedQSR        int64  `json:"fused_qsr"`
	PlasmaAvailable uint64 `json:"plasma_available"`
	Attempted       int    `json:"attempted"`
	Submitted       int    `json:"submitted"`
	Included        int    `json:"included"`
	Rejected        int    `json:"rejected"`
	Unresolved      int    `json:"unresolved"`
}

type overpayMomentumContent struct {
	Height   uint64 `json:"height"`
	Accounts map[int]int `json:"accounts"`
}

// runOverpay: content selector price priority experiment (phase 14).
//
// Creates N groups of accounts with different overpay multipliers (1x, 2x, 4x, 8x, ...).
// Each account sends M self-transfers with FusedPlasma = multiplier × basePlasma.
// Total transactions exceed the 200 tx/momentum limit.
//
// If the content selector's price-based priority works:
// - Higher-priced (overpaid) transactions should be included in earlier momenta
// - Lower-priced (1x) transactions should be deferred to later momenta
//
// If selection is random/FIFO (as phase 13 suggested for same-price txs):
// - Excluded transactions should be randomly distributed across all groups
//
// Account indices: group g uses indices 500 + g*100 + i (g=0..groups-1, i=0..accountsPerGroup-1)
// This avoids overlap with previous phases (100=saturate, 300=mixcontend/mixsaturate).
func runOverpay(groups int, accountsPerGroup int, txPerAcct int, amt int64) {
	baseIdx := uint32(900)

	// Calculate fusion amounts per group
	// Each account needs: txPerAcct × multiplier × 21000 plasma
	// QSR needed = plasma / 2100
	type groupConfig struct {
		multiplier int
		qsrNeeded  int64
	}
	var groupConfigs []groupConfig
	for g := 0; g < groups; g++ {
		mult := 1 << uint(g) // 1, 2, 4, 8, ...
		plasmaNeeded := int64(txPerAcct * mult * 21000)
		qsrNeeded := plasmaNeeded / 2100
		// Add 20% buffer
		qsrNeeded = qsrNeeded * 12 / 10
		groupConfigs = append(groupConfigs, groupConfig{mult, qsrNeeded})
	}

	totalAccounts := groups * accountsPerGroup
	totalTxs := totalAccounts * txPerAcct
	fmt.Fprintf(os.Stderr, "overpay setup: %d groups x %d accounts x %d txs = %d total txs (limit=200/momentum)\n",
		groups, accountsPerGroup, txPerAcct, totalTxs)
	for g, gc := range groupConfigs {
		fmt.Fprintf(os.Stderr, "  group %d: %dx overpay, %d QSR per account\n", g, gc.multiplier, gc.qsrNeeded)
	}

	// ---- Setup: fund all accounts (sequential - agent account chain) ----
	fmt.Fprintf(os.Stderr, "overpay setup: funding %d accounts...\n", totalAccounts)
	for g := 0; g < groups; g++ {
		for i := 0; i < accountsPerGroup; i++ {
			idx := baseIdx + uint32(g*100+i)
			w := derivedAccount(idx)
			_, h := frontierAccount(w.addrStr)
			if h == 0 {
				fmt.Fprintf(os.Stderr, "  funding account %d...\n", idx)
				runFund(idx, 100)
			}
		}
	}

	// Wait for funding inclusion
	targetHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "overpay setup: waiting for funding inclusion (height %d)...\n", targetHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= targetHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Receive funded blocks in parallel
	fmt.Fprintf(os.Stderr, "overpay setup: receiving funded blocks (parallel)...\n")
	var recvWg sync.WaitGroup
	for g := 0; g < groups; g++ {
		for i := 0; i < accountsPerGroup; i++ {
			recvWg.Add(1)
			go func(g, i int) {
				defer recvWg.Done()
				idx := baseIdx + uint32(g*100+i)
				w := derivedAccount(idx)
				_, h := frontierAccount(w.addrStr)
				if h <= 1 {
					receiveAll(w)
				}
			}(g, i)
		}
	}
	recvWg.Wait()

	// ---- Setup: fuse QSR for all accounts (sequential - agent account chain) ----
	fmt.Fprintf(os.Stderr, "overpay setup: fusing QSR for %d accounts...\n", totalAccounts)
	for g := 0; g < groups; g++ {
		for i := 0; i < accountsPerGroup; i++ {
			idx := baseIdx + uint32(g*100+i)
			w := derivedAccount(idx)
			existingPlasma := plasmaLevel(w.addrStr)
			if existingPlasma > 0 {
				fmt.Fprintf(os.Stderr, "  skipping account %d (already has plasma=%d)\n", idx, existingPlasma)
				continue
			}
			fmt.Fprintf(os.Stderr, "  fusing %d QSR for account %d...\n", groupConfigs[g].qsrNeeded, idx)
			runFuseFor(idx, groupConfigs[g].qsrNeeded)
		}
	}

	// Wait for fusion inclusion
	fuseHeight := frontierMomentum().Height + 2
	fmt.Fprintf(os.Stderr, "overpay setup: waiting for fusion inclusion (height %d)...\n", fuseHeight)
	for {
		fm := frontierMomentum()
		if fm.Height >= fuseHeight {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Record plasma levels
	plasmaLevels := make(map[uint32]uint64)
	for g := 0; g < groups; g++ {
		for i := 0; i < accountsPerGroup; i++ {
			idx := baseIdx + uint32(g*100+i)
			w := derivedAccount(idx)
			plasmaLevels[idx] = plasmaLevel(w.addrStr)
		}
	}

	fmt.Fprintf(os.Stderr, "overpay: starting experiment (%d total txs)...\n", totalTxs)

	// ---- Experiment phase ----
	fmBefore := frontierMomentum()
	startM := fmBefore.Height
	cpuStart := getCPUTime()
	tStart := time.Now()

	var allResults []txResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// All accounts send txs concurrently
	for g := 0; g < groups; g++ {
		for i := 0; i < accountsPerGroup; i++ {
			wg.Add(1)
			go func(g, i int) {
				defer wg.Done()
				idx := baseIdx + uint32(g*100+i)
				w := derivedAccount(idx)
				mult := groupConfigs[g].multiplier
				for j := 0; j < txPerAcct; j++ {
					b, err := buildSendOverpay(w, w.addr, znn, dec(amt), nil, mult)
					if err != nil {
						mu.Lock()
						allResults = append(allResults, txResult{Idx: int(idx)*100000 + j, Error: fmt.Sprintf("acct%d: %v", idx, err)})
						mu.Unlock()
						continue
					}
					subM := frontierMomentum().Height
					if err := publish(b); err != nil {
						mu.Lock()
						allResults = append(allResults, txResult{Idx: int(idx)*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM, Error: fmt.Sprintf("acct%d: %v", idx, err)})
						mu.Unlock()
						continue
					}
					mu.Lock()
					allResults = append(allResults, txResult{
						Idx: int(idx)*100000 + j, BlockHeight: b.Height, SubmitMomentum: subM,
						Included: true,
					})
					mu.Unlock()
				}
			}(g, i)
		}
	}
	wg.Wait()

	// Wait for all transactions to be included
	waitSecs := 90
	if totalTxs > 300 {
		waitSecs = 120
	}
	fmt.Fprintf(os.Stderr, "overpay: waiting %ds for all inclusions (backlog drain)...\n", waitSecs)
	time.Sleep(time.Duration(waitSecs) * time.Second)

	// ---- Verify inclusion ----
	mu.Lock()
	for i := range allResults {
		r := &allResults[i]
		if r.Error != "" {
			r.Included = false
			continue
		}
		acctIdx := uint32(r.Idx / 100000)
		w := derivedAccount(acctIdx)
		_, h := frontierAccount(w.addrStr)
		if h >= r.BlockHeight {
			r.LatencyMs = 0
			r.InclMomentum = 0
		} else {
			r.Included = false
		}
	}
	mu.Unlock()

	fmAfter := frontierMomentum()
	endM := fmAfter.Height
	duration := time.Since(tStart).Seconds()
	cpuSec := getCPUTime() - cpuStart

	// ---- Per-momentum content tracking ----
	fmt.Fprintf(os.Stderr, "overpay: analyzing momentum contents (%d to %d)...\n", startM, endM)
	var momentumContents []overpayMomentumContent
	for h := startM; h <= endM; h++ {
		var ml struct {
			List []struct {
				Height  uint64 `json:"height"`
				Content []struct {
					Address string `json:"address"`
				} `json:"content"`
			} `json:"list"`
		}
		if err := rpc("ledger.getMomentumsByHeight", []interface{}{h, 1}, &ml); err != nil {
			continue
		}
		if len(ml.List) == 0 {
			continue
		}
		acctCounts := make(map[int]int)
		for _, blk := range ml.List[0].Content {
			for g := 0; g < groups; g++ {
				for i := 0; i < accountsPerGroup; i++ {
					idx := int(baseIdx) + g*100 + i
					w := derivedAccount(uint32(idx))
					if blk.Address == w.addrStr {
						acctCounts[idx]++
						break
					}
				}
			}
		}
		if len(acctCounts) > 0 {
			momentumContents = append(momentumContents, overpayMomentumContent{
				Height:   ml.List[0].Height,
				Accounts: acctCounts,
			})
		}
	}

	// ---- Per-account breakdown ----
	var breakdowns []overpayBreakdown
	for g := 0; g < groups; g++ {
		for i := 0; i < accountsPerGroup; i++ {
			idx := baseIdx + uint32(g*100+i)
			w := derivedAccount(idx)
			bd := overpayBreakdown{
				AccountIndex:    int(idx),
				Address:         w.addrStr,
				Group:           fmt.Sprintf("%dx", groupConfigs[g].multiplier),
				OverpayMult:     groupConfigs[g].multiplier,
				FusedQSR:        groupConfigs[g].qsrNeeded,
				PlasmaAvailable: plasmaLevels[idx],
			}
			for _, r := range allResults {
				if uint32(r.Idx/100000) != idx {
					continue
				}
				bd.Attempted++
				if r.Error != "" {
					bd.Rejected++
					continue
				}
				bd.Submitted++
				if r.Included {
					bd.Included++
				} else {
					bd.Unresolved++
				}
			}
			breakdowns = append(breakdowns, bd)
		}
	}

	// ---- Summary ----
	s := summarize("overpay", allResults, startM, endM, duration, cpuSec)

	// Count per-group totals
	groupStats := make(map[string]map[string]int)
	for _, bd := range breakdowns {
		if _, ok := groupStats[bd.Group]; !ok {
			groupStats[bd.Group] = map[string]int{"attempted": 0, "included": 0, "rejected": 0, "unresolved": 0}
		}
		groupStats[bd.Group]["attempted"] += bd.Attempted
		groupStats[bd.Group]["included"] += bd.Included
		groupStats[bd.Group]["rejected"] += bd.Rejected
		groupStats[bd.Group]["unresolved"] += bd.Unresolved
	}

	type overpayOutput struct {
		runSummary
		Groups           int                      `json:"groups"`
		AccountsPerGroup int                      `json:"accounts_per_group"`
		TxPerAcct        int                      `json:"tx_per_acct"`
		TotalTxs         int                      `json:"total_txs"`
		MomentumLimit    int                      `json:"momentum_limit"`
		GroupStats       map[string]map[string]int `json:"group_stats"`
		Breakdowns       []overpayBreakdown       `json:"breakdowns"`
		MomentumContents []overpayMomentumContent `json:"momentum_contents"`
	}

	out := overpayOutput{
		runSummary:       s,
		Groups:           groups,
		AccountsPerGroup: accountsPerGroup,
		TxPerAcct:        txPerAcct,
		TotalTxs:         totalTxs,
		MomentumLimit:    200,
		GroupStats:       groupStats,
		Breakdowns:       breakdowns,
		MomentumContents: momentumContents,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}


// runTxType: transaction type diversity experiment (phase 16).
//
// Tests plasma costs and inclusion behavior for different transaction types
// from a single fused account. All prior phases used only self-transfers
// (BlockTypeUserSend with nil data, 21000 plasma each). This experiment
// measures the plasma cost ladder:
//
//  1. Self-transfer (baseline):     21000 plasma
//  2. Send with 100-byte data:      21000 + 68*100 = 27800 plasma
//  3. Cross-account send + receive:  21000 (send) + 21000 (receive) = 42000 total
//  4. Contract call (stake):        52500 plasma (EmbeddedSimple)
//
// For each type, sends 3 transactions sequentially and records plasma
// consumption, inclusion latency, and PoW usage.
func runTxType(idx uint32, qsrAmt int64) {
	agent := agentAccount()
	target := derivedAccount(idx)
	partner := derivedAccount(idx + 100) // separate partner account for cross-account tests

	// Step 0: Fund and fuse target account
	_, targetHeight := frontierAccount(target.addrStr)
	if targetHeight == 0 {
		fmt.Fprintf(os.Stderr, "txtype: funding account[%d] with 100 ZNN...\n", idx)
		b, _, err := buildSend(agent, target.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	// Fund partner account for cross-account receive test
	_, partnerHeight := frontierAccount(partner.addrStr)
	if partnerHeight == 0 {
		fmt.Fprintf(os.Stderr, "txtype: funding partner account[%d] with 10 ZNN...\n", idx+100)
		b, _, err := buildSend(agent, partner.addr, znn, dec(10), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund partner: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund partner: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	// Wait for funding to arrive at target
	fmt.Fprintf(os.Stderr, "txtype: waiting for funding to arrive at account[%d]...\n", idx)
	for i := 0; i < 30; i++ {
		var ul unreceivedList
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{target.addrStr, 0, 50}, &ul)
		if len(ul.List) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "txtype: receiving pending blocks on account[%d]...\n", idx)
	receiveAll(target)

	// Wait for partner funding to arrive
	fmt.Fprintf(os.Stderr, "txtype: waiting for funding to arrive at partner[%d]...\n", idx+100)
	for i := 0; i < 30; i++ {
		var ul unreceivedList
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{partner.addrStr, 0, 50}, &ul)
		if len(ul.List) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	receiveAll(partner)

	// Fuse QSR for plasma
	fmt.Fprintf(os.Stderr, "txtype: fusing %d QSR on account[%d]...\n", qsrAmt, idx)
	fuseData := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, target.addr)
	fb, _, err := buildSend(agent, types.PlasmaContract, qsr, dec(qsrAmt), fuseData, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL fuse: %v\n", err)
		os.Exit(1)
	}
	if err := publish(fb); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish fuse: %v\n", err)
		os.Exit(1)
	}
	waitIncluded(agent.addrStr, fb.Height, 120*time.Second)

	// Wait for fusion to be confirmed and plasma available
	fmt.Fprintf(os.Stderr, "txtype: waiting for fusion to confirm...\n")
	for i := 0; i < 60; i++ {
		p := plasmaLevel(target.addrStr)
		if p >= uint64(qsrAmt*2100) {
			break
		}
		time.Sleep(2 * time.Second)
	}

	initialPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "txtype: initial plasma=%d (fused %d QSR)\n", initialPlasma, qsrAmt)

	type txTypeResult struct {
		TxType          string `json:"tx_type"`
		PlannedCost     uint64 `json:"planned_cost"`
		PlasmaBefore    uint64 `json:"plasma_before"`
		PlasmaAfter     uint64 `json:"plasma_after"`
		ActualCost      uint64 `json:"actual_cost"`
		InclusionLatencyMs int64 `json:"inclusion_latency_ms"`
		PoWUsed         bool   `json:"pow_used"`
		Included        bool   `json:"included"`
	}

	var typeResults []txTypeResult
	fusedPlasma := uint64(qsrAmt * 2100)

	// --- Test 1: Self-transfer baseline (3 txs) ---
	fmt.Fprintf(os.Stderr, "txtype: testing self-transfer baseline...\n")
	for i := 0; i < 3; i++ {
		plasmaBefore := plasmaLevel(target.addrStr)
		b, powMs, err := buildSend(target, target.addr, znn, dec(1), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "txtype: self-transfer %d BUILD FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "self_transfer", PlannedCost: 21000,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "txtype: self-transfer %d PUBLISH FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "self_transfer", PlannedCost: 21000,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		lat, _, ok := waitIncluded(target.addrStr, b.Height, 60*time.Second)
		plasmaAfter := plasmaLevel(target.addrStr)
		actualCost := plasmaBefore - plasmaAfter
		if plasmaAfter > plasmaBefore {
			actualCost = 0 // regen during test
		}
		typeResults = append(typeResults, txTypeResult{
			TxType: "self_transfer", PlannedCost: 21000,
			PlasmaBefore: plasmaBefore, PlasmaAfter: plasmaAfter,
			ActualCost: actualCost,
			InclusionLatencyMs: lat.Milliseconds(),
			PoWUsed:            powMs > 0,
			Included:           ok,
		})
		fmt.Fprintf(os.Stderr, "txtype: self-transfer %d included=%v cost=%d plasma=%d->%d\n",
			i, ok, actualCost, plasmaBefore, plasmaAfter)
	}

	// --- Test 2: Send with 100-byte data payload (3 txs) ---
	fmt.Fprintf(os.Stderr, "txtype: testing send with data payload...\n")
	dataPayload := make([]byte, 100)
	for i := range dataPayload {
		dataPayload[i] = byte(i % 256)
	}
	expectedDataCost := uint64(21000 + 68*100) // 27800
	for i := 0; i < 3; i++ {
		plasmaBefore := plasmaLevel(target.addrStr)
		b, powMs, err := buildSend(target, target.addr, znn, dec(1), dataPayload, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "txtype: data-send %d BUILD FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "send_with_data", PlannedCost: expectedDataCost,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "txtype: data-send %d PUBLISH FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "send_with_data", PlannedCost: expectedDataCost,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		lat, _, ok := waitIncluded(target.addrStr, b.Height, 60*time.Second)
		plasmaAfter := plasmaLevel(target.addrStr)
		actualCost := plasmaBefore - plasmaAfter
		if plasmaAfter > plasmaBefore {
			actualCost = 0
		}
		typeResults = append(typeResults, txTypeResult{
			TxType: "send_with_data", PlannedCost: expectedDataCost,
			PlasmaBefore: plasmaBefore, PlasmaAfter: plasmaAfter,
			ActualCost: actualCost,
			InclusionLatencyMs: lat.Milliseconds(),
			PoWUsed:            powMs > 0,
			Included:           ok,
		})
		fmt.Fprintf(os.Stderr, "txtype: data-send %d included=%v cost=%d (planned %d) plasma=%d->%d\n",
			i, ok, actualCost, expectedDataCost, plasmaBefore, plasmaAfter)
	}

	// --- Test 3: Cross-account send + receive (3 rounds) ---
	fmt.Fprintf(os.Stderr, "txtype: testing cross-account send + receive...\n")
	for i := 0; i < 3; i++ {
		// Send from target to partner
		plasmaBeforeSend := plasmaLevel(target.addrStr)
		b, powMs, err := buildSend(target, partner.addr, znn, dec(1), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "txtype: cross-send %d BUILD FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "cross_send", PlannedCost: 21000,
				PlasmaBefore: plasmaBeforeSend, Included: false,
			})
			continue
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "txtype: cross-send %d PUBLISH FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "cross_send", PlannedCost: 21000,
				PlasmaBefore: plasmaBeforeSend, Included: false,
			})
			continue
		}
		latSend, _, okSend := waitIncluded(target.addrStr, b.Height, 60*time.Second)
		plasmaAfterSend := plasmaLevel(target.addrStr)
		sendCost := plasmaBeforeSend - plasmaAfterSend
		if plasmaAfterSend > plasmaBeforeSend {
			sendCost = 0
		}
		typeResults = append(typeResults, txTypeResult{
			TxType: "cross_send", PlannedCost: 21000,
			PlasmaBefore: plasmaBeforeSend, PlasmaAfter: plasmaAfterSend,
			ActualCost: sendCost,
			InclusionLatencyMs: latSend.Milliseconds(),
			PoWUsed:            powMs > 0,
			Included:           okSend,
		})
		fmt.Fprintf(os.Stderr, "txtype: cross-send %d included=%v cost=%d\n", i, okSend, sendCost)

		// Partner receives — wait for unreceived block to appear
		for w := 0; w < 15; w++ {
			var ul unreceivedList
			mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{partner.addrStr, 0, 50}, &ul)
			if len(ul.List) > 0 {
				break
			}
			time.Sleep(2 * time.Second)
		}
		plasmaBeforeRecv := plasmaLevel(partner.addrStr)
		recvCount := receiveAll(partner)
		plasmaAfterRecv := plasmaLevel(partner.addrStr)
		recvCost := plasmaBeforeRecv - plasmaAfterRecv
		if plasmaAfterRecv > plasmaBeforeRecv {
			recvCost = 0
		}
		typeResults = append(typeResults, txTypeResult{
			TxType: "cross_receive", PlannedCost: 21000,
			PlasmaBefore: plasmaBeforeRecv, PlasmaAfter: plasmaAfterRecv,
			ActualCost:       recvCost,
			PoWUsed:          false,
			Included:         recvCount > 0,
		})
		fmt.Fprintf(os.Stderr, "txtype: cross-receive %d received=%d cost=%d\n", i, recvCount, recvCost)
	}

	// --- Test 4: Contract call — Stake (3 txs, 52500 plasma each) ---
	fmt.Fprintf(os.Stderr, "txtype: testing contract call (stake)...\n")
	expectedStakeCost := uint64(52500)
	for i := 0; i < 3; i++ {
		plasmaBefore := plasmaLevel(target.addrStr)
		if plasmaBefore < expectedStakeCost {
			fmt.Fprintf(os.Stderr, "txtype: insufficient plasma for stake (%d < %d), skipping\n",
				plasmaBefore, expectedStakeCost)
			typeResults = append(typeResults, txTypeResult{
				TxType: "contract_stake", PlannedCost: expectedStakeCost,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		stakeData := definition.ABIStake.PackMethodPanic(definition.StakeMethodName, int64(2592000))
		b, powMs, err := buildSend(target, types.StakeContract, znn, dec(1), stakeData, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "txtype: stake %d BUILD FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "contract_stake", PlannedCost: expectedStakeCost,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "txtype: stake %d PUBLISH FAILED: %v\n", i, err)
			typeResults = append(typeResults, txTypeResult{
				TxType: "contract_stake", PlannedCost: expectedStakeCost,
				PlasmaBefore: plasmaBefore, Included: false,
			})
			continue
		}
		lat, _, ok := waitIncluded(target.addrStr, b.Height, 60*time.Second)
		plasmaAfter := plasmaLevel(target.addrStr)
		actualCost := plasmaBefore - plasmaAfter
		if plasmaAfter > plasmaBefore {
			actualCost = 0
		}
		typeResults = append(typeResults, txTypeResult{
			TxType: "contract_stake", PlannedCost: expectedStakeCost,
			PlasmaBefore: plasmaBefore, PlasmaAfter: plasmaAfter,
			ActualCost: actualCost,
			InclusionLatencyMs: lat.Milliseconds(),
			PoWUsed:            powMs > 0,
			Included:           ok,
		})
		fmt.Fprintf(os.Stderr, "txtype: stake %d included=%v cost=%d (planned %d) plasma=%d->%d\n",
			i, ok, actualCost, expectedStakeCost, plasmaBefore, plasmaAfter)
	}

	// --- Summary ---
	type txTypeSummary struct {
		AccountIndex   uint64           `json:"account_index"`
		AccountAddress string           `json:"account_address"`
		QSRFused       int64            `json:"qsr_fused"`
		FusedPlasma    uint64           `json:"fused_plasma"`
		InitialPlasma  uint64           `json:"initial_plasma"`
		FinalPlasma    uint64           `json:"final_plasma"`
		TypeResults    []txTypeResult   `json:"type_results"`
	}

	finalPlasma := plasmaLevel(target.addrStr)
	out := txTypeSummary{
		AccountIndex:   uint64(idx),
		AccountAddress: target.addrStr,
		QSRFused:       qsrAmt,
		FusedPlasma:    fusedPlasma,
		InitialPlasma:  initialPlasma,
		FinalPlasma:    finalPlasma,
		TypeResults:    typeResults,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// ---- Phase 17: mempool cap + frontier conflict ----

type poolCapResult struct {
	TxIndex          int    `json:"tx_index"`
	BlockHeight      uint64 `json:"block_height"`
	SubmitMomentum   uint64 `json:"submit_momentum"`
	PlasmaBefore     uint64 `json:"plasma_before"`
	PublishError     string `json:"publish_error,omitempty"`
	Included         bool   `json:"included"`
	InclusionLatencyMs int64 `json:"inclusion_latency_ms,omitempty"`
}

type poolCapSummary struct {
	AccountIndex     uint64           `json:"account_index"`
	AccountAddress   string           `json:"account_address"`
	QSRFused         int64            `json:"qsr_fused"`
	FusedPlasma      uint64           `json:"fused_plasma"`
	Attempted        int              `json:"attempted"`
	Submitted        int              `json:"submitted"`
	Rejected         int              `json:"rejected"`
	Included         int              `json:"included"`
	Unresolved       int              `json:"unresolved"`
	FirstRejectIdx   int              `json:"first_reject_idx"`
	FirstRejectError string           `json:"first_reject_error"`
	MomentaUsed      int              `json:"momenta_used"`
	StartMomentum    uint64           `json:"start_momentum"`
	EndMomentum      uint64           `json:"end_momentum"`
	Results          []poolCapResult  `json:"results"`
}

// runPoolCap tests the per-account mempool cap (MaxUncommittedBlocksPerAccount=500).
// Sends maxTx self-transfers as fast as possible from a single account without
// waiting for inclusion. Measures when rejects start and whether the account recovers.
func runPoolCap(idx uint32, qsrAmt int64, maxTx int) {
	agent := agentAccount()
	target := derivedAccount(idx)

	// Setup: fund + fuse
	_, targetHeight := frontierAccount(target.addrStr)
	if targetHeight == 0 {
		fmt.Fprintf(os.Stderr, "poolcap: funding account[%d] with 100 ZNN...\n", idx)
		b, _, err := buildSend(agent, target.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	// Wait for funding to arrive
	for i := 0; i < 30; i++ {
		var ul unreceivedList
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{target.addrStr, 0, 50}, &ul)
		if len(ul.List) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	receiveAll(target)

	// Fuse QSR for plasma
	fmt.Fprintf(os.Stderr, "poolcap: fusing %d QSR on account[%d]...\n", qsrAmt, idx)
	fuseData := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, target.addr)
	fb, _, err := buildSend(agent, types.PlasmaContract, qsr, dec(qsrAmt), fuseData, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL fuse: %v\n", err)
		os.Exit(1)
	}
	if err := publish(fb); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish fuse: %v\n", err)
		os.Exit(1)
	}
	waitIncluded(agent.addrStr, fb.Height, 120*time.Second)

	// Wait for plasma
	for i := 0; i < 60; i++ {
		p := plasmaLevel(target.addrStr)
		if p >= uint64(qsrAmt*2100) {
			break
		}
		time.Sleep(2 * time.Second)
	}

	initialPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "poolcap: initial plasma=%d, sending %d txs...\n", initialPlasma, maxTx)

	fmStart := frontierMomentum()
	startM := fmStart.Height

	var results []poolCapResult
	firstRejectIdx := -1
	var firstRejectError string

	// Use tiny amount (1 raw unit) so balance doesn't limit tx count.
	// Need enough plasma for maxTx txs in rapid succession:
	// each tx costs 21000 plasma, so maxTx=600 needs 12.6M plasma = 6000 QSR.
	tinyAmt := big.NewInt(1)
	for i := 0; i < maxTx; i++ {
		plasmaBefore := plasmaLevel(target.addrStr)
		b, _, err := buildSend(target, target.addr, znn, tinyAmt, nil, true)
		if err != nil {
			results = append(results, poolCapResult{
				TxIndex: i, PublishError: fmt.Sprintf("build: %v", err),
				PlasmaBefore: plasmaBefore,
			})
			if firstRejectIdx < 0 {
				firstRejectIdx = i
				firstRejectError = fmt.Sprintf("build: %v", err)
			}
			continue
		}
		subM := frontierMomentum().Height
		if err := publish(b); err != nil {
			errStr := fmt.Sprintf("%v", err)
			results = append(results, poolCapResult{
				TxIndex: i, BlockHeight: b.Height, SubmitMomentum: subM,
				PlasmaBefore: plasmaBefore, PublishError: errStr,
			})
			if firstRejectIdx < 0 {
				firstRejectIdx = i
				firstRejectError = errStr
			}
			continue
		}
		results = append(results, poolCapResult{
			TxIndex: i, BlockHeight: b.Height, SubmitMomentum: subM,
			PlasmaBefore: plasmaBefore,
		})
	}

	// Wait for inclusion — with 500+ txs this may take a while
	fmt.Fprintf(os.Stderr, "poolcap: waiting for inclusion (120s)...\n")
	time.Sleep(120 * time.Second)

	// Verify inclusion
	submitted := 0
	included := 0
	rejected := 0
	for i := range results {
		r := &results[i]
		if r.PublishError != "" {
			rejected++
			continue
		}
		submitted++
		_, h := frontierAccount(target.addrStr)
		if h >= r.BlockHeight {
			r.Included = true
			included++
		}
	}

	fmEnd := frontierMomentum()
	endM := fmEnd.Height

	out := poolCapSummary{
		AccountIndex:     uint64(idx),
		AccountAddress:   target.addrStr,
		QSRFused:         qsrAmt,
		FusedPlasma:      uint64(qsrAmt * 2100),
		Attempted:        maxTx,
		Submitted:        submitted,
		Rejected:         rejected,
		Included:         included,
		Unresolved:       submitted - included,
		FirstRejectIdx:   firstRejectIdx,
		FirstRejectError: firstRejectError,
		MomentaUsed:      int(endM - startM + 1),
		StartMomentum:    startM,
		EndMomentum:      endM,
		Results:          results,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

type frontConfResult struct {
	TxIndex      int    `json:"tx_index"`
	BlockHeight  uint64 `json:"block_height"`
	Hash         string `json:"hash"`
	DataLen      int    `json:"data_len"`
	FusedPlasma  uint64 `json:"fused_plasma"`
	BasePlasma   uint64 `json:"base_plasma"`
	OverpayMult  int    `json:"overpay_mult"`
	PublishError string `json:"publish_error,omitempty"`
	Included     bool   `json:"included"`
}

type frontConfSummary struct {
	AccountIndex     uint64            `json:"account_index"`
	AccountAddress   string            `json:"account_address"`
	QSRFused         int64             `json:"qsr_fused"`
	ConflictsTested  int               `json:"conflicts_tested"`
	EqualPlasmaRounds int              `json:"equal_plasma_rounds"`
	OverpayRounds    int               `json:"overpay_rounds"`
	EqualHashWins    int               `json:"equal_hash_wins"`
	PriceWins        int               `json:"price_wins"`
	CorrectPredictions int             `json:"correct_predictions"`
	Results          []frontConfResult `json:"results"`
}

// runFrontConf tests frontier conflict resolution in the account pool.
// For each conflict round, it builds two transactions at the same height
// with different content (different data payloads → different hashes).
// Both are published rapidly. The pool's price-based replacement rule
// should keep exactly one; the other is rejected or replaced.
//
// With equal plasma, the tie-break is smallest hash wins (pool replacement).
// With different data lengths, the one with higher price-per-base-plasma wins.
func runFrontConf(idx uint32, qsrAmt int64) {
	agent := agentAccount()
	target := derivedAccount(idx)

	// Setup: fund + fuse
	_, targetHeight := frontierAccount(target.addrStr)
	if targetHeight == 0 {
		fmt.Fprintf(os.Stderr, "frontconf: funding account[%d] with 100 ZNN...\n", idx)
		b, _, err := buildSend(agent, target.addr, znn, dec(100), nil, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL fund: %v\n", err)
			os.Exit(1)
		}
		if err := publish(b); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL publish fund: %v\n", err)
			os.Exit(1)
		}
		waitIncluded(agent.addrStr, b.Height, 120*time.Second)
	}

	for i := 0; i < 30; i++ {
		var ul unreceivedList
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{target.addrStr, 0, 50}, &ul)
		if len(ul.List) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	receiveAll(target)

	fmt.Fprintf(os.Stderr, "frontconf: fusing %d QSR on account[%d]...\n", qsrAmt, idx)
	fuseData := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, target.addr)
	fb, _, err := buildSend(agent, types.PlasmaContract, qsr, dec(qsrAmt), fuseData, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL fuse: %v\n", err)
		os.Exit(1)
	}
	if err := publish(fb); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL publish fuse: %v\n", err)
		os.Exit(1)
	}
	waitIncluded(agent.addrStr, fb.Height, 120*time.Second)

	for i := 0; i < 60; i++ {
		p := plasmaLevel(target.addrStr)
		if p >= uint64(qsrAmt*2100) {
			break
		}
		time.Sleep(2 * time.Second)
	}

	initialPlasma := plasmaLevel(target.addrStr)
	fmt.Fprintf(os.Stderr, "frontconf: initial plasma=%d, testing frontier conflicts...\n", initialPlasma)

	// Test both equal-plasma (hash tie-break) and overpay (price priority) scenarios.
	// Rounds 0-1: equal plasma (both base) — smallest hash should win
	// Rounds 2-3: txB overpays 2x — txB should win regardless of hash
	// Round 4: txA overpays 2x — txA should win regardless of hash
	type conflictRound struct {
		round      int
		overpayA   int // 1 = base, 2 = 2x overpay
		overpayB   int
		expectWinA bool // true if txA should win
	}
	rounds := []conflictRound{
		{0, 1, 1, false}, // equal plasma, smallest hash wins (determined at runtime)
		{1, 1, 1, false}, // equal plasma, smallest hash wins
		{2, 1, 2, false}, // txB overpays — txB should win
		{3, 1, 2, false}, // txB overpays — txB should win
		{4, 2, 1, true},  // txA overpays — txA should win
	}

	var allResults []frontConfResult
	equalHashWins := 0
	priceWins := 0
	correctPredictions := 0
	equalRounds := 0
	overpayRounds := 0

	for _, cr := range rounds {
		round := cr.round
		prevHash, height := frontierAccount(target.addrStr)
		fm := frontierMomentum()

		// Build txA
		dataA := []byte{byte(round)}
		bA := &nom.AccountBlock{
			Version:              1,
			ChainIdentifier:      chainID,
			BlockType:            nom.BlockTypeUserSend,
			PreviousHash:         prevHash,
			Height:               height + 1,
			MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
			Address:              target.addr,
			ToAddress:            target.addr,
			Amount:               big.NewInt(1),
			TokenStandard:        znn,
			Data:                 dataA,
		}
		reqA := requiredPoW(target.addrStr, bA.BlockType, &target.addr, dataA)
		bA.FusedPlasma = reqA.BasePlasma * uint64(cr.overpayA)
		bA.Hash = bA.ComputeHash()
		bA.PublicKey = target.kp.Public
		bA.Signature = target.kp.Sign(bA.Hash.Bytes())

		// Build txB (different data → different hash)
		dataB := make([]byte, 100)
		for i := range dataB {
			dataB[i] = byte((i + round) % 256)
		}
		bB := &nom.AccountBlock{
			Version:              1,
			ChainIdentifier:      chainID,
			BlockType:            nom.BlockTypeUserSend,
			PreviousHash:         prevHash,
			Height:               height + 1,
			MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
			Address:              target.addr,
			ToAddress:            target.addr,
			Amount:               big.NewInt(1),
			TokenStandard:        znn,
			Data:                 dataB,
		}
		reqB := requiredPoW(target.addrStr, bB.BlockType, &target.addr, dataB)
		bB.FusedPlasma = reqB.BasePlasma * uint64(cr.overpayB)
		bB.Hash = bB.ComputeHash()
		bB.PublicKey = target.kp.Public
		bB.Signature = target.kp.Sign(bB.Hash.Bytes())

		// Determine expected winner
		var expectedWinner string
		if cr.overpayA == cr.overpayB {
			// Equal price ratio — smallest hash wins
			equalRounds++
			if bA.Hash.String() < bB.Hash.String() {
				expectedWinner = "A"
			} else {
				expectedWinner = "B"
			}
		} else {
			overpayRounds++
			if cr.overpayA > cr.overpayB {
				expectedWinner = "A"
			} else {
				expectedWinner = "B"
			}
		}

		fmt.Fprintf(os.Stderr, "frontconf: round %d — txA(hash=%s... data=%dB plasma=%d overpay=%dx) vs txB(hash=%s... data=%dB plasma=%d overpay=%dx) expect=%s\n",
			round, bA.Hash.String()[:16], len(dataA), bA.FusedPlasma, cr.overpayA,
			bB.Hash.String()[:16], len(dataB), bB.FusedPlasma, cr.overpayB, expectedWinner)

		// Publish A first, then B rapidly
		errA := publish(bA)
		errB := publish(bB)

		resA := frontConfResult{
			TxIndex: round*2, BlockHeight: bA.Height,
			Hash: bA.Hash.String(), DataLen: len(dataA),
			FusedPlasma: bA.FusedPlasma, BasePlasma: reqA.BasePlasma, OverpayMult: cr.overpayA,
		}
		resB := frontConfResult{
			TxIndex: round*2 + 1, BlockHeight: bB.Height,
			Hash: bB.Hash.String(), DataLen: len(dataB),
			FusedPlasma: bB.FusedPlasma, BasePlasma: reqB.BasePlasma, OverpayMult: cr.overpayB,
		}
		if errA != nil {
			resA.PublishError = fmt.Sprintf("%v", errA)
		}
		if errB != nil {
			resB.PublishError = fmt.Sprintf("%v", errB)
		}

		// Wait for inclusion of whichever won
		time.Sleep(15 * time.Second)
		frontierHash, h := frontierAccount(target.addrStr)
		actualWinner := "?"
		if h >= bA.Height {
			if frontierHash == bA.Hash {
				resA.Included = true
				actualWinner = "A"
			} else if frontierHash == bB.Hash {
				resB.Included = true
				actualWinner = "B"
			} else {
				// Frontier advanced past this height — check the block at contested height
				var blk struct {
					Hash string `json:"hash"`
				}
				if err := rpc("ledger.getAccountBlockByHeight", []interface{}{target.addrStr, bA.Height}, &blk); err == nil && blk.Hash != "" {
					if blk.Hash == bA.Hash.String() {
						resA.Included = true
						actualWinner = "A"
					} else if blk.Hash == bB.Hash.String() {
						resB.Included = true
						actualWinner = "B"
					}
				}
			}
		}

		// Classify the win
		if actualWinner == expectedWinner {
			correctPredictions++
		}
		if cr.overpayA == cr.overpayB {
			// Equal plasma rounds — hash tie-break
			if actualWinner != "?" {
				equalHashWins++
				fmt.Fprintf(os.Stderr, "frontconf: round %d — %s won (equal plasma, hash tie-break) expected=%s correct=%v\n",
					round, actualWinner, expectedWinner, actualWinner == expectedWinner)
			}
		} else {
			// Overpay rounds — price priority
			if actualWinner != "?" {
				priceWins++
				fmt.Fprintf(os.Stderr, "frontconf: round %d — %s won (price priority) expected=%s correct=%v\n",
					round, actualWinner, expectedWinner, actualWinner == expectedWinner)
			}
		}

		if resA.PublishError != "" || resB.PublishError != "" {
			fmt.Fprintf(os.Stderr, "frontconf: round %d — publish errors: A=%q B=%q\n",
				round, resA.PublishError, resB.PublishError)
		}

		allResults = append(allResults, resA, resB)

		// Wait a moment between rounds
		time.Sleep(3 * time.Second)
	}

	out := frontConfSummary{
		AccountIndex:       uint64(idx),
		AccountAddress:     target.addrStr,
		QSRFused:           qsrAmt,
		ConflictsTested:    len(rounds),
		EqualPlasmaRounds:  equalRounds,
		OverpayRounds:      overpayRounds,
		EqualHashWins:      equalHashWins,
		PriceWins:          priceWins,
		CorrectPredictions: correctPredictions,
		Results:            allResults,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
