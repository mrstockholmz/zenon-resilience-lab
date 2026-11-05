// Command zvmplasmaprobe measures the NoM L1 plasma cost of data-bearing
// sends on a ZVM devnet (phase 25). It confirms the 21,000 + 68*bytes cost
// model and the 16 KiB per-block data cap on the live ZVM devnet L1, so the
// relayer's per-batch plasma cost can be grounded in measured numbers rather
// than only code constants.
//
// The account is devnet-mnemonic index N (public test material). If it has no
// funds, the devnet faucet is asked for a drip (ZNN + QSR + 200 QSR fusion).
//
// Usage:
//
//	go run . -l1 http://172.18.0.8:35997 -faucet http://172.18.0.3:8549 -index 400
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Zenoglyphs-VM/zvm-core/devnet"
	"github.com/Zenoglyphs-VM/zvm-core/znn"
)

type sizeResult struct {
	DataSizeBytes  int    `json:"data_size_bytes"`
	PlannedPlasma  uint64 `json:"planned_plasma"` // 21000 + 68*size at price 1000
	BasePlasma     uint64 `json:"base_plasma"`    // as reported by the block
	UsedPlasma     uint64 `json:"used_plasma"`    // base * price, as reported by the block
	FusedPlasma    uint64 `json:"fused_plasma"`   // plasma drawn from the fusion
	PowDifficulty  uint64 `json:"pow_difficulty"` // >0 means PoW covered (part of) the cost
	PlasmaDelta    uint64 `json:"plasma_delta"`   // currentPlasma drop across confirmation
	FusionPrice    uint64 `json:"fusion_price_next"` // confirming momentum's nextFusionPrice
	SendHash       string `json:"send_hash,omitempty"`
	IncludedHeight uint64 `json:"included_height,omitempty"`
	InclusionSec   float64 `json:"inclusion_sec,omitempty"`
	PublishError   string  `json:"publish_error,omitempty"`
	RejectedOverCap bool   `json:"rejected_over_cap,omitempty"`
}

type summary struct {
	Network      string       `json:"network"`
	ChainID      uint64       `json:"chain_id"`
	L1RPC        string       `json:"l1_rpc"`
	AccountIndex uint32       `json:"account_index"`
	Account      string       `json:"account"`
	FusedQSR     string       `json:"fused_qsr"`
	MaxPlasma    uint64       `json:"max_plasma"`
	FrontierMom  uint64       `json:"frontier_momentum_at_start"`
	Sizes        []sizeResult `json:"sizes"`
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
	os.Exit(1)
}

// frontierMomentumPrice returns the frontier momentum's nextFusionPrice (0 if absent).
func frontierMomentumPrice(ctx context.Context, c *znn.Client) uint64 {
	m, err := c.FrontierMomentum(ctx)
	if err != nil || m.NextFusionPrice == nil {
		return 0
	}
	return *m.NextFusionPrice
}

func drip(faucetURL, addr string) error {
	body, _ := json.Marshal(map[string]string{"address": addr})
	resp, err := http.Post(faucetURL+"/drip", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	fmt.Fprintf(os.Stderr, "faucet drip: status=%d resp=%v\n", resp.StatusCode, out)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("drip failed: %v", out)
	}
	return nil
}

