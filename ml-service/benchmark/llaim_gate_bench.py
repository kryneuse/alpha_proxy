"""Isolated LLAIM gate benchmark for the ALFAGEN PII pipeline (v2).

RESEARCH/EXPERIMENT script. NOT part of production. Reuses the existing
ml_service.core.gate.LlaimGate and ml_service.core.onnx_model.OnnxTokenClassifier
so the comparison is against the real production gate logic.

v2 methodology (matches the real cascade):
  original -> Rule Engine -> residual -> Heuristic Gate
    SAFE       -> stop
    UNCERTAIN  -> ML gate (this is where LLAIM/spaCy sit)
    LIKELY_PII -> expensive extractor directly

The dataset is the REAL residual dataset produced by cmd/gateeval-residual.
The gate is treated as a binary suspicious/not-suspicious classifier:
  gate.open == True  -> predicted PII = true
  gate.open == False -> predicted PII = false

Metrics are reported on the full real residual dataset (diagnostic) and the
UNCERTAIN subset (main comparison). Corrected safe-rate metrics and end-to-end
cascade metrics are included.

Usage:
  python llaim_gate_bench.py --onnx <path> --tokenizer <dir> --dataset residual_dataset.json

The ONNX model is not shipped in the repo (gitignored). It must be exported
from the PyTorch safetensors first (see scripts/export_llaim.py).
"""
from __future__ import annotations

import argparse
import json
import os
import resource
import statistics
import sys
import time
from typing import Dict, List, Tuple


def compute_metrics(gate, samples: List[Dict]) -> Tuple[Dict, List[Dict]]:
    tp = fp = tn = fn = 0
    fn_examples: List[Dict] = []
    for s in samples:
        pred = gate.run(s["residual"]).open
        label = s["has_pii"]
        if pred:
            if label == 1:
                tp += 1
            else:
                fp += 1
        else:
            if label == 1:
                fn += 1
                fn_examples.append({"residual": s["residual"], "has_pii": label})
            else:
                tn += 1
    total = len(samples)
    precision = tp / (tp + fp) if (tp + fp) else 0.0
    recall = tp / (tp + fn) if (tp + fn) else 0.0
    f1 = 2 * precision * recall / (precision + recall) if (precision + recall) else 0.0
    fnr = fn / (fn + tp) if (fn + tp) else 0.0
    predicted_safe = (tn + fn) / total if total else 0.0
    correct_safe = tn / total if total else 0.0
    leak = fn / total if total else 0.0
    metrics = {
        "TP": tp, "FP": fp, "TN": tn, "FN": fn,
        "Precision": precision, "Recall": recall, "F1": f1, "FNR": fnr,
        "PredictedSafeRate": predicted_safe,
        "CorrectSafeRate": correct_safe,
        "LeakRate": leak,
    }
    return metrics, fn_examples


def end_to_end_metrics(samples: List[Dict], cheap_preds: List[bool]) -> Dict:
    """Cascade routing metrics.

    Routing:
      SAFE       -> stop
      UNCERTAIN  -> cheap positive -> expensive; cheap negative -> stop
      LIKELY_PII -> expensive directly

    RoutingRecall denominator = number of POSITIVE residual chunks only
    (not all chunks). It measures what fraction of residual chunks with PII the
    cascade routed to the expensive extractor instead of releasing early. It is
    NOT the final extractor recall (the expensive model is not actually run).
    """
    total = len(samples)
    total_pos = sum(1 for s in samples if s["has_pii"] == 1)
    routing_tp = 0
    routing_fn = 0
    expensive = 0
    cheap_invoked = 0
    correct_safe = 0
    leak = 0
    for s, pred in zip(samples, cheap_preds):
        route = s["route"]
        label = s["has_pii"]
        if route == "SAFE":
            if label == 1:
                routing_fn += 1
                leak += 1
            else:
                correct_safe += 1
        elif route == "UNCERTAIN":
            cheap_invoked += 1
            if pred:
                expensive += 1
                if label == 1:
                    routing_tp += 1
            else:
                if label == 1:
                    routing_fn += 1
                    leak += 1
                else:
                    correct_safe += 1
        elif route == "LIKELY_PII":
            expensive += 1
            if label == 1:
                routing_tp += 1
        else:
            raise ValueError(f"unknown route {route}")

    routing_recall = routing_tp / total_pos if total_pos else 0.0
    return {
        "RoutingRecall": routing_recall,
        "RoutingFN": routing_fn,
        "CheapInvocationRate": cheap_invoked / total if total else 0.0,
        "ExpensiveInvocationRate": expensive / total if total else 0.0,
        "CorrectSafeRate": correct_safe / total if total else 0.0,
        "LeakRate": leak / total if total else 0.0,
    }


