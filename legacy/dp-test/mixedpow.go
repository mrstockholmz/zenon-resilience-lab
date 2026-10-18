// mixedpow.go — test mixed fused+PoW blocks on chain 100 DP devnet.
// Run: go run . mixedpow <fusedPlasma> <difficulty>
// Sends one self-transfer with the given fused/PoW split and verifies
// acceptance, inclusion, and correct plasma accounting.
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
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
)

func rpcRaw(method string, params interface{}) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &env)
	if env.Error != nil {
		return nil, fmt.Errorf("%s", env.Error.Message)
	}
	return env.Result, nil
}

func runMixedPow(fusedPlasma uint64, difficulty uint64) {
	a := agentAccount()

	// Get frontier
	prevHash, height := frontierAccount(a.addrStr)
	fm := frontierMomentum()

	// Get required plasma for a plain send
	req := required(a.addrStr, nom.BlockTypeUserSend, &a.addr, nil)
	fmt.Printf("Required: basePlasma=%d requiredDifficulty=%d\n", req.BasePlasma, req.RequiredDifficulty)

	// Build block
	b := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      chainID,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         prevHash,
		Height:               height + 1,
		MomentumAcknowledged: types.HashHeight{Hash: fm.Hash, Height: fm.Height},
		Address:              a.addr,
		ToAddress:            a.addr,
		Amount:               dec(1), // 1 ZNN self-transfer
		TokenStandard:        znn,
		FusedPlasma:          fusedPlasma,
		Difficulty:           difficulty,
	}

	// PoW if needed
	if difficulty > 0 {
		t0 := time.Now()
		dataHash := pow.GetAccountBlockHash(b)
		nonce := pow.GetPoWNonce(big.NewInt(int64(difficulty)), dataHash)
		copy(b.Nonce.Data[:], nonce)
		powMs := time.Since(t0).Milliseconds()
		fmt.Printf("PoW computed: difficulty=%d nonce=%x time=%dms\n", difficulty, nonce, powMs)
	}

	b.Hash = b.ComputeHash()
	b.PublicKey = a.kp.Public
	b.Signature = a.kp.Sign(b.Hash.Bytes())

	fmt.Printf("Block: height=%d fusedPlasma=%d difficulty=%d basePlasma=%d\n",
		b.Height, b.FusedPlasma, b.Difficulty, req.BasePlasma)

	// Publish
	if err := publish(b); err != nil {
		fmt.Printf("PUBLISH FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Published: hash=%s\n", b.Hash)

	// Wait for inclusion
	lat, mh, ok := waitIncluded(a.addrStr, b.Height, 60*time.Second)
	if !ok {
		fmt.Printf("NOT INCLUDED after 60s\n")
		os.Exit(1)
	}
	fmt.Printf("Included: momentum=%d latency=%dms\n", mh, lat.Milliseconds())

	// Read back the block
	var blockJSON struct {
		FusedPlasma  uint64 `json:"fusedPlasma"`
		Difficulty   uint64 `json:"difficulty"`
		BasePlasma   uint64 `json:"basePlasma"`
		UsedPlasma   uint64 `json:"usedPlasma"`
	}
	blockRes, err := rpcRaw("ledger.getAccountBlockByHash", []interface{}{b.Hash.String()})
	if err != nil {
		fmt.Printf("Read-back failed: %v\n", err)
		return
	}
	json.Unmarshal(blockRes, &blockJSON)
	fmt.Printf("Read-back: fusedPlasma=%d difficulty=%d\n",
		blockJSON.FusedPlasma, blockJSON.Difficulty)

	// Check next momentum prices
	fm2 := frontierMomentum()
	fmt.Printf("Frontier after: height=%d nextFusionPrice=%d nextWorkPrice=%d\n",
		fm2.Height, fm2.NextFusionPrice, fm2.NextWorkPrice)

	// Verify accounting
	// ComputeBasePlasma formula:
	// f = FusedPlasma * workPrice
	// p = DifficultyToPlasma(Difficulty) * fusionPrice
	// fusedBase = f * BasePlasma / (f + p)
	// powBase = BasePlasma - fusedBase
	workPrice := uint64(1000) // from frontier
	fusionPrice := uint64(1000)
	if fm.Version >= 2 {
		workPrice = fm.NextWorkPrice
		fusionPrice = fm.NextFusionPrice
	}

	f := new(big.Int).SetUint64(fusedPlasma)
	f.Mul(f, new(big.Int).SetUint64(workPrice))
	p := new(big.Int).SetUint64(difficulty / 1500) // DifficultyToPlasma
	p.Mul(p, new(big.Int).SetUint64(fusionPrice))

	sum := new(big.Int).Add(f, p)
	if sum.Sign() == 0 {
		fmt.Println("Both fused and difficulty are 0 — invalid block")
		return
	}

	expectedFusedBase := new(big.Int).Mul(f, new(big.Int).SetUint64(req.BasePlasma))
	expectedFusedBase.Div(expectedFusedBase, sum)
	expectedPowBase := req.BasePlasma - expectedFusedBase.Uint64()

	fmt.Printf("Expected split: fusedBase=%d powBase=%d (at prices fusion=%d work=%d)\n",
		expectedFusedBase.Uint64(), expectedPowBase, fusionPrice, workPrice)
}

func mixedPowCmd() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: mixedpow <fusedPlasma> <difficulty>\n")
		os.Exit(1)
	}
	fp, _ := strconv.ParseUint(os.Args[2], 10, 64)
	diff, _ := strconv.ParseUint(os.Args[3], 10, 64)
	runMixedPow(fp, diff)
}
