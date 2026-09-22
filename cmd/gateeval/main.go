// Command gateeval runs the heuristic gate evaluation and threshold sweep.
package main

import (
	"fmt"

	"github.com/kryneuse/alpha_proxy/internal/gate"
	"github.com/kryneuse/alpha_proxy/internal/gateeval"
)

func main() {
	samples := gateeval.Dataset()
	g := gate.New(gate.DefaultConfig())
	m := gateeval.Evaluate(g, samples)
	m.Print()

	fmt.Printf("\n=== LowThreshold sweep (HighThreshold=%.2f) ===\n", gate.DefaultConfig().HighThreshold)
	fmt.Printf("%-12s %-8s %-10s %-8s %-10s\n", "LowThresh", "Recall", "Precision", "FNR", "SafeRate")
	for _, r := range gateeval.SweepLowThreshold(samples, gate.DefaultConfig().HighThreshold, []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7}) {
		fmt.Printf("%-12.2f %-8.3f %-10.3f %-8.3f %-10.3f\n", r.LowThreshold, r.Recall, r.Precision, r.FNR, r.SafeRate)
	}
}
