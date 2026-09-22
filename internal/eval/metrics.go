package eval

import (
	"fmt"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// Metrics holds computed evaluation metrics.
type Metrics struct {
	// SpanDetection metrics (IoU >= 0.5, type NOT required to match).
	SpanDetectionPrecision float64
	SpanDetectionRecall    float64
	SpanDetectionF1        float64
	// TypedSpan metrics (IoU >= 0.5, type must match).
	TypedSpanPrecision float64
	TypedSpanRecall    float64
	TypedSpanF1        float64
	// Exact-span metrics (IoU == 1.0, type must match).
	ExactPrecision float64
	ExactRecall    float64
	ExactF1        float64
	// Typed holds per-type metrics.
	Typed map[entity.Type]TypeMetrics
	// FalsePositiveRate is the fraction of negative cases that produced at
	// least one entity.
	FalsePositiveRate float64
	// Latency is the average analysis time per sample.
	Latency time.Duration
	// OffsetErrors is the number of predicted entities whose
	// original[start:end] != text.
	OffsetErrors int
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

// IoUThreshold is the minimum IoU for a span match.
const IoUThreshold = 0.5

// Evaluate runs the engine over the dataset and computes metrics.
func Evaluate(e *engine.Engine, samples []Sample) Metrics {
	var detTP, detFP, detFN int
	var spanTP, spanFP, spanFN int
	var exactTP, exactFP, exactFN int
	typed := make(map[entity.Type]*TypeMetrics)
	for _, t := range entity.AllTypes() {
		typed[t] = &TypeMetrics{}
	}

	var negTotal, negFP int
	var totalLatency time.Duration
	offsetErrors := 0

	for _, s := range samples {
		start := time.Now()
		got := e.Analyze(s.Text)
		totalLatency += time.Since(start)

		// Validate offsets: original[start:end] must equal text.
		for _, g := range got {
			if g.Start < 0 || g.End > len(s.Text) || g.Start > g.End {
				offsetErrors++
				continue
			}
			if s.Text[g.Start:g.End] != g.Text {
				offsetErrors++
			}
		}

		if s.Negative {
			negTotal++
			if len(got) > 0 {
				negFP++
			}
			// Every prediction on a negative sample is a false positive.
			for _, g := range got {
				detFP++
				spanFP++
				exactFP++
				typed[g.Type].FP++
			}
			continue
		}

		// Span detection matching (IoU >= threshold, type NOT required).
		detMatch := matchSpans(s.Expected, got, IoUThreshold, false)
		// Typed span matching (IoU >= threshold, type required).
		spanMatch := matchSpans(s.Expected, got, IoUThreshold, true)
		// Exact matching (IoU == 1.0, type required).
		exactMatch := matchSpans(s.Expected, got, 1.0, true)

		// Span detection TP/FP/FN.
		for _, m := range detMatch.results {
			if m.matched {
				detTP++
			} else {
				detFN++
			}
		}
		for i := range got {
			if !detMatch.used[i] {
				detFP++
			}
		}

		// Typed span TP/FP/FN.
		for _, m := range spanMatch.results {
			if m.matched {
				spanTP++
				typed[m.expected.Type].TP++
			} else {
				spanFN++
				typed[m.expected.Type].FN++
			}
		}
		for i, g := range got {
			if !spanMatch.used[i] {
				spanFP++
				typed[g.Type].FP++
			}
		}

		// Exact-level TP/FP/FN.
		for _, m := range exactMatch.results {
			if m.matched {
				exactTP++
			} else {
				exactFN++
			}
		}
		for i := range got {
			if !exactMatch.used[i] {
				exactFP++
			}
		}
	}

	detPrecision := ratio(detTP, detTP+detFP)
	detRecall := ratio(detTP, detTP+detFN)
	spanPrecision := ratio(spanTP, spanTP+spanFP)
	spanRecall := ratio(spanTP, spanTP+spanFN)
	exactPrecision := ratio(exactTP, exactTP+exactFP)
	exactRecall := ratio(exactTP, exactTP+exactFN)

	typedMetrics := make(map[entity.Type]TypeMetrics)
	for t, m := range typed {
		p := ratio(m.TP, m.TP+m.FP)
		r := ratio(m.TP, m.TP+m.FN)
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
		SpanDetectionPrecision: detPrecision,
		SpanDetectionRecall:    detRecall,
		SpanDetectionF1:        f1Score(detPrecision, detRecall),
		TypedSpanPrecision:     spanPrecision,
		TypedSpanRecall:        spanRecall,
		TypedSpanF1:            f1Score(spanPrecision, spanRecall),
		ExactPrecision:         exactPrecision,
		ExactRecall:            exactRecall,
		ExactF1:                f1Score(exactPrecision, exactRecall),
		Typed:                  typedMetrics,
		FalsePositiveRate:      fpr,
		Latency:                totalLatency / time.Duration(len(samples)),
		OffsetErrors:           offsetErrors,
	}
}

// matchResult is the result of matching one expected span.
type matchResult struct {
	expected Expected
	matched  bool
}

// matchSet is the result of matching expected to predicted spans.
type matchSet struct {
	results []matchResult
	// used[i] is true if predicted span i was consumed by a match.
	used []bool
}

// matchSpans performs greedy one-to-one matching of expected to predicted
// spans by IoU. Each predicted span can match at most one expected span, and
// each expected span at most one predicted span. A match requires IoU >=
// threshold. If requireType is true, the predicted type must equal the
// expected type. The used slice records which predicted indices were consumed.
func matchSpans(expected []Expected, got []entity.Entity, threshold float64, requireType bool) matchSet {
	results := make([]matchResult, len(expected))
	used := make([]bool, len(got))
	for i, exp := range expected {
		bestIdx := -1
		bestIoU := 0.0
		for j, g := range got {
			if used[j] {
				continue
			}
			if requireType && g.Type != exp.Type {
				continue
			}
			iou := spanIoU(exp.Start, exp.End, g.Start, g.End)
			if iou > bestIoU {
				bestIoU = iou
				bestIdx = j
			}
		}
		if bestIdx >= 0 && bestIoU >= threshold {
			used[bestIdx] = true
			results[i] = matchResult{expected: exp, matched: true}
		} else {
			results[i] = matchResult{expected: exp}
		}
	}
	return matchSet{results: results, used: used}
}

// spanIoU computes the intersection-over-union of two spans.
func spanIoU(aStart, aEnd, bStart, bEnd int) float64 {
	interStart := max(aStart, bStart)
	interEnd := min(aEnd, bEnd)
	inter := interEnd - interStart
	if inter <= 0 {
		return 0
	}
	union := (aEnd - aStart) + (bEnd - bStart) - inter
	return float64(inter) / float64(union)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Print writes a human-readable report.
func (m Metrics) Print() {
	fmt.Printf("=== Rule Engine Evaluation ===\n")
	fmt.Printf("Span detection (IoU>=%.1f, type NOT required):\n", IoUThreshold)
	fmt.Printf("  Precision: %.3f\n", m.SpanDetectionPrecision)
	fmt.Printf("  Recall:    %.3f\n", m.SpanDetectionRecall)
	fmt.Printf("  F1:        %.3f\n", m.SpanDetectionF1)
	fmt.Printf("Typed span (IoU>=%.1f, type required):\n", IoUThreshold)
	fmt.Printf("  Precision: %.3f\n", m.TypedSpanPrecision)
	fmt.Printf("  Recall:    %.3f\n", m.TypedSpanRecall)
	fmt.Printf("  F1:        %.3f\n", m.TypedSpanF1)
	fmt.Printf("Exact-span (IoU==1.0, type required):\n")
	fmt.Printf("  Precision: %.3f\n", m.ExactPrecision)
	fmt.Printf("  Recall:    %.3f\n", m.ExactRecall)
	fmt.Printf("  F1:        %.3f\n", m.ExactF1)
	fmt.Printf("False positive rate (negatives): %.3f\n", m.FalsePositiveRate)
	fmt.Printf("Offset errors: %d\n", m.OffsetErrors)
	fmt.Printf("Avg latency per sample: %s\n", m.Latency)
	fmt.Printf("\n--- Typed metrics ---\n")
	for _, t := range entity.AllTypes() {
		tm := m.Typed[t]
		fmt.Printf("%-20s P=%.3f R=%.3f F1=%.3f (TP=%d FP=%d FN=%d)\n",
			t, tm.Precision, tm.Recall, tm.F1, tm.TP, tm.FP, tm.FN)
	}
}
