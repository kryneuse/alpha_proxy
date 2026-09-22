package eval

import (
	"fmt"
	"time"

	"github.com/alpha-proxy/rule-engine/internal/engine"
	"github.com/alpha-proxy/rule-engine/internal/entity"
)

// Metrics holds computed evaluation metrics.
type Metrics struct {
	Precision float64
	Recall    float64
	F1        float64
	// Typed holds per-type metrics.
	Typed map[entity.Type]TypeMetrics
	// FalsePositiveRate is the fraction of negative cases that produced at
	// least one entity.
	FalsePositiveRate float64
	// Latency is the average analysis time per sample.
	Latency time.Duration
}

// TypeMetrics holds per-type precision/recall/F1.
type TypeMetrics struct {
	Precision float64
	Recall    float64
	F1        float64
	TP        int
	FP        int
	FN        int
}

// Evaluate runs the engine over the dataset and computes metrics.
func Evaluate(e *engine.Engine, samples []Sample) Metrics {
	// Global counts.
	var tp, fp, fn int
	// Per-type counts.
	typed := make(map[entity.Type]*TypeMetrics)
	for _, t := range entity.AllTypes() {
		typed[t] = &TypeMetrics{}
	}

	var negTotal, negFP int
	var totalLatency time.Duration

	for _, s := range samples {
		start := time.Now()
		got := e.Analyze(s.Text)
		totalLatency += time.Since(start)

		if s.Negative {
			negTotal++
			if len(got) > 0 {
				negFP++
			}
			continue
		}

		// Count expected types.
		expectedSet := map[entity.Type]bool{}
		for _, t := range s.Expected {
			expectedSet[t] = true
		}
		// Count detected types.
		gotSet := map[entity.Type]bool{}
		for _, g := range got {
			gotSet[g.Type] = true
		}

		for t := range expectedSet {
			if gotSet[t] {
				tp++
				typed[t].TP++
			} else {
				fn++
				typed[t].FN++
			}
		}
		for t := range gotSet {
			if !expectedSet[t] {
				fp++
				typed[t].FP++
			}
		}
	}

	precision := float64(tp) / float64(tp+fp)
	recall := float64(tp) / float64(tp+fn)
	f1 := f1Score(precision, recall)

	typedMetrics := make(map[entity.Type]TypeMetrics)
	for t, m := range typed {
		p := float64(m.TP) / float64(m.TP+m.FP)
		r := float64(m.TP) / float64(m.TP+m.FN)
		typedMetrics[t] = TypeMetrics{
			Precision: p,
			Recall:    r,
			F1:        f1Score(p, r),
			TP:        m.TP,
			FP:        m.FP,
			FN:        m.FN,
		}
	}

	fpr := 0.0
	if negTotal > 0 {
		fpr = float64(negFP) / float64(negTotal)
	}

	return Metrics{
		Precision:         precision,
		Recall:            recall,
		F1:                f1,
		Typed:             typedMetrics,
		FalsePositiveRate: fpr,
		Latency:           totalLatency / time.Duration(len(samples)),
	}
}

func f1Score(p, r float64) float64 {
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// Print writes a human-readable report.
func (m Metrics) Print() {
	fmt.Printf("=== Rule Engine Evaluation ===\n")
	fmt.Printf("Precision: %.3f\n", m.Precision)
	fmt.Printf("Recall:    %.3f\n", m.Recall)
	fmt.Printf("F1:        %.3f\n", m.F1)
	fmt.Printf("False positive rate (negatives): %.3f\n", m.FalsePositiveRate)
	fmt.Printf("Avg latency per sample: %s\n", m.Latency)
	fmt.Printf("\n--- Typed metrics ---\n")
	for _, t := range entity.AllTypes() {
		tm := m.Typed[t]
		fmt.Printf("%-20s P=%.3f R=%.3f F1=%.3f (TP=%d FP=%d FN=%d)\n",
			t, tm.Precision, tm.Recall, tm.F1, tm.TP, tm.FP, tm.FN)
	}
}
