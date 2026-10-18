// plasma-sim — off-chain port of the TminusZ Dynamic Plasma controller (spec v0.8).
// Replays load patterns (measured on devnet chain 99) through the multiplier logic.
// Zero consensus risk: simulation only.
//
// INTERPRETATION FLAG: the spec defines AlphaUpFP/BetaDownFP and MulDiv3 but does not
// spell out the exact target formula. We implement the dimensionally consistent reading:
//   congestion: M_target = M + MulDiv3(M, AlphaUpFP, U-TargetHigh, MScale*MScale)
//   underload:  M_target = M - MulDiv3(M, BetaDownFP, TargetLow-U, MScale*MScale)
// (denominator MScale^2 — single-MScale division would jump M to max in one epoch,
// contradicting the spec's own "no violent jumps" design goal and its hostile review).
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

const (
	MScale        uint64 = 1_000_000
	MInitial      uint64 = 1_000_000
	MMin          uint64 = 500_000
	MMax          uint64 = 5_000_000
	TargetLowFP   uint64 = 450_000
	TargetHighFP  uint64 = 650_000
	AlphaUpFP     uint64 = 500_000
	BetaDownFP    uint64 = 100_000
	EMAOldFP      uint64 = 750_000
	EMATargetFP   uint64 = 250_000
	EpochLen      uint64 = 720
	SimpleBase    uint64 = 21_000
	MaxBlocks     uint64 = 100
	BaselineEpoch uint64 = MaxBlocks * SimpleBase * EpochLen // 1,512,000,000
)

func mulDiv3(a, b, c, denom uint64) uint64 {
	// NOTE: spec helper; a*b*c can reach ~1.4e18 < 2^64 under current bounds
	return (a * b * c) / denom
}

func utilizationFP(consumed uint64) uint64 {
	u := (consumed * MScale) / BaselineEpoch
	if u > MScale {
		return MScale
	}
	return u
}

func applyMultiplier(base, m uint64) uint64 {
	return (base*m + MScale - 1) / MScale
}

type controller struct {
	M uint64
}

func (c *controller) closeEpoch(consumed uint64) (uint64, uint64, uint64) {
	u := utilizationFP(consumed)
	mTarget := c.M
	denom := MScale * MScale
	if u > TargetHighFP {
		mTarget = c.M + mulDiv3(c.M, AlphaUpFP, u-TargetHighFP, denom)
	} else if u < TargetLowFP {
		reduction := mulDiv3(c.M, BetaDownFP, TargetLowFP-u, denom)
		if reduction > c.M {
			mTarget = 0
		} else {
			mTarget = c.M - reduction
		}
	}
	mEma := (c.M*EMAOldFP + mTarget*EMATargetFP) / MScale
	mNew := mEma
	if mNew < MMin {
		mNew = MMin
	}
	if mNew > MMax {
		mNew = MMax
	}
	c.M = mNew
	return u, mTarget, mNew
}

// scenario: per-momentum base-plasma consumption pattern, tiled to length.
type scenario struct {
	name    string
	pattern []uint64 // base plasma per momentum
	epochs  int
}

func run(s scenario, w *csv.Writer) {
	c := &controller{M: MInitial}
	var consumed uint64
	epoch := 0
	var mIdx uint64
	for epoch < s.epochs {
		for i := uint64(0); i < EpochLen; i++ {
			consumed += s.pattern[mIdx%uint64(len(s.pattern))]
			mIdx++
		}
		u, mTarget, mNew := c.closeEpoch(consumed)
		w.Write([]string{
			s.name,
			strconv.Itoa(epoch),
			strconv.FormatUint(consumed, 10),
			fmt.Sprintf("%.4f", float64(u)/float64(MScale)),
			fmt.Sprintf("%.4f", float64(mTarget)/float64(MScale)),
			fmt.Sprintf("%.4f", float64(mNew)/float64(MScale)),
			strconv.FormatUint(applyMultiplier(SimpleBase, mNew), 10),
		})
		consumed = 0
		epoch++
	}
}

func main() {
	sat := MaxBlocks * SimpleBase // 2,100,000 = full saturation, plain sends
	// measured mix-01 momentum: 50 plain + 16 delegate calls + 16 contract receives
	// contract receive base plasma: not directly measured; bounded 0..21,000.
	// pattern A uses 0 for receives (conservative), pattern B counts 21,000 each (upper bound).
	mixedLo := 50*SimpleBase + 16*52_500
	mixedHi := mixedLo + 16*SimpleBase

	scenarios := []scenario{
		{"idle", []uint64{0}, 120},
		{"honest-whale-saturation", []uint64{sat}, 200},
		{"oscillation-farming", []uint64{sat, sat, sat, sat, sat, sat, sat, sat, sat, sat,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 200}, // 10 momenta on/off => 1 epoch on, 1 off (720=2x360? use epoch-scale below)
		{"mixed-load-measured-lo", []uint64{mixedLo}, 200},
		{"mixed-load-measured-hi", []uint64{mixedHi}, 200},
		{"burst-then-recover", append(rep(sat, 20*int(EpochLen)), rep(0, 180*int(EpochLen))...), 200},
		{"deadband-cruise", []uint64{55 * SimpleBase * 100 / 100 * 55 / 100 * 100}, 120}, // placeholder fixed below
	}
	// fix deadband-cruise: 55% of saturation
	scenarios[6].pattern = []uint64{sat * 55 / 100}
	// fix oscillation: full epochs on/off (pattern is per-momentum; tile handles it)
	scenarios[2].pattern = append(rep(sat, int(EpochLen)), rep(0, int(EpochLen))...)

	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"scenario", "epoch", "consumed_base_plasma", "U", "M_target", "M", "required_plasma_plain_send"})
	for _, s := range scenarios {
		run(s, w)
	}
	w.Flush()
}

func rep(v uint64, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = v
	}
	return out
}