def print_metrics(title: str, m: Dict) -> None:
    print(f"--- {title} ---")
    print(f"TP={m['TP']} FP={m['FP']} TN={m['TN']} FN={m['FN']}")
    print(f"Precision: {m['Precision']:.3f}")
    print(f"Recall:    {m['Recall']:.3f}")
    print(f"F1:        {m['F1']:.3f}")
    print(f"FNR:       {m['FNR']:.3f}")
    print(f"predicted safe rate: {m['PredictedSafeRate']:.3f}")
    print(f"correct safe rate:   {m['CorrectSafeRate']:.3f}")
    print(f"leak rate:           {m['LeakRate']:.3f}")


def main() -> None:
    parser = argparse.ArgumentParser(description="LLAIM gate benchmark v2")
    parser.add_argument("--onnx", required=True, help="path to llaim-ru-legal-ner.onnx")
    parser.add_argument("--tokenizer", required=True, help="path to tokenizer dir")
    parser.add_argument(
        "--dataset",
        default=os.path.join(os.path.dirname(__file__), "residual_dataset.json"),
        help="path to real residual dataset JSON",
    )
    args = parser.parse_args()

    if not os.path.exists(args.onnx):
        print(f"ONNX model not found: {args.onnx}\n"
              "The LLAIM ONNX is gitignored and not shipped. Export it first:\n"
              "  python scripts/export_llaim.py   (requires torch + onnx)\n"
              "or download the PyTorch weights and export to ONNX.")
        sys.exit(2)

    sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
    from ml_service.core.gate import LlaimGate
    from ml_service.core.onnx_model import OnnxTokenClassifier

    with open(args.dataset, "r", encoding="utf-8") as f:
        samples = json.load(f)["samples"]

    uncertain = [s for s in samples if s["route"] == "UNCERTAIN"]

    t_load0 = time.perf_counter()
    model = OnnxTokenClassifier(args.onnx, args.tokenizer, intra_op_threads=1, inter_op_threads=1)
    gate = LlaimGate(model, threshold=0.001)
    load_s = time.perf_counter() - t_load0

    # Warm-up.
    for _ in range(3):
        gate.run("Иван Петров, тел. +7 999 123-45-67")

    print("=== LLAIM gate benchmark v2 ===")
    print(f"model load: {load_s:.2f}s")
    print(f"full residual dataset: {len(samples)} samples, "
          f"UNCERTAIN subset: {len(uncertain)} samples")

    full_metrics, full_fn = compute_metrics(gate, samples)
    print_metrics("Full real residual dataset", full_metrics)

    unc_metrics, unc_fn = compute_metrics(gate, uncertain)
    print_metrics("UNCERTAIN subset", unc_metrics)

    # Latency on full dataset.
    latencies: List[float] = []
    for s in samples:
        t0 = time.perf_counter()
        gate.run(s["residual"])
        latencies.append((time.perf_counter() - t0) * 1000.0)
    latencies.sort()
    p50 = latencies[len(latencies) // 2]
    p95 = latencies[int(len(latencies) * 0.95) - 1] if latencies else 0.0
    avg = sum(latencies) / len(latencies) if latencies else 0.0
    throughput = len(samples) / (sum(latencies) / 1000.0) if latencies else 0.0
    rss_kb = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    ram_mb = rss_kb / 1024.0
    print(f"p50: {p50:.2f}ms  p95: {p95:.2f}ms  avg: {avg:.2f}ms")
    print(f"throughput: {throughput:.1f} chunks/s  RAM: {ram_mb:.0f} MB")

    # End-to-end cascade metrics.
    cheap_preds = [gate.run(s["residual"]).open for s in samples]
    e2e = end_to_end_metrics(samples, cheap_preds)
    print("--- Cascade routing metrics ---")
    print(f"routing recall:           {e2e['RoutingRecall']:.3f}")
    print(f"routing FN:               {e2e['RoutingFN']}")
    print(f"cheap-model invocation:   {e2e['CheapInvocationRate']:.3f}")
    print(f"expensive-model invocation: {e2e['ExpensiveInvocationRate']:.3f}")
    print(f"correct safe rate:        {e2e['CorrectSafeRate']:.3f}")
    print(f"leak rate:                {e2e['LeakRate']:.3f}")

    print(f"\n=== False Negatives on UNCERTAIN subset ({len(unc_fn)}) ===")
    for ex in unc_fn:
        print(f"  - {ex['residual']!r}")

    print(f"\n=== False Negatives on full dataset ({len(full_fn)}) ===")
    for ex in full_fn:
        print(f"  - {ex['residual']!r}")


if __name__ == "__main__":
    main()