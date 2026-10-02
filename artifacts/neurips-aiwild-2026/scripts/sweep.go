// Parameterized Monte Carlo harness for the NeurIPS AIWILD short paper.
// Preserves the reference control-law ordering from cmd/simulator, but
// exposes malicious rate, fan-out, ablation flags, and adaptive load.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
)

type Mode string

const (
	ModeBaseline    Mode = "baseline"
	ModeFull        Mode = "full"
	ModeNoCVP       Mode = "no_cvp"
	ModeNoBreaker   Mode = "no_breaker"
	ModeNoBurst     Mode = "no_burst"
	ModeBreakerOnly Mode = "breaker_only"
	ModeCVPOnly     Mode = "cvp_only"
)

type AgentNode struct {
	ID          int
	IsMalicious bool
	IsDead      bool
	Load        float64
	CVP         float64
}

type Config struct {
	Mode          Mode
	NetworkSize   int
	MaliciousRate float64
	Epochs        int
	Runs          int
	FanOut        int
	BenignLoad    float64
	MaliciousLoad float64
	CVPFloor      float64
	EntropyLimit  float64
	BurstThresh   float64
	SeedBase      int64
}

func (c Config) step(nodes []*AgentNode, rng *rand.Rand) (alive int, avgLoad float64) {
	aliveNodes := make([]*AgentNode, 0, len(nodes))
	for _, n := range nodes {
		if !n.IsDead {
			n.Load *= 0.5
			aliveNodes = append(aliveNodes, n)
		}
	}
	if len(aliveNodes) == 0 {
		return 0, 0
	}

	useCVP := c.Mode == ModeFull || c.Mode == ModeNoBreaker || c.Mode == ModeNoBurst || c.Mode == ModeCVPOnly
	useBreaker := c.Mode == ModeFull || c.Mode == ModeNoCVP || c.Mode == ModeNoBurst || c.Mode == ModeBreakerOnly
	useBurst := c.Mode == ModeFull || c.Mode == ModeNoCVP || c.Mode == ModeNoBreaker

	for _, src := range aliveNodes {
		for i := 0; i < c.FanOut; i++ {
			target := aliveNodes[rng.Intn(len(aliveNodes))]
			if src.ID == target.ID {
				continue
			}
			requestLoad := c.BenignLoad
			if src.IsMalicious {
				requestLoad = c.MaliciousLoad
			}

			if c.Mode == ModeBaseline {
				target.Load += requestLoad
				continue
			}

			if useCVP && target.CVP < c.CVPFloor {
				continue
			}
			if useBreaker && target.Load+requestLoad > c.EntropyLimit {
				if src.IsMalicious {
					src.CVP -= 0.5
				} else {
					src.CVP -= 0.1
				}
				continue
			}
			if useBurst && requestLoad > c.BurstThresh {
				src.CVP -= 0.3
			}
			target.Load += requestLoad
		}
	}

	totalLoad := 0.0
	for _, n := range nodes {
		if n.IsDead {
			continue
		}
		if n.Load >= 1.0 {
			n.IsDead = true
			continue
		}
		alive++
		totalLoad += n.Load
	}
	if alive > 0 {
		avgLoad = totalLoad / float64(alive)
	}
	return alive, avgLoad
}

func newNodes(c Config, rng *rand.Rand) []*AgentNode {
	nodes := make([]*AgentNode, c.NetworkSize)
	for i := 0; i < c.NetworkSize; i++ {
		nodes[i] = &AgentNode{
			ID:          i,
			IsMalicious: rng.Float64() < c.MaliciousRate,
			CVP:         1.0,
		}
	}
	return nodes
}

func runOnce(c Config, seed int64) (finalAlive float64, curve []float64) {
	rng := rand.New(rand.NewSource(seed))
	nodes := newNodes(c, rng)
	curve = make([]float64, c.Epochs)
	var alive int
	for e := 0; e < c.Epochs; e++ {
		alive, _ = c.step(nodes, rng)
		curve[e] = float64(alive)
	}
	return float64(alive), curve
}

