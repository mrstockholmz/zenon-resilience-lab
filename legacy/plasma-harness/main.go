// plasma-harness — load/measurement tool for the mrstockholmez devnet (chain 99).
// Commands:
//   info                              balance/plasma/frontier of agent address
//   fuse <qsr>                        fuse QSR for plasma (beneficiary: agent)
//   burst <n> <znn>                   sequential self-transfers from agent
//   workers <n>                       print derived worker addresses (indices 1..n)
//   fund <n> <znn> <fuse-qsr>         send ZNN + fuse QSR (beneficiary=worker) for workers 1..n
//   hammer <n> <seconds>              n workers send self-transfers in parallel for <seconds>
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
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

var (
	rpcURL   = "http://127.0.0.1:45997"
	agentStr = os.Getenv("LAB_ADDR")
	keyfile  = os.Getenv("LAB_KEYFILE")
	passFile = os.Getenv("LAB_PASSFILE")
	chainID  = 99
)

var (
	agent = types.ParseAddressPanic(agentStr)
	znn   = types.ZnnTokenStandard
	qsr   = types.QsrTokenStandard
)

func rpc(method string, params interface{}, out interface{}) error {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("rpc %s: bad response: %s", method, body)
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

func must(method string, params interface{}, out interface{}) {
	if err := rpc(method, params, out); err != nil {
		panic(err)
	}
}

type frontierBlock struct {
	Hash   types.Hash `json:"hash"`
	Height uint64     `json:"height"`
}

type momentumInfo struct {
	Hash      types.Hash `json:"hash"`
	Height    uint64     `json:"height"`
	Timestamp uint64     `json:"timestamp"`
}

func frontierAccount(addr string) (types.Hash, uint64) {
	var fb *frontierBlock
	must("ledger.getFrontierAccountBlock", []string{addr}, &fb)
	if fb == nil {
		return types.ZeroHash, 0
	}
	return fb.Hash, fb.Height
}

func frontierMomentum() momentumInfo {
	var m momentumInfo
	must("ledger.getFrontierMomentum", []interface{}{}, &m)
	return m
}

type requiredPlasma struct {
	AvailablePlasma    uint64 `json:"availablePlasma"`
	BasePlasma         uint64 `json:"basePlasma"`
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
}

func required(addrStr string, blockType uint64, to *types.Address, data []byte) requiredPlasma {
	param := map[string]interface{}{"address": addrStr, "blockType": blockType}
	if to != nil {
		param["toAddress"] = to.String()
	}
	if data != nil {
		param["data"] = data
	}
	var r requiredPlasma
	must("embedded.plasma.getRequiredPoWForAccountBlock", []interface{}{param}, &r)
	return r
}

func plasmaNow(addrStr string) uint64 {
	var info struct {
		CurrentPlasma uint64 `json:"currentPlasma"`
	}
	must("embedded.plasma.get", []string{addrStr}, &info)
	return info.CurrentPlasma
}

// account is a signing context for one address derived from the keystore.
type account struct {
	addr    types.Address
	addrStr string
	kp      *wallet.KeyPair
}

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
	req := required(a.addrStr, b.BlockType, &to, data)
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

type unreceivedList struct {
	List []*struct {
		Hash types.Hash `json:"hash"`
	} `json:"list"`
}

func receiveAll(a account) int {
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
			fmt.Fprintf(os.Stderr, "%s starved for receive, waiting 20s...\n", a.addrStr[:14])
			time.Sleep(20 * time.Second)
			continue
		}
		b.FusedPlasma = req.BasePlasma
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

var cachedKS *wallet.KeyStore

func keystore() *wallet.KeyStore {
	if cachedKS != nil {
		return cachedKS
	}
	passBytes, err := os.ReadFile(passFile)
	if err != nil {
		panic(err)
	}
	kf, err := wallet.ReadKeyFile(keyfile)
	if err != nil {
		panic(err)
	}
	ks, err := kf.Decrypt(strings.TrimSpace(string(passBytes)))
	if err != nil {
		panic(err)
	}
	cachedKS = ks
	return ks
}

func agentAccount() account {
	kp, _, err := keystore().FindAddress(agent)
	if err != nil {
		panic(err)
	}
	return account{agent, agentStr, kp}
}

func workerAccount(index uint32) account {
	_, kp, err := keystore().DeriveForIndexPath(index)
	if err != nil {
		panic(err)
	}
	return account{kp.Address, kp.Address.String(), kp}
}

func dec(x int64) *big.Int { return big.NewInt(x * 100_000_000) }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: harness info|fuse <qsr>|burst <n> <znn>|workers <n>|fund <n> <znn> <fuseqsr>|hammer <n> <sec>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "info":
		a := agentAccount()
		fmt.Fprintf(os.Stderr, "plasma available: %d\n", plasmaNow(a.addrStr))
		_, h := frontierAccount(a.addrStr)
		fm := frontierMomentum()
		fmt.Fprintf(os.Stderr, "account height: %d, momentum height: %d\n", h, fm.Height)

	case "fuse":
		amt, _ := strconv.ParseInt(os.Args[2], 10, 64)
		a := agentAccount()
		data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, agent)
		b, powMs, err := buildSend(a, types.PlasmaContract, qsr, dec(amt), data, true)
		if err != nil {
			panic(err)
		}
		if err := publish(b); err != nil {
			panic(err)
		}
		lat, mh, ok := waitIncluded(a.addrStr, b.Height, 120*time.Second)
		fmt.Fprintf(os.Stderr, "fuse %d QSR (height %d, pow %d ms), included=%v latency=%v momentum=%d plasma=%d\n",
			amt, b.Height, powMs, ok, lat, mh, plasmaNow(a.addrStr))

	case "burst":
		n, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		a := agentAccount()
		fmt.Fprintln(os.Stdout, "idx,block_height,submit_momentum,pow_ms,included,latency_ms,included_momentum,plasma_after")
		for i := 0; i < n; i++ {
			b, powMs, err := buildSend(a, a.addr, znn, dec(amt), nil, true)
			if err != nil {
				panic(err)
			}
			submitM := frontierMomentum().Height
			if err := publish(b); err != nil {
				panic(err)
			}
			lat, mh, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
			fmt.Fprintf(os.Stdout, "%d,%d,%d,%d,%v,%d,%d,%d\n",
				i, b.Height, submitM, powMs, ok, lat.Milliseconds(), mh, plasmaNow(a.addrStr))
			if !ok {
				break
			}
		}

	case "receive":
		n, _ := strconv.Atoi(os.Args[2])
		for i := 1; i <= n; i++ {
			w := workerAccount(uint32(i))
			c := receiveAll(w)
			fmt.Fprintf(os.Stderr, "worker %d (%s): received %d blocks\n", i, w.addrStr[:14], c)
		}

	case "give":
		idx, _ := strconv.Atoi(os.Args[2])
		amt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		a := agentAccount()
		w := workerAccount(uint32(idx))
		for {
			b, _, err := buildSend(a, w.addr, znn, dec(amt), nil, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agent starved, waiting 30s...\n")
				time.Sleep(30 * time.Second)
				continue
			}
			if err := publish(b); err != nil {
				panic(err)
			}
			_, _, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
			fmt.Fprintf(os.Stderr, "gave worker %d %d ZNN ok=%v\n", idx, amt, ok)
			break
		}

	case "fusefor":
		n, _ := strconv.Atoi(os.Args[2])
		fuseAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		a := agentAccount()
		for i := 1; i <= n; i++ {
			w := workerAccount(uint32(i))
			for {
				data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, w.addr)
				b, _, err := buildSend(a, types.PlasmaContract, qsr, dec(fuseAmt), data, false)
				if err != nil {
					fmt.Fprintf(os.Stderr, "agent starved fusing worker %d, waiting 30s...\n", i)
					time.Sleep(30 * time.Second)
					continue
				}
				if err := publish(b); err != nil {
					panic(err)
				}
				_, _, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
				fmt.Fprintf(os.Stderr, "worker %d fused +%d QSR ok=%v\n", i, fuseAmt, ok)
				break
			}
		}

	case "grief":
		whaleIdx, _ := strconv.Atoi(os.Args[2])
		numSmalls, _ := strconv.Atoi(os.Args[3])
		firstSmall, _ := strconv.Atoi(os.Args[4])
		secs, _ := strconv.Atoi(os.Args[5])
		deadline := time.Now().Add(time.Duration(secs) * time.Second)
		fmt.Fprintln(os.Stdout, "worker,idx,block_height,submit_momentum,starve_events,included,latency_ms,included_momentum,plasma_after")
		var wg sync.WaitGroup
		var mu sync.Mutex
		runner := func(idx int, isWhale bool) {
			defer wg.Done()
			w := workerAccount(uint32(idx))
			if c := receiveAll(w); c > 0 {
				fmt.Fprintf(os.Stderr, "worker %d auto-received %d pending\n", idx, c)
			}
			txIdx, starves := 0, 0
			for time.Now().Before(deadline) {
				b, _, err := buildSend(w, w.addr, znn, dec(1), nil, false)
				if err != nil {
					starves++
					time.Sleep(2 * time.Second)
					continue
				}
				submitM := frontierMomentum().Height
				if err := publish(b); err != nil {
					mu.Lock()
					fmt.Fprintf(os.Stdout, "%d,%d,%d,%d,%d,publish_err:%v,0,0,0\n", idx, txIdx, b.Height, submitM, starves, err)
					mu.Unlock()
					txIdx++
					time.Sleep(300 * time.Millisecond)
					continue
				}
				lat, mh, ok := waitIncluded(w.addrStr, b.Height, 30*time.Second)
				mu.Lock()
				fmt.Fprintf(os.Stdout, "%d,%d,%d,%d,%d,%v,%d,%d,%d\n",
					idx, txIdx, b.Height, submitM, starves, ok, lat.Milliseconds(), mh, plasmaNow(w.addrStr))
				mu.Unlock()
				txIdx++
				if !ok {
					time.Sleep(2 * time.Second)
				}
				if !isWhale && txIdx%5 == 0 {
					receiveAll(w)
				}
			}
		}
		wg.Add(1)
		go runner(whaleIdx, true)
		for i := firstSmall; i < firstSmall+numSmalls; i++ {
			wg.Add(1)
			go runner(i, false)
		}
		wg.Wait()
		fmt.Fprintln(os.Stderr, "grief complete")

	case "mix":
		// mix <nPlain> <firstPlain> <nContract> <firstContract> <secs>
		nPlain, _ := strconv.Atoi(os.Args[2])
		firstPlain, _ := strconv.Atoi(os.Args[3])
		nContract, _ := strconv.Atoi(os.Args[4])
		firstContract, _ := strconv.Atoi(os.Args[5])
		secs, _ := strconv.Atoi(os.Args[6])
		deadline := time.Now().Add(time.Duration(secs) * time.Second)
		fmt.Fprintln(os.Stdout, "class,worker,idx,block_height,submit_momentum,base_plasma,included,latency_ms,included_momentum,plasma_after")
		var wg sync.WaitGroup
		var mu sync.Mutex
		runner := func(idx int, class string) {
			defer wg.Done()
			w := workerAccount(uint32(idx))
			receiveAll(w)
			txIdx := 0
			for time.Now().Before(deadline) {
				var b *nom.AccountBlock
				var err error
				if class == "contract" {
					data := definition.ABIPillars.PackMethodPanic(definition.DelegateMethodName, "mrstockholmez-dev")
					b, _, err = buildSend(w, types.PillarContract, types.ZeroTokenStandard, big.NewInt(0), data, false)
				} else {
					b, _, err = buildSend(w, w.addr, znn, dec(1), nil, false)
				}
				if err != nil {
					time.Sleep(2 * time.Second)
					continue
				}
				submitM := frontierMomentum().Height
				if perr := publish(b); perr != nil {
					mu.Lock()
					fmt.Fprintf(os.Stdout, "%s,%d,%d,%d,%d,%d,publish_err:%v,0,0,0\n", class, idx, txIdx, b.Height, submitM, b.FusedPlasma, perr)
					mu.Unlock()
					txIdx++
					time.Sleep(300 * time.Millisecond)
					continue
				}
				lat, mh, ok := waitIncluded(w.addrStr, b.Height, 30*time.Second)
				mu.Lock()
				fmt.Fprintf(os.Stdout, "%s,%d,%d,%d,%d,%d,%v,%d,%d,%d\n",
					class, idx, txIdx, b.Height, submitM, b.FusedPlasma, ok, lat.Milliseconds(), mh, plasmaNow(w.addrStr))
				mu.Unlock()
				txIdx++
				if !ok {
					time.Sleep(2 * time.Second)
				}
				if class == "plain" && txIdx%5 == 0 {
					receiveAll(w)
				}
			}
		}
		for i := firstPlain; i < firstPlain+nPlain; i++ {
			wg.Add(1)
			go runner(i, "plain")
		}
		for i := firstContract; i < firstContract+nContract; i++ {
			wg.Add(1)
			go runner(i, "contract")
		}
		wg.Wait()
		fmt.Fprintln(os.Stderr, "mix complete")

	case "workers":
		n, _ := strconv.Atoi(os.Args[2])
		for i := 1; i <= n; i++ {
			w := workerAccount(uint32(i))
			fmt.Fprintf(os.Stdout, "%d,%s\n", i, w.addrStr)
		}

	case "fund":
		n, _ := strconv.Atoi(os.Args[2])
		znnAmt, _ := strconv.ParseInt(os.Args[3], 10, 64)
		fuseAmt, _ := strconv.ParseInt(os.Args[4], 10, 64)
		start := 1
		if len(os.Args) > 5 {
			start, _ = strconv.Atoi(os.Args[5])
		}
		a := agentAccount()
		for i := start; i <= n; i++ {
			w := workerAccount(uint32(i))
			// send ZNN
			for {
				b, _, err := buildSend(a, w.addr, znn, dec(znnAmt), nil, false)
				if err != nil {
					fmt.Fprintf(os.Stderr, "agent plasma starved funding worker %d, waiting 30s for recharge...\n", i)
					time.Sleep(30 * time.Second)
					continue
				}
				if err := publish(b); err != nil {
					panic(err)
				}
				lat, _, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
				fmt.Fprintf(os.Stderr, "worker %d (%s) funded %d ZNN included=%v lat=%v\n", i, w.addrStr[:14], znnAmt, ok, lat)
				break
			}
			// fuse QSR with beneficiary = worker
			for {
				data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, w.addr)
				b, _, err := buildSend(a, types.PlasmaContract, qsr, dec(fuseAmt), data, false)
				if err != nil {
					fmt.Fprintf(os.Stderr, "agent plasma starved fusing worker %d, waiting 30s for recharge...\n", i)
					time.Sleep(30 * time.Second)
					continue
				}
				if err := publish(b); err != nil {
					panic(err)
				}
				lat, _, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
				fmt.Fprintf(os.Stderr, "worker %d fused %d QSR included=%v lat=%v (worker plasma now %d)\n",
					i, fuseAmt, ok, lat, plasmaNow(w.addrStr))
				break
			}
		}
		fmt.Fprintln(os.Stderr, "funding complete")

	case "hammer":
		n, _ := strconv.Atoi(os.Args[2])
		secs, _ := strconv.Atoi(os.Args[3])
		deadline := time.Now().Add(time.Duration(secs) * time.Second)
		fmt.Fprintln(os.Stdout, "worker,idx,block_height,submit_momentum,starve_events,included,latency_ms,included_momentum,plasma_after")
		var wg sync.WaitGroup
		var mu sync.Mutex
		for i := 1; i <= n; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				w := workerAccount(uint32(idx))
				if c := receiveAll(w); c > 0 {
					fmt.Fprintf(os.Stderr, "worker %d auto-received %d pending blocks\n", idx, c)
				}
				txIdx, starves := 0, 0
				for time.Now().Before(deadline) {
					b, _, err := buildSend(w, w.addr, znn, dec(1), nil, false)
					if err != nil {
						starves++
						time.Sleep(5 * time.Second)
						continue
					}
					submitM := frontierMomentum().Height
					if err := publish(b); err != nil {
						mu.Lock()
						fmt.Fprintf(os.Stdout, "%d,%d,%d,%d,%d,publish_err:%v,0,0,0\n", idx, txIdx, b.Height, submitM, starves, err)
						mu.Unlock()
						txIdx++
						time.Sleep(500 * time.Millisecond)
						continue
					}
					lat, mh, ok := waitIncluded(w.addrStr, b.Height, 30*time.Second)
					mu.Lock()
					fmt.Fprintf(os.Stdout, "%d,%d,%d,%d,%d,%v,%d,%d,%d\n",
						idx, txIdx, b.Height, submitM, starves, ok, lat.Milliseconds(), mh, plasmaNow(w.addrStr))
					mu.Unlock()
					txIdx++
					if !ok {
						time.Sleep(2 * time.Second)
					}
					if txIdx%10 == 0 {
						receiveAll(w)
					}
				}
			}(i)
		}
		wg.Wait()
		fmt.Fprintln(os.Stderr, "hammer complete")
	}
}
