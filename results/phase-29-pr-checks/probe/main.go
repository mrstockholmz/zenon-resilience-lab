// phase27probe — Zenon Resilience Lab phase 27 driver.
//
// Drives the isolated next-sporks devnet (go-zenon master) over RPC:
// spork lifecycle, bridge governance for the #271 live trigger, wraps,
// stale-ack admission probes, and observation dumps. Uses only the
// disposable go-zenon devnet identities. Never touches real wallets.
//
// Env: LAB_RPC_URL (default http://127.0.0.1:48001), LAB_DEVNET_HOME
// (default ~/lab-devnet-p27).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/wallet"
)

var rpcURL = getenv("LAB_RPC_URL", "http://127.0.0.1:48001")
var devnetHome = getenv("LAB_DEVNET_HOME", os.Getenv("HOME")+"/lab-devnet-p27")

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", a...)
	os.Exit(1)
}

// ---- RPC ----

func rpc(method string, params interface{}, out interface{}) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if r.Error != nil {
		return fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

func mustRPC(method string, params interface{}, out interface{}) {
	if err := rpc(method, params, out); err != nil {
		fatalf("%s: %v", method, err)
	}
}

// ---- keys/accounts ----

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
	kf, err := wallet.ReadKeyFile(devnetHome + "/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq")
	if err != nil {
		fatalf("read keyfile: %v", err)
	}
	ks, err := kf.Decrypt("devnet")
	if err != nil {
		fatalf("decrypt: %v", err)
	}
	cachedKS = ks
	return ks
}

func derivedAccount(index uint32) account {
	_, kp, err := keystore().DeriveForIndexPath(index)
	if err != nil {
		fatalf("derive %d: %v", index, err)
	}
	return account{kp.Address, kp.Address.String(), kp, index}
}

// ---- chain helpers ----

type momentumInfo struct {
	Hash   types.Hash `json:"hash"`
	Height uint64     `json:"height"`
}

func frontierMomentum() momentumInfo {
	var fm momentumInfo
	mustRPC("ledger.getFrontierMomentum", []interface{}{}, &fm)
	return fm
}

func frontierAccount(addrStr string) (types.Hash, uint64) {
	var fb struct {
		Hash   types.Hash `json:"hash"`
		Height uint64     `json:"height"`
	}
	mustRPC("ledger.getFrontierAccountBlock", []interface{}{addrStr}, &fb)
	return fb.Hash, fb.Height
}

type requiredPlasma struct {
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
	BasePlasma         uint64 `json:"basePlasma"`
}

func requiredPoW(addrStr string, blockType uint64, to *types.Address, data []byte) requiredPlasma {
	param := map[string]interface{}{
		"address":   addrStr,
		"blockType": blockType,
		"data":      data,
	}
	if to != nil {
		param["toAddress"] = to.String()
	}
	var req requiredPlasma
	mustRPC("embedded.plasma.getRequiredPoWForAccountBlock", []interface{}{param}, &req)
	return req
}

func publish(b *nom.AccountBlock) error {
	return rpc("ledger.publishRawTransaction", []interface{}{b.ToNomMarshalJson()}, nil)
}

func waitIncluded(addrStr string, height uint64, timeout time.Duration) bool {
	start := time.Now()
	for time.Since(start) < timeout {
		_, h := frontierAccount(addrStr)
		if h >= height {
			return true
		}
		time.Sleep(400 * time.Millisecond)
	}
	return false
}

// buildSend signs a user send. If ackHeight > 0 the block acknowledges the
// momentum at that height (used for the stale-ack probe); otherwise the
// current frontier.
func buildSend(a account, to types.Address, zts types.ZenonTokenStandard, amount *big.Int, data []byte, ackHeight uint64) *nom.AccountBlock {
	prevHash, height := frontierAccount(a.addrStr)
	fm := frontierMomentum()
	ack := types.HashHeight{Hash: fm.Hash, Height: fm.Height}
	if ackHeight > 0 {
		var page struct {
			List []momentumInfo `json:"list"`
		}
		mustRPC("ledger.getMomentumsByHeight", []interface{}{ackHeight, 1}, &page)
		if len(page.List) != 1 {
			fatalf("no momentum at height %d", ackHeight)
		}
		ack = types.HashHeight{Hash: page.List[0].Hash, Height: ackHeight}
	}
	b := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      70,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         prevHash,
		Height:               height + 1,
		MomentumAcknowledged: ack,
		Address:              a.addr,
		ToAddress:            to,
		Amount:               amount,
		TokenStandard:        zts,
		Data:                 data,
	}
	req := requiredPoW(a.addrStr, b.BlockType, &to, data)
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
	return b
}