// receiveAll drains unreceived blocks one by one (sequential account chain).
func receiveAll(ctx context.Context, a *znn.Account) int {
	n := 0
	for i := 0; i < 20; i++ {
		unrecv, err := a.Client.UnreceivedBlocks(ctx, a.Address(), 0, 10)
		if err != nil || len(unrecv.List) == 0 {
			return n
		}
		if _, err := a.Receive(ctx, unrecv.List[0].Hash); err != nil {
			fmt.Fprintf(os.Stderr, "receive: %v (retrying)\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
		n++
	}
	return n
}

// waitBlockConfirmed polls the block itself (by hash) until it carries a
// confirmation detail. Robust against later blocks on the same account chain.
func waitBlockConfirmed(ctx context.Context, c *znn.Client, h znn.Hash, timeout time.Duration) (uint64, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b, err := c.AccountBlockByHash(ctx, h)
		if err == nil && b != nil && b.ConfirmationDetail != nil {
			return b.ConfirmationDetail.MomentumHeight, nil
		}
		time.Sleep(2 * time.Second)
	}
	return 0, fmt.Errorf("timeout waiting for confirmation of %s", h)
}

func main() {
	l1 := flag.String("l1", "http://172.18.0.8:35997", "NoM L1 RPC URL")
	faucetURL := flag.String("faucet", "http://172.18.0.3:8549", "faucet URL")
	index := flag.Uint("index", 400, "devnet mnemonic index")
	burst := flag.Bool("burst", false, "run phase B burst measurement instead of the size ladder")
	flag.Parse()
	faucetFlag = faucetURL

	ctx := context.Background()
	c := znn.NewClient(*l1)

	if *burst {
		sum, err := runBurst(ctx, c, []uint32{uint32(*index), uint32(*index) + 1, uint32(*index) + 2}, 8192, 3)
		if err != nil {
			fatal(err)
		}
		writeJSON("burst-result.json", sum)
		out, _ := json.MarshalIndent(sum, "", "  ")
		fmt.Println(string(out))
		return
	}
	w := znn.MustDevWallet()
	kp := w.Key(uint32(*index))
	acct := znn.NewAccount(kp, c, devnet.L1ChainID)

	front, err := c.FrontierMomentum(ctx)
	if err != nil {
		fatal(err)
	}
	s := summary{
		Network:      "zvm-devnet L1 (go-zenon dev ruleset, dynamic plasma)",
		ChainID:      devnet.L1ChainID,
		L1RPC:        *l1,
		AccountIndex: uint32(*index),
		Account:      acct.Address().String(),
		FrontierMom:  front.Height,
	}
	fmt.Fprintf(os.Stderr, "L1 frontier momentum=%d account=%s\n", front.Height, s.Account)

	// --- Funding ---
	info, err := c.AccountInfo(ctx, acct.Address())
	if err != nil {
		fatal(err)
	}
	znnBal := info.Balance(znn.ZNN)
	fmt.Fprintf(os.Stderr, "ZNN balance=%s\n", znnBal)
	if znnBal.Sign() == 0 {
		if err := drip(*faucetURL, s.Account); err != nil {
			fatal(err)
		}
		// faucet sends ZNN, QSR and a 200-QSR fusion; receive until funded
		for i := 0; i < 30; i++ {
			receiveAll(ctx, acct)
			info, err = c.AccountInfo(ctx, acct.Address())
			if err == nil && info.Balance(znn.ZNN).Sign() > 0 {
				break
			}
			time.Sleep(4 * time.Second)
		}
		if info.Balance(znn.ZNN).Sign() == 0 {
			fatal(fmt.Errorf("funding never arrived"))
		}
		fmt.Fprintf(os.Stderr, "funded: ZNN=%s QSR=%s\n", info.Balance(znn.ZNN), info.Balance(znn.QSR))
	}
	// drain any remaining unreceived (incl. the fusion block)
	for i := 0; i < 10; i++ {
		if receiveAll(ctx, acct) == 0 {
			break
		}
	}

	pi, err := c.PlasmaInfo(ctx, acct.Address())
	if err != nil {
		fatal(err)
	}
	s.MaxPlasma = pi.MaxPlasma
	s.FusedQSR = pi.QsrAmount.String()
	fmt.Fprintf(os.Stderr, "plasma: current=%d max=%d fusedQSR=%s\n", pi.CurrentPlasma, pi.MaxPlasma, pi.QsrAmount)

	if pi.MaxPlasma == 0 {
		fatal(fmt.Errorf("no fused plasma; faucet fusion not credited"))
	}

	// --- Data size ladder ---
	sizes := []int{0, 1024, 4096, 8192, 16384, 16385}
	for _, size := range sizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i % 251)
		}
		res := sizeResult{
			DataSizeBytes: size,
			PlannedPlasma: uint64(21000 + 68*size),
		}
		before, err := c.PlasmaInfo(ctx, acct.Address())
		if err != nil {
			fatal(err)
		}
		fmt.Fprintf(os.Stderr, "size=%d planned=%d plasmaBefore=%d\n", size, res.PlannedPlasma, before.CurrentPlasma)

		t0 := time.Now()
		b, err := acct.Send(ctx, acct.Address(), znn.ZNN, nil, data)
		if err != nil {
			res.PublishError = err.Error()
			if size > 16384 {
				res.RejectedOverCap = true
			}
			fmt.Fprintf(os.Stderr, "size=%d publish error: %v\n", size, err)
		} else {
			res.SendHash = b.Hash.String()
			res.BasePlasma = b.BasePlasma
			res.UsedPlasma = b.UsedPlasma
			res.FusedPlasma = b.FusedPlasma
			res.PowDifficulty = b.Difficulty
			mom, err := waitBlockConfirmed(ctx, c, b.Hash, 180*time.Second)
			if err != nil {
				fatal(err)
			}
			res.IncludedHeight = mom
			res.InclusionSec = time.Since(t0).Round(time.Millisecond).Seconds()
			if m, err := c.MomentumByHeight(ctx, mom); err == nil && m.NextFusionPrice != nil {
				res.FusionPrice = *m.NextFusionPrice
			}
			after, err := c.PlasmaInfo(ctx, acct.Address())
			if err != nil {
				fatal(err)
			}
			if after.CurrentPlasma <= before.CurrentPlasma {
				res.PlasmaDelta = before.CurrentPlasma - after.CurrentPlasma
			}
			fmt.Fprintf(os.Stderr, "size=%d included mom=%d base=%d used=%d fused=%d powDiff=%d delta=%d (planned %d)\n",
				size, mom, res.BasePlasma, res.UsedPlasma, res.FusedPlasma, res.PowDifficulty, res.PlasmaDelta, res.PlannedPlasma)
		}
		s.Sizes = append(s.Sizes, res)
	}

	out, _ := json.MarshalIndent(s, "", "  ")
	fmt.Println(string(out))
}
