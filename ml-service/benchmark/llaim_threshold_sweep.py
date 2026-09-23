"""LLAIM gate threshold sweep for the ALFAGEN PII pipeline.

RESEARCH/EXPERIMENT script. NOT part of production. Does NOT change the
production threshold (0.001) or any production code.

The LLAIM gate decision is:
    gate_open = bool(matched_types) OR gate_score >= threshold

where gate_score is the softmax probability P(B-type)+P(I-type) maximised over
allowed types/tokens (in [0,1]). The production threshold is 0.001.

This script sweeps the score threshold on the UNCERTAIN subset (where the cheap
ML gate sits in the cascade) and reports per-threshold metrics plus the impact
on the whole cascade.

Routing logic:
    SAFE       -> stop
    LIKELY_PII -> expensive extractor directly (always)
    UNCERTAIN  -> cheap gate positive -> expensive; cheap gate negative -> stop

Corrected routing recall (denominator = positive residual chunks only):
    routing TP = (LIKELY_PII positives) + (UNCERTAIN positives caught)
    routing FN = (UNCERTAIN positives missed) + (SAFE positives)
    routing recall = routing TP / total_positive_residual

Usage:
  python llaim_threshold_sweep.py --onnx <path> --tokenizer <dir> --dataset residual_dataset.json
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Dict, List

# Sweep thresholds. The production threshold is 0.001. The requested range is
# 0.05..0.90 (raising the threshold). We also include the current threshold and
# lower values to explore the "lower threshold -> higher recall" direction.
SWEEP_THRESHOLDS = [
    0.00001, 0.00005, 0.0001, 0.0005, 0.001,  # current + lower
    0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35, 0.40, 0.45, 0.50,
    0.60, 0.70, 0.80, 0.90,
]


def gate_open(matched_types: List[str], score: float, threshold: float) -> bool:
    """Replicate production gate decision: matched_types OR score >= threshold."""
    return bool(matched_types) or score >= threshold


def uncertain_metrics(samples: List[Dict], threshold: float) -> Dict:
    """Metrics on the UNCERTAIN subset at a given threshold."""
    tp = fp = tn = fn = 0
    for s in samples:
        pred = gate_open(s["matched_types"], s["score"], threshold)
        label = s["has_pii"]
        if pred:
            if label == 1:
                tp += 1
            else:
                fp += 1
        else:
            if label == 1:
                fn += 1
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
    return {
        "TP": tp, "FP": fp, "TN": tn, "FN": fn,
        "Precision": precision, "Recall": recall, "F1": f1, "FNR": fnr,
        "PredictedSafeRate": predicted_safe,
        "CorrectSafeRate": correct_safe,
        "LeakRate": leak,
    }


def cascade_metrics(all_samples: List[Dict], threshold: float) -> Dict:
    """Cascade metrics over all chunks at a given threshold.

    SAFE -> stop; LIKELY_PII -> expensive; UNCERTAIN -> cheap gate decides.
    """
    total = len(all_samples)
    total_pos = sum(1 for s in all_samples if s["has_pii"] == 1)
    routing_tp = 0
    routing_fn = 0
    expensive = 0
    correct_safe = 0
    leak = 0
    for s in all_samples:
        route = s["route"]
        label = s["has_pii"]
        if route == "SAFE":
            if label == 1:
                routing_fn += 1
                leak += 1
            else:
                correct_safe += 1
        elif route == "LIKELY_PII":
            expensive += 1
            if label == 1:
                routing_tp += 1
        elif route == "UNCERTAIN":
            pred = gate_open(s["matched_types"], s["score"], threshold)
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
        else:
            raise ValueError(f"unknown route {route}")

    routing_recall = routing_tp / total_pos if total_pos else 0.0
    return {
        "RoutingRecall": routing_recall,
        "RoutingFN": routing_fn,
        "ExpensiveRate": expensive / total if total else 0.0,
        "CorrectSafeRate": correct_safe / total if total else 0.0,
        "LeakRate": leak / total if total else 0.0,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="LLAIM threshold sweep")
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
              "Export it first (see scripts/export_llaim.py).")
        sys.exit(2)

    sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
    from ml_service.core.gate import LlaimGate
    from ml_service.core.onnx_model import OnnxTokenClassifier

    with open(args.dataset, "r", encoding="utf-8") as f:
        all_samples = json.load(f)["samples"]

    model = OnnxTokenClassifier(args.onnx, args.tokenizer, intra_op_threads=1, inter_op_threads=1)
    gate = LlaimGate(model, threshold=0.001)

    # Warm-up.
    for _ in range(3):
        gate.run("Иван Петров, тел. +7 999 123-45-67")

    # Capture raw score + matched_types for every sample (same preprocessing +
    # ONNX model as production; production API unchanged).
    for s in all_samples:
        r = gate.run(s["residual"])
        s["score"] = r.score
        s["matched_types"] = r.matched_types

    uncertain = [s for s in all_samples if s["route"] == "UNCERTAIN"]
    total_pos = sum(1 for s in all_samples if s["has_pii"] == 1)

    print("=== LLAIM threshold sweep ===")
    print(f"total chunks: {len(all_samples)}, positive residual: {total_pos}, "
          f"UNCERTAIN subset: {len(uncertain)} "
          f"({sum(1 for s in uncertain if s['has_pii']==1)} pos / "
          f"{sum(1 for s in uncertain if s['has_pii']==0)} neg)")
    print(f"production threshold: 0.001")
    print()
    print("--- UNCERTAIN subset per-threshold ---")
    print(f"{'thr':>8s} {'TP':>3s} {'FP':>3s} {'TN':>3s} {'FN':>3s} "
          f"{'P':>6s} {'R':>6s} {'F1':>6s} {'FNR':>6s} "
          f"{'predSafe':>8s} {'corrSafe':>8s} {'leak':>6s}")
    rows = []
    for t in SWEEP_THRESHOLDS:
        m = uncertain_metrics(uncertain, t)
        c = cascade_metrics(all_samples, t)
        rows.append((t, m, c))
        print(f"{t:8.5f} {m['TP']:3d} {m['FP']:3d} {m['TN']:3d} {m['FN']:3d} "
              f"{m['Precision']:6.3f} {m['Recall']:6.3f} {m['F1']:6.3f} {m['FNR']:6.3f} "
              f"{m['PredictedSafeRate']:8.3f} {m['CorrectSafeRate']:8.3f} {m['LeakRate']:6.3f}")

    print()
    print("--- Cascade impact per-threshold ---")
    print(f"{'thr':>8s} {'routingR':>8s} {'routingFN':>9s} {'expRate':>8s} "
          f"{'corrSafe':>8s} {'leak':>6s}")
    for t, m, c in rows:
        print(f"{t:8.5f} {c['RoutingRecall']:8.3f} {c['RoutingFN']:9d} "
              f"{c['ExpensiveRate']:8.3f} {c['CorrectSafeRate']:8.3f} {c['LeakRate']:6.3f}")

    # Best operating points.
    print()
    print("--- Best operating points ---")
    # routing recall == 1.0
    best_r1 = [r for r in rows if r[2]["RoutingRecall"] >= 1.0 - 1e-9]
    if best_r1:
        t, m, c = best_r1[0]
        print(f"routing recall = 1.000 at threshold {t:.5f}: "
              f"LLAIM P={m['Precision']:.3f} R={m['Recall']:.3f} "
              f"routingFN={c['RoutingFN']} expRate={c['ExpensiveRate']:.3f} "
              f"leak={c['LeakRate']:.3f}")
    else:
        print("No threshold gives routing recall = 1.000.")
        # max achievable
        t, m, c = max(rows, key=lambda r: r[2]["RoutingRecall"])
        print(f"max routing recall = {c['RoutingRecall']:.3f} at threshold {t:.5f}: "
              f"LLAIM P={m['Precision']:.3f} R={m['Recall']:.3f} "
              f"routingFN={c['RoutingFN']} expRate={c['ExpensiveRate']:.3f} "
              f"leak={c['LeakRate']:.3f}")

    for target in (0.99, 0.95, 0.90):
        cands = [r for r in rows if r[2]["RoutingRecall"] >= target]
        if cands:
            # pick the one with lowest expensive rate among those meeting target
            t, m, c = min(cands, key=lambda r: r[2]["ExpensiveRate"])
            print(f"routing recall >= {target:.2f}: threshold {t:.5f} "
                  f"(routingR={c['RoutingRecall']:.3f}, routingFN={c['RoutingFN']}, "
                  f"expRate={c['ExpensiveRate']:.3f}, leak={c['LeakRate']:.3f}, "
                  f"LLAIM P={m['Precision']:.3f} R={m['Recall']:.3f})")
        else:
            print(f"routing recall >= {target:.2f}: not achievable in sweep range")


if __name__ == "__main__":
    main()