package main

// Burst mode: measure how many full 8 KiB data blocks fit into one momentum
// on the ZVM devnet L1 (phase B). Three accounts fuse ~1,000 QSR each
// (2.1M plasma each, 6.3M total), then fire 8 blocks of 8,192 data bytes
// concurrently. Expected protocol cap: floor(4,200,000 / 578,056) = 7 blocks
// per momentum at floor price.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/Zenoglyphs-VM/zvm-core/devnet"
	"github.com/Zenoglyphs-VM/zvm-core/znn"
)

// plasmaContract is go-zenon's embedded plasma contract (common/types/address.go).
const plasmaContract = "z1qxemdeddedxplasmaxxxxxxxxxxxxxxxxsctrp"

var qsrDecimals = new(big.Int).Exp(big.NewInt(10), big.NewInt(8), nil)

type burstBlock struct {
	Account        uint32 `json:"account"`
	Size           int    `json:"size_bytes"`
	Hash           string `json:"hash"`
	Height         uint64 `json:"account_height"`
	UsedPlasma     uint64 `json:"used_plasma"`
	ConfirmingMom  uint64 `json:"confirming_momentum"`
	SendError      string `json:"send_error,omitempty"`
}

type burstSummary struct {
	Blocks            []burstBlock    `json:"blocks"`
	PerMomentumCounts map[uint64]int  `json:"per_momentum_counts"`
	FusedPlasma       map[uint32]uint64 `json:"fused_plasma_per_account"`
	Notes             string          `json:"notes"`
}

func fuseCallData(beneficiary znn.Address) []byte {
	sel := znn.Sha3([]byte("Fuse(address)"))
	return append(append([]byte{}, sel[:4]...), common.LeftPadBytes(beneficiary[:], 32)...)
}

// ensureFusedAccount funds (via faucet) and fuses up to targetQSR on devnet-mnemonic index.
func ensureFusedAccount(ctx context.Context, c *znn.Client, index uint32, targetQSR int64) (*znn.Account, uint64, error) {
	w := znn.MustDevWallet()
	kp := w.Key(index)
	acct := znn.NewAccount(kp, c, devnet.L1ChainID)

	info, err := c.AccountInfo(ctx, acct.Address())
	if err != nil {
		return nil, 0, err
	}
	if info.Balance(znn.ZNN).Sign() == 0 {
		if err := drip(*faucetFlag, acct.Address().String()); err != nil {
			return nil, 0, err
		}
		for i := 0; i < 30; i++ {
			receiveAll(ctx, acct)
			info, err = c.AccountInfo(ctx, acct.Address())
			if err == nil && info.Balance(znn.ZNN).Sign() > 0 {
				break
			}
			time.Sleep(4 * time.Second)
		}
	}
	for i := 0; i < 10; i++ {
		if receiveAll(ctx, acct) == 0 {
			break
		}
	}
	pi, err := c.PlasmaInfo(ctx, acct.Address())
	if err != nil {
		return nil, 0, err
	}
	fusedQSR := new(big.Int).Div(&pi.QsrAmount.Int, qsrDecimals).Int64()
	if want := targetQSR - fusedQSR; want > 0 {
		qsrBal := new(big.Int).Div(info.Balance(znn.QSR), qsrDecimals).Int64()
		if qsrBal < want {
			want = qsrBal
		}
		if want > 0 {
			to, err := znn.ParseAddress(plasmaContract)
			if err != nil {
				return nil, 0, err
			}
			b, err := acct.Send(ctx, to, znn.QSR, new(big.Int).Mul(big.NewInt(want), qsrDecimals), fuseCallData(acct.Address()))
			if err != nil {
				return nil, 0, fmt.Errorf("fuse send: %w", err)
			}
			if _, err := waitBlockConfirmed(ctx, c, b.Hash, 180*time.Second); err != nil {
				return nil, 0, err
			}
			// The plasma contract credits the beneficiary in a following block;
			// poll (receiving as we go) until the fusion shows up.
			deadline := time.Now().Add(90 * time.Second)
			for {
				receiveAll(ctx, acct)
				pi, err = c.PlasmaInfo(ctx, acct.Address())
				if err != nil {
					return nil, 0, err
				}
				if pi.MaxPlasma >= uint64(targetQSR)*2100 {
					break
				}
				if time.Now().After(deadline) {
					return nil, 0, fmt.Errorf("fusion not credited after 90s, maxPlasma=%d", pi.MaxPlasma)
				}
				time.Sleep(4 * time.Second)
			}
		}
	}
	return acct, pi.MaxPlasma, nil
}

var faucetFlag *string

func runBurst(ctx context.Context, c *znn.Client, indexes []uint32, blockSize, blocksPerAccount int) (*burstSummary, error) {
	sum := &burstSummary{PerMomentumCounts: map[uint64]int{}, FusedPlasma: map[uint32]uint64{}}
	type acctEntry struct {
		idx   uint32
		acct  *znn.Account
	}
	var accts []acctEntry
	for _, idx := range indexes {
		acct, maxPlasma, err := ensureFusedAccount(ctx, c, idx, 1000)
		if err != nil {
			return nil, fmt.Errorf("account %d: %w", idx, err)
		}
		sum.FusedPlasma[idx] = maxPlasma
		accts = append(accts, acctEntry{idx, acct})
		fmt.Fprintf(os.Stderr, "burst: account[%d] maxPlasma=%d\n", idx, maxPlasma)
	}

	var wg sync.WaitGroup
	results := make(chan burstBlock, len(indexes)*blocksPerAccount)
	for _, e := range accts {
		for j := 0; j < blocksPerAccount; j++ {
			wg.Add(1)
			go func(e acctEntry, j int) {
				defer wg.Done()
				data := make([]byte, blockSize)
				data[0] = byte(e.idx % 256)
				data[1] = byte(j)
				res := burstBlock{Account: e.idx, Size: blockSize}
				b, err := e.acct.Send(ctx, e.acct.Address(), znn.ZNN, nil, data)
				if err != nil {
					res.SendError = err.Error()
					results <- res
					return
				}
				res.Hash = b.Hash.String()
				res.Height = b.Height
				res.UsedPlasma = b.UsedPlasma
				mom, err := waitBlockConfirmed(ctx, c, b.Hash, 300*time.Second)
				if err != nil {
					res.SendError = "confirm: " + err.Error()
					results <- res
					return
				}
				res.ConfirmingMom = mom
				results <- res
			}(e, j)
		}
	}
	wg.Wait()
	close(results)
	for r := range results {
		sum.Blocks = append(sum.Blocks, r)
		if r.ConfirmingMom > 0 {
			sum.PerMomentumCounts[r.ConfirmingMom]++
		}
	}
	sum.Notes = fmt.Sprintf("%d accounts x %d blocks of %d data bytes; planned plasma per block = 21000+68*%d = %d; protocol budget hypothesis: floor(4200000/planned) = %d blocks/momentum at floor price",
		len(indexes), blocksPerAccount, blockSize, blockSize, 21000+68*blockSize, 4200000/(21000+68*uint64(blockSize)))
	return sum, nil
}

func writeJSON(path string, v any) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal(err)
	}
}