func sendAndWait(a account, to types.Address, zts types.ZenonTokenStandard, amount *big.Int, data []byte) (uint64, bool) {
	b := buildSend(a, to, zts, amount, data, 0)
	if err := publish(b); err != nil {
		fatalf("publish from %s: %v", a.addrStr, err)
	}
	ok := waitIncluded(a.addrStr, b.Height, 120*time.Second)
	return b.Height, ok
}

// receiveAll autoreceives every unreceived block for an account.
func receiveAll(a account) int {
	var ul struct {
		List []*struct {
			Hash types.Hash `json:"hash"`
		} `json:"list"`
	}
	mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{a.addrStr, 0, 50}, &ul)
	count := 0
	for _, ub := range ul.List {
		prevHash, height := frontierAccount(a.addrStr)
		fm := frontierMomentum()
		b := &nom.AccountBlock{
			Version:              1,
			ChainIdentifier:      70,
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
			fmt.Fprintf(os.Stderr, "receive publish err: %v\n", err)
			continue
		}
		if waitIncluded(a.addrStr, b.Height, 60*time.Second) {
			count++
		}
	}
	return count
}

// ---- observation ----

func dump(label string, v interface{}) {
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("=== %s ===\n%s\n", label, out)
}

type observation struct {
	Frontier momentumInfo            `json:"frontier"`
	Sporks   json.RawMessage         `json:"sporks"`
	TokenQ   json.RawMessage         `json:"tokenContractUnreceived"`
	BridgeQ  json.RawMessage         `json:"bridgeContractUnreceived"`
	SporkQ   json.RawMessage         `json:"sporkContractUnreceived"`
	Accounts map[string]json.RawMessage `json:"accountBalances"`
}

func observe(label string, addrs ...string) {
	var fm = frontierMomentum()
	var sporks, tokenQ, bridgeQ, sporkQ json.RawMessage
	mustRPC("embedded.spork.getAll", []interface{}{0, 20}, &sporks)
	mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{types.TokenContract.String(), 0, 50}, &tokenQ)
	mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{types.BridgeContract.String(), 0, 50}, &bridgeQ)
	mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{types.SporkContract.String(), 0, 50}, &sporkQ)
	accs := map[string]json.RawMessage{}
	for _, a := range addrs {
		var info json.RawMessage
		mustRPC("ledger.getAccountInfoByAddress", []interface{}{a}, &info)
		accs[a] = info
	}
	dump(label, observation{fm, sporks, tokenQ, bridgeQ, sporkQ, accs})
}