func aggregate(c Config) (meanFinal float64, meanCurve []float64) {
	type result struct {
		final float64
		curve []float64
	}
	ch := make(chan result, c.Runs)
	var wg sync.WaitGroup
	for r := 0; r < c.Runs; r++ {
		wg.Add(1)
		go func(runIdx int) {
			defer wg.Done()
			final, curve := runOnce(c, c.SeedBase+int64(runIdx)*997)
			ch <- result{final: final, curve: curve}
		}(r)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	meanCurve = make([]float64, c.Epochs)
	var n int
	for res := range ch {
		meanFinal += res.final
		for i := range res.curve {
			meanCurve[i] += res.curve[i]
		}
		n++
	}
	meanFinal /= float64(n)
	for i := range meanCurve {
		meanCurve[i] /= float64(n)
	}
	return meanFinal, meanCurve
}

func writeCSV(path string, header []string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func f(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

func main() {
	outDir := flag.String("out", "results", "output directory")
	runsMain := flag.Int("runs-main", 1000, "runs for the reference curve")
	runsSweep := flag.Int("runs-sweep", 300, "runs for sensitivity/ablation")
	n := flag.Int("n", 500, "network size")
	epochs := flag.Int("epochs", 100, "epochs")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		panic(err)
	}

	base := Config{
		NetworkSize:   *n,
		Epochs:        *epochs,
		FanOut:        3,
		BenignLoad:    0.1,
		MaliciousLoad: 0.8,
		CVPFloor:      0.3,
		EntropyLimit:  0.95,
		BurstThresh:   0.5,
		SeedBase:      20260816,
	}

	// 1) Reference survival curves.
	fmt.Println("== reference curves ==")
	curveRows := [][]string{}
	for _, mode := range []Mode{ModeBaseline, ModeFull} {
		c := base
		c.Mode = mode
		c.MaliciousRate = 0.05
		c.Runs = *runsMain
		final, curve := aggregate(c)
		fmt.Printf("  %s final alive = %.2f / %d\n", mode, final, c.NetworkSize)
		for i, v := range curve {
			curveRows = append(curveRows, []string{string(mode), strconv.Itoa(i + 1), f(v), f(100 * v / float64(c.NetworkSize))})
		}
	}
	if err := writeCSV(*outDir+"/survival_curve.csv", []string{"mode", "epoch", "alive", "survival_pct"}, curveRows); err != nil {
		panic(err)
	}

	// 2) Malicious-rate sensitivity.
	fmt.Println("== malicious-rate sweep ==")
	rates := []float64{0.01, 0.05, 0.10, 0.15, 0.20, 0.30, 0.40, 0.50}
	sensRows := [][]string{}
	for _, rate := range rates {
		for _, mode := range []Mode{ModeBaseline, ModeFull} {
			c := base
			c.Mode = mode
			c.MaliciousRate = rate
			c.Runs = *runsSweep
			final, _ := aggregate(c)
			pct := 100 * final / float64(c.NetworkSize)
			fmt.Printf("  m=%.2f %s  %.2f (%.1f%%)\n", rate, mode, final, pct)
			sensRows = append(sensRows, []string{f(rate), string(mode), f(final), f(pct)})
		}
	}
	if err := writeCSV(*outDir+"/sensitivity_malicious.csv", []string{"malicious_rate", "mode", "alive", "survival_pct"}, sensRows); err != nil {
		panic(err)
	}

	// 3) Ablations at 5% and 20%.
	fmt.Println("== ablations ==")
	ablateModes := []Mode{ModeFull, ModeNoCVP, ModeNoBreaker, ModeNoBurst, ModeBreakerOnly, ModeCVPOnly, ModeBaseline}
	ablateRows := [][]string{}
	for _, rate := range []float64{0.05, 0.20} {
		for _, mode := range ablateModes {
			c := base
			c.Mode = mode
			c.MaliciousRate = rate
			c.Runs = *runsSweep
			final, _ := aggregate(c)
			pct := 100 * final / float64(c.NetworkSize)
			fmt.Printf("  m=%.2f %-14s %.2f (%.1f%%)\n", rate, mode, final, pct)
			ablateRows = append(ablateRows, []string{f(rate), string(mode), f(final), f(pct)})
		}
	}
	if err := writeCSV(*outDir+"/ablation.csv", []string{"malicious_rate", "mode", "alive", "survival_pct"}, ablateRows); err != nil {
		panic(err)
	}

	// 4) Adaptive / fragmented-load adversary (evade burst tax).
	fmt.Println("== adaptive malicious load ==")
	adaptRows := [][]string{}
	for _, mLoad := range []float64{0.80, 0.45, 0.20, 0.12} {
		for _, mode := range []Mode{ModeBaseline, ModeFull} {
			c := base
			c.Mode = mode
			c.MaliciousRate = 0.05
			c.MaliciousLoad = mLoad
			c.Runs = *runsSweep
			final, _ := aggregate(c)
			pct := 100 * final / float64(c.NetworkSize)
			fmt.Printf("  load=%.2f %s  %.2f (%.1f%%)\n", mLoad, mode, final, pct)
			adaptRows = append(adaptRows, []string{f(mLoad), string(mode), f(final), f(pct)})
		}
	}
	if err := writeCSV(*outDir+"/adaptive_load.csv", []string{"malicious_load", "mode", "alive", "survival_pct"}, adaptRows); err != nil {
		panic(err)
	}

	fmt.Println("done")
}
