package gateeval

import (
	"fmt"

	"github.com/kryneuse/alpha_proxy/internal/gate"
)

// Metrics holds binary gate metrics.
type Metrics struct {
	TP int
	FP int
	TN int
	FN int

	Precision float64
	Recall    float64
	F1        float64
	FNR       float64

	// Routing rates.
	SafeRate      float64
	UncertainRate float64
	LikelyRate    float64
}

// Evaluate runs the gate over the dataset and computes binary metrics.
// SAFE is treated as predicted negative; UNCERTAIN and LIKELY_PII are treated
// as predicted positive (suspicious).
func Evaluate(g *gate.Gate, samples []Sample) Metrics {
	var tp, fp, tn, fn int
	var safe, uncertain, likely int
	total := len(samples)

	for _, s := range samples {
		d := g.Evaluate(s.Text)
		switch d.Route {
		case gate.SAFE:
			safe++
			if s.HasPII == 1 {
				fn++
			} else {
				tn++
			}
		case gate.UNCERTAIN:
			uncertain++
			if s.HasPII == 1 {
				tp++
			} else {
				fp++
			}
		case gate.LIKELY_PII:
			likely++
			if s.HasPII == 1 {
				tp++
			} else {
				fp++
			}
		}
	}

	precision := ratio(tp, tp+fp)
	recall := ratio(tp, tp+fn)
	f1 := f1Score(precision, recall)
	fnr := ratio(fn, fn+tp)

	return Metrics{
		TP:            tp,
		FP:            fp,
		TN:            tn,
		FN:            fn,
		Precision:     precision,
		Recall:        recall,
		F1:            f1,
		FNR:           fnr,
		SafeRate:      float64(safe) / float64(total),
		UncertainRate: float64(uncertain) / float64(total),
		LikelyRate:    float64(likely) / float64(total),
	}
}

// SweepResult is one row of a threshold sweep.
type SweepResult struct {
	LowThreshold float64
	Recall       float64
	Precision    float64
	FNR          float64
	SafeRate     float64
}

// SweepLowThreshold runs the gate over the dataset for several LowThreshold
// values (HighThreshold fixed) and returns the trade-off.
func SweepLowThreshold(samples []Sample, highThreshold float64, lows []float64) []SweepResult {
	var out []SweepResult
	for _, low := range lows {
		cfg := gate.DefaultConfig()
		cfg.LowThreshold = low
		cfg.HighThreshold = highThreshold
		g := gate.New(cfg)
		m := Evaluate(g, samples)
		out = append(out, SweepResult{
			LowThreshold: low,
			Recall:       m.Recall,
			Precision:    m.Precision,
			FNR:          m.FNR,
			SafeRate:     m.SafeRate,
		})
	}
	return out
}

// Print writes a human-readable report.
func (m Metrics) Print() {
	fmt.Printf("=== Heuristic Gate Evaluation ===\n")
	fmt.Printf("TP=%d FP=%d TN=%d FN=%d\n", m.TP, m.FP, m.TN, m.FN)
	fmt.Printf("Precision: %.3f\n", m.Precision)
	fmt.Printf("Recall:    %.3f\n", m.Recall)
	fmt.Printf("F1:        %.3f\n", m.F1)
	fmt.Printf("FNR:       %.3f\n", m.FNR)
	fmt.Printf("Safe rate:      %.3f\n", m.SafeRate)
	fmt.Printf("Uncertain rate: %.3f\n", m.UncertainRate)
	fmt.Printf("Likely-PII rate: %.3f\n", m.LikelyRate)
}

func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func f1Score(p, r float64) float64 {
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}