// ---- subcommands ----

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: phase27probe <cmd> [args]")
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "unjail":
		// unjail <fromIdx> <amountSmallestUnits> <pillarName>  — sends Unjail(name)
		// with exactly <amount> ZNN from derived account[fromIdx] (the pillar's
		// stake address) to the pillar contract. Prints the send inclusion result.
		fi_, _ := strconv.ParseUint(args[0], 10, 32)
		fa := derivedAccount(uint32(fi_))
		amt, _ := new(big.Int).SetString(args[1], 10)
		data := definition.ABIPillars.PackMethodPanic(definition.UnjailMethodName, args[2])
		h, ok := sendAndWait(fa, types.PillarContract, types.ZnnTokenStandard, amt, data)
		fmt.Printf("unjail send included=%v height=%d\n", ok, h)

	case "jailinfo":
		// prints every pillar's name, isJailed flag and produced-block stats
		var info json.RawMessage
		mustRPC("embedded.pillar.getAll", []interface{}{0, 10}, &info)
		dump("pillar.getAll", info)

	case "tokeninfo":
		// prints ZNN token info incl. totalSupply
		var info json.RawMessage
		mustRPC("embedded.token.getByZts", []interface{}{types.ZnnTokenStandard.String()}, &info)
		dump("embedded.token.getByZts", info)

	case "sendto":
		// sendto <fromIdx> <toAddr> <zts|ZNN|QSR> <amount>
		fi_, _ := strconv.ParseUint(args[0], 10, 32)
		fa := derivedAccount(uint32(fi_))
		ta, err := types.ParseAddress(args[1])
		if err != nil {
			fatalf("bad to addr: %v", err)
		}
		var zts types.ZenonTokenStandard
		switch args[2] {
		case "ZNN":
			zts = types.ZnnTokenStandard
		case "QSR":
			zts = types.QsrTokenStandard
		default:
			zts = types.ParseZTSPanic(args[2])
		}
		amt, _ := new(big.Int).SetString(args[3], 10)
		h, ok := sendAndWait(fa, ta, zts, amt, nil)
		fmt.Printf("send included=%v height=%d\n", ok, h)

	case "fusefor":
		// fusefor <idx> <qsrAmount> — fuse QSR for plasma on derived account[idx]
		idx, _ := strconv.ParseUint(args[0], 10, 32)
		a := derivedAccount(uint32(idx))
		amt, _ := new(big.Int).SetString(args[1], 10)
		data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, a.addr)
		h, ok := sendAndWait(a, types.PlasmaContract, types.QsrTokenStandard, amt, data)
		fmt.Printf("fuse send included=%v height=%d\n", ok, h)

	case "addrs":
		for i := uint32(0); i <= 9; i++ {
			a := derivedAccount(i)
			fmt.Printf("%d %s\n", i, a.addrStr)
		}

	case "balance":
		var info json.RawMessage
		mustRPC("ledger.getAccountInfoByAddress", []interface{}{args[0]}, &info)
		dump("balance "+args[0], info)

	case "waitfor":
		h, _ := strconv.ParseUint(args[0], 10, 64)
		for {
			fm := frontierMomentum()
			if fm.Height >= h {
				fmt.Printf("height %d reached (frontier %d)\n", h, fm.Height)
				return
			}
			time.Sleep(2 * time.Second)
		}

	case "momentum":
		h, _ := strconv.ParseUint(args[0], 10, 64)
		var moms []momentumInfo
		mustRPC("ledger.getMomentumsByHeight", []interface{}{h, 1}, &moms)
		dump("momentum", moms)

	case "observe":
		observe(args[0], args[1:]...)

	case "sporkcreate":
		a := derivedAccount(0) // the genesis SporkAddress
		data := definition.ABISpork.PackMethodPanic(definition.SporkCreateMethodName, args[0], args[1])
		h, ok := sendAndWait(a, types.SporkContract, types.ZnnTokenStandard, big.NewInt(0), data)
		if !ok {
			fatalf("sporkcreate not included")
		}
		fmt.Printf("created spork send included at account height %d\n", h)

	case "sporkactivate":
		a := derivedAccount(0)
		id := types.HexToHashPanic(args[0])
		data := definition.ABISpork.PackMethodPanic(definition.SporkActivateMethodName, id)
		h, ok := sendAndWait(a, types.SporkContract, types.ZnnTokenStandard, big.NewInt(0), data)
		if !ok {
			fatalf("sporkactivate not included")
		}
		fmt.Printf("activate spork send included at account height %d\n", h)

	case "sporkid":
		// prints the id (create-send hash) of the most recent spork with this name.
		// waits for the embedded receive, like sporkwait.
		for {
			var sp struct {
				List []*struct {
					Id   types.Hash `json:"id"`
					Name string     `json:"name"`
				} `json:"list"`
			}
			mustRPC("embedded.spork.getAll", []interface{}{0, 20}, &sp)
			for i := len(sp.List) - 1; i >= 0; i-- {
				if sp.List[i].Name == args[0] {
					fmt.Println(sp.List[i].Id.String())
					return
				}
			}
			time.Sleep(5 * time.Second)
		}

	case "send":
		// send <fromIdx> <toIdx> <zts|ZNN|QSR> <amount>
		fi_, _ := strconv.ParseUint(args[0], 10, 32)
		ti_, _ := strconv.ParseUint(args[1], 10, 32)
		fa := derivedAccount(uint32(fi_))
		ta := derivedAccount(uint32(ti_))
		var zts types.ZenonTokenStandard
		switch args[2] {
		case "ZNN":
			zts = types.ZnnTokenStandard
		case "QSR":
			zts = types.QsrTokenStandard
		default:
			zts = types.ParseZTSPanic(args[2])
		}
		amt, _ := new(big.Int).SetString(args[3], 10)
		h, ok := sendAndWait(fa, ta.addr, zts, amt, nil)
		fmt.Printf("send included=%v height=%d\n", ok, h)

	case "recvall":
		idx, _ := strconv.ParseUint(args[0], 10, 32)
		a := derivedAccount(uint32(idx))
		fmt.Printf("received %d blocks\n", receiveAll(a))

	case "refundstate":
		// compact state snapshot for the embedded-refund checks.
		// args: zts [canarySymbol]
		zts := types.ParseZTSPanic(args[0])
		fm := frontierMomentum()
		var sp struct {
			List []*struct {
				Name              string `json:"name"`
				Activated         bool   `json:"activated"`
				EnforcementHeight uint64 `json:"enforcementHeight"`
			} `json:"list"`
		}
		mustRPC("embedded.spork.getAll", []interface{}{0, 20}, &sp)
		sporks := []interface{}{}
		for _, s := range sp.List {
			sporks = append(sporks, map[string]interface{}{"name": s.Name, "activated": s.Activated, "enforcementHeight": s.EnforcementHeight})
		}
		var tu struct {
			List []*struct {
				Hash          types.Hash          `json:"hash"`
				Address       types.Address       `json:"address"`
				FromAddress   types.Address       `json:"fromAddress"`
				TokenStandard types.ZenonTokenStandard `json:"tokenStandard"`
				Amount        string              `json:"amount"`
				BlockType     uint64              `json:"blockType"`
			} `json:"list"`
		}
		mustRPC("ledger.getUnreceivedBlocksByAddress", []interface{}{types.TokenContract.String(), 0, 50}, &tu)
		tokenQ := []interface{}{}
		for _, b := range tu.List {
			tokenQ = append(tokenQ, map[string]interface{}{
				"hash": b.Hash.String(), "from": b.FromAddress.String(),
				"addr": b.Address.String(),
				"zts": b.TokenStandard.String(), "amount": b.Amount, "blockType": b.BlockType})
		}
		balOf := func(addr types.Address, token types.ZenonTokenStandard) string {
			var info struct {
				BalanceInfoMap map[string]*struct {
					Balance string `json:"balance"`
				} `json:"balanceInfoMap"`
			}
			if err := rpc("ledger.getAccountInfoByAddress", []interface{}{addr.String()}, &info); err != nil {
				return "err"
			}
			bi, ok := info.BalanceInfoMap[token.String()]
			if !ok {
				return "0"
			}
			return bi.Balance
		}
		supply := "0"
		var tk struct {
			TotalSupply string `json:"totalSupply"`
		}
		if err := rpc("embedded.token.getByZts", []interface{}{zts.String()}, &tk); err == nil {
			supply = tk.TotalSupply
		}
		canary := false
		if len(args) > 1 {
			var cl struct {
				List []*struct {
					Symbol string `json:"symbol"`
				} `json:"list"`
			}
			if err := rpc("embedded.token.getAll", []interface{}{0, 100}, &cl); err == nil {
				for _, t := range cl.List {
					if t.Symbol == args[1] {
						canary = true
					}
				}
			}
		}
		dump("refundstate", map[string]interface{}{
			"frontierHeight":    fm.Height,
			"sporks":            sporks,
			"tokenContractQueue": tokenQ,
			"bridgeZtsBalance":  balOf(types.BridgeContract, zts),
			"tokenContractZtsBalance": balOf(types.TokenContract, zts),
			"ztsTotalSupply":    supply,
			"canaryTokenExists": canary,
		})

	case "sporkwait":
		// polls until the named spork is activated; prints enforcementHeight
		for {
			var sp struct {
				List []*struct {
					Id   types.Hash `json:"id"`
					Name string     `json:"name"`
					Activated bool   `json:"activated"`
					EnforcementHeight uint64 `json:"enforcementHeight"`
				} `json:"list"`
			}
			mustRPC("embedded.spork.getAll", []interface{}{0, 20}, &sp)
			for _, s := range sp.List {
				if s.Name == args[0] && s.Activated {
					fmt.Printf("%d\n", s.EnforcementHeight)
					return
				}
			}
			time.Sleep(5 * time.Second)
		}

	case "fuse":
		// fuse QSR for plasma: fuse <idx> <qsrRaw> [beneficiaryIdx]
		idx, _ := strconv.ParseUint(args[0], 10, 32)
		a := derivedAccount(uint32(idx))
		amt, _ := new(big.Int).SetString(args[1], 10)
		beneficiary := a.addr
		if len(args) > 2 {
			bi, _ := strconv.ParseUint(args[2], 10, 32)
			beneficiary = derivedAccount(uint32(bi)).addr
		}
		data := definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, beneficiary)
		h, ok := sendAndWait(a, types.PlasmaContract, types.QsrTokenStandard, amt, data)
		fmt.Printf("fuse included=%v height=%d\n", ok, h)

	case "bridge-guardians":
		// nominate derived accounts 4..9 as guardians (6 >= MinGuardians 5)
		a := derivedAccount(0)
		guards := make([]types.Address, 0, 6)
		for i := uint32(4); i <= 9; i++ {
			guards = append(guards, derivedAccount(i).addr)
		}
		data := definition.ABIBridge.PackMethodPanic(definition.NominateGuardiansMethodName, guards)
		h, ok := sendAndWait(a, types.BridgeContract, types.ZnnTokenStandard, big.NewInt(0), data)
		fmt.Printf("bridge-guardians included=%v height=%d\n", ok, h)

	case "bridge-orch":
		a := derivedAccount(0)
		window, _ := strconv.ParseUint(args[0], 10, 32)
		keygen, _ := strconv.ParseUint(args[1], 10, 32)
		confirm, _ := strconv.ParseUint(args[2], 10, 32)
		est, _ := strconv.ParseUint(args[3], 10, 32)
		data := definition.ABIBridge.PackMethodPanic(definition.SetOrchestratorInfoMethodName,
			uint64(window), uint32(keygen), uint32(confirm), uint32(est))
		h, ok := sendAndWait(a, types.BridgeContract, types.ZnnTokenStandard, big.NewInt(0), data)
		fmt.Printf("bridge-orch included=%v height=%d\n", ok, h)

	case "bridge-tss":
		a := derivedAccount(0)
		data := definition.ABIBridge.PackMethodPanic(definition.ChangeTssECDSAPubKeyMethodName, args[0], "", "")
		h, ok := sendAndWait(a, types.BridgeContract, types.ZnnTokenStandard, big.NewInt(0), data)
		fmt.Printf("bridge-tss included=%v height=%d\n", ok, h)

	case "bridge-network":
		a := derivedAccount(0)
		class, _ := strconv.ParseUint(args[0], 10, 32)
		chainIDArg, _ := strconv.ParseUint(args[1], 10, 32)
		data := definition.ABIBridge.PackMethodPanic(definition.SetNetworkMethodName,
			uint32(class), uint32(chainIDArg), args[2], args[3], args[4])
		h, ok := sendAndWait(a, types.BridgeContract, types.ZnnTokenStandard, big.NewInt(0), data)
		fmt.Printf("bridge-network included=%v height=%d\n", ok, h)

	case "bridge-pair":
		a := derivedAccount(0)
		class, _ := strconv.ParseUint(args[0], 10, 32)
		chainIDArg, _ := strconv.ParseUint(args[1], 10, 32)
		minAmount, _ := new(big.Int).SetString(args[7], 10)
		fee, _ := strconv.ParseUint(args[8], 10, 32)
		redeemDelay, _ := strconv.ParseUint(args[9], 10, 32)
		bridgeable := args[4] == "true"
		redeemable := args[5] == "true"
		owned := args[6] == "true"
		data := definition.ABIBridge.PackMethodPanic(definition.SetTokenPairMethod,
			uint32(class), uint32(chainIDArg),
			types.ParseZTSPanic(args[2]), args[3],
			bridgeable, redeemable, owned, minAmount, uint32(fee), uint32(redeemDelay), args[10])
		h, ok := sendAndWait(a, types.BridgeContract, types.ZnnTokenStandard, big.NewInt(0), data)
		fmt.Printf("bridge-pair included=%v height=%d\n", ok, h)

	case "issue":
		idx, _ := strconv.ParseUint(args[0], 10, 32)
		a := derivedAccount(uint32(idx))
		total, _ := new(big.Int).SetString(args[3], 10)
		max, _ := new(big.Int).SetString(args[4], 10)
		dec, _ := strconv.ParseUint(args[5], 10, 8)
		mintable := args[6] == "true"
		burnable := args[7] == "true"
		data := definition.ABIToken.PackMethodPanic(definition.IssueMethodName,
			args[1], args[2], "", total, max, uint8(dec), mintable, burnable, false)
		h, ok := sendAndWait(a, types.TokenContract, types.ZnnTokenStandard, constants.TokenIssueAmount, data)
		fmt.Printf("issue included=%v height=%d\n", ok, h)
		if !ok {
			return
		}
		n := receiveAll(a)
		fmt.Printf("received %d blocks\n", n)
		var tl struct {
			List []*struct {
				ZTS types.ZenonTokenStandard `json:"tokenStandard"`
			} `json:"list"`
		}
		mustRPC("embedded.token.getByOwner", []interface{}{a.addrStr, 0, 10}, &tl)
		if len(tl.List) > 0 {
			fmt.Printf("lastOwnedZts %s\n", tl.List[len(tl.List)-1].ZTS.String())
		}

	case "wrap":
		idx, _ := strconv.ParseUint(args[0], 10, 32)
		a := derivedAccount(uint32(idx))
		amount, _ := new(big.Int).SetString(args[2], 10)
		class, _ := strconv.ParseUint(args[3], 10, 32)
		chainIDArg, _ := strconv.ParseUint(args[4], 10, 32)
		data := definition.ABIBridge.PackMethodPanic(definition.WrapTokenMethodName,
			uint32(class), uint32(chainIDArg), args[5])
		h, ok := sendAndWait(a, types.BridgeContract,
			types.ParseZTSPanic(args[1]), amount, data)
		fmt.Printf("wrap included=%v height=%d\n", ok, h)

	case "staleack":
		idx, _ := strconv.ParseUint(args[0], 10, 32)
		toIdx, _ := strconv.ParseUint(args[1], 10, 32)
		mode := args[2] // "stale" or "fresh"
		label := args[3]
		var ackHeight uint64
		if mode == "stale" {
			ackHeight, _ = strconv.ParseUint(args[4], 10, 64)
		}
		a := derivedAccount(uint32(idx))
		to := derivedAccount(uint32(toIdx))
		fmAtBuild := frontierMomentum()
		b := buildSend(a, to.addr, types.ZnnTokenStandard, big.NewInt(1), nil, ackHeight)
		err := publish(b)
		res := map[string]interface{}{
			"label":          label,
			"mode":           mode,
			"ackHeight":      b.MomentumAcknowledged.Height,
			"ackHash":        b.MomentumAcknowledged.Hash.String(),
			"frontierAtBuild": fmAtBuild.Height,
			"blockHash":      b.Hash.String(),
			"blockHeight":    b.Height,
			"publishError":   nil,
		}
		if err != nil {
			res["publishError"] = err.Error()
		} else {
			// control must be included; stale must not be
			included := waitIncluded(a.addrStr, b.Height, 45*time.Second)
			res["included"] = included
		}
		dump(label, res)

	default:
		fatalf("unknown cmd %q", cmd)
	}
}
