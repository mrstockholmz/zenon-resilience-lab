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

// ---- Main ----

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: labctl info|ordinary <n> <amt>|burst <n> <amt>|waitfor <height>|probe <sec>|fuse <qsr>|fund <idx> <znn>|fusefor <idx> <qsr>|matrix <case> <seed>")
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

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
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
