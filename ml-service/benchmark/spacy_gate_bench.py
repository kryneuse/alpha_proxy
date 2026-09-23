"""Isolated spaCy cheap-gate benchmark for the ALFAGEN PII pipeline (v2).

RESEARCH/EXPERIMENT script. NOT part of production. Must NOT be added to the
production Docker image or runtime dependencies.

v2 methodology (matches the real cascade):
  original -> Rule Engine -> residual -> Heuristic Gate
    SAFE       -> stop
    UNCERTAIN  -> ML gate (this is where spaCy/LLAIM sit)
    LIKELY_PII -> expensive extractor directly

The dataset is the REAL residual dataset produced by cmd/gateeval-residual:
each sample carries the residual text (after rule masking), the ground-truth
label (does PII remain after rules), and the heuristic-gate route.

Two policies are supported:
  Mode A: PER or LOC -> suspicious
  Mode B: PER or LOC or ORG -> suspicious

Metrics are reported on:
  - the full real residual dataset (diagnostic)
  - the UNCERTAIN subset (the main comparison, where the ML gate sits)

Corrected safe-rate metrics:
  predicted safe rate = (TN + FN) / N
  correct safe rate   = TN / N
  leak rate           = FN / N

End-to-end cascade metrics assume:
  SAFE       -> stop (no expensive call)
  UNCERTAIN  -> cheap gate positive -> expensive; cheap gate negative -> stop
  LIKELY_PII -> expensive extractor directly (regardless of cheap gate)

Usage:
  python spacy_gate_bench.py --model ru_core_news_sm --mode A --dataset residual_dataset.json
  python spacy_gate_bench.py --model ru_core_news_md --mode B --dataset residual_dataset.json
  python spacy_gate_bench.py --model ru_core_news_lg --mode A --dataset residual_dataset.json

If the model is not installed, a clear install command is printed.
Only NER-required components (tok2vec + ner) are enabled.
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

INSTALL_HINTS = {
    "ru_core_news_sm": "python -m spacy download ru_core_news_sm",
    "ru_core_news_md": "python -m spacy download ru_core_news_md",
    "ru_core_news_lg": "python -m spacy download ru_core_news_lg",
}

NON_NER_COMPONENTS = ("parser", "morphologizer", "attribute_ruler", "lemmatizer")


def load_model(model_name: str):
    import spacy

    nlp = spacy.load(model_name)
    for comp in NON_NER_COMPONENTS:
        if comp in nlp.pipe_names:
            nlp.disable_pipe(comp)
    if "ner" not in nlp.pipe_names:
        raise RuntimeError(f"model {model_name} has no 'ner' component")
    return nlp


def predict(nlp, text: str, mode: str) -> bool:
    doc = nlp(text)
    if mode == "A":
        return any(e.label_ in ("PER", "LOC") for e in doc.ents)
    return any(e.label_ in ("PER", "LOC", "ORG") for e in doc.ents)


def compute_metrics(nlp, samples: List[Dict], mode: str) -> Tuple[Dict, List[Dict]]:
    tp = fp = tn = fn = 0
    fn_examples: List[Dict] = []
    for s in samples:
        pred = predict(nlp, s["residual"], mode)
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
    """Compute cascade routing metrics given cheap-gate predictions per sample.

    Routing:
      SAFE       -> stop
      UNCERTAIN  -> cheap positive -> expensive; cheap negative -> stop
      LIKELY_PII -> expensive directly

    RoutingRecall denominator = number of POSITIVE residual chunks only (not
    all chunks). It measures what fraction of residual chunks with PII the
    cascade routed to the expensive extractor. It is NOT the final extractor
    recall (the expensive model is not actually run).
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
            # stop; if PII remains this is a leak
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
                # cheap negative -> stop
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


def perf_benchmark(nlp, texts: List[str], batch_size: int = 64, warmup: int = 5) -> Dict:
    """Measure load time, latency percentiles, batched throughput and RAM.

    Throughput uses real batching via nlp.pipe(texts, batch_size=...).
    Latency is measured per individual sample after warm-up.
    """
    # Warm-up.
    for _ in range(warmup):
        list(nlp.pipe(texts, batch_size=batch_size))

    # Per-sample latency (individual).
    latencies: List[float] = []
    for text in texts:
        t = time.perf_counter()
        list(nlp.pipe([text]))
        latencies.append((time.perf_counter() - t) * 1000.0)

    # Batched throughput.
    t0 = time.perf_counter()
    list(nlp.pipe(texts, batch_size=batch_size))
    total_s = time.perf_counter() - t0

    latencies.sort()
    p50 = latencies[len(latencies) // 2]
    p95 = latencies[int(len(latencies) * 0.95) - 1] if latencies else 0.0
    avg = sum(latencies) / len(latencies) if latencies else 0.0
    throughput = len(texts) / total_s if total_s else 0.0

    rss_kb = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    ram_mb = rss_kb / 1024.0

    return {
        "p50_ms": p50,
        "p95_ms": p95,
        "avg_ms": avg,
        "throughput_chunks_per_s": throughput,
        "ram_mb": ram_mb,
    }


def print_metrics(title: str, m: Dict, perf: Dict, load_s: float) -> None:
    print(f"--- {title} ---")
    print(f"TP={m['TP']} FP={m['FP']} TN={m['TN']} FN={m['FN']}")
    print(f"Precision: {m['Precision']:.3f}")
    print(f"Recall:    {m['Recall']:.3f}")
    print(f"F1:        {m['F1']:.3f}")
    print(f"FNR:       {m['FNR']:.3f}")
    print(f"predicted safe rate: {m['PredictedSafeRate']:.3f}")
    print(f"correct safe rate:   {m['CorrectSafeRate']:.3f}")
    print(f"leak rate:           {m['LeakRate']:.3f}")
    if perf:
        print(f"model load: {load_s:.2f}s  p50: {perf['p50_ms']:.2f}ms  "
              f"p95: {perf['p95_ms']:.2f}ms  avg: {perf['avg_ms']:.2f}ms")
        print(f"batched throughput: {perf['throughput_chunks_per_s']:.1f} chunks/s  "
              f"RAM: {perf['ram_mb']:.0f} MB")


def main() -> None:
    parser = argparse.ArgumentParser(description="spaCy cheap-gate benchmark v2")
    parser.add_argument("--model", required=True, help="ru_core_news_sm|md|lg")
    parser.add_argument("--mode", required=True, choices=["A", "B"], help="gate policy")
    parser.add_argument(
        "--dataset",
        default=os.path.join(os.path.dirname(__file__), "residual_dataset.json"),
        help="path to real residual dataset JSON",
    )
    parser.add_argument("--batch-size", type=int, default=64)
    args = parser.parse_args()

    if args.model not in INSTALL_HINTS:
        print(f"Unknown model {args.model!r}. Expected one of: " + ", ".join(INSTALL_HINTS))
        sys.exit(2)

    try:
        import spacy  # noqa: F401
    except ImportError:
        print("spaCy is not installed. Install it and the model with:\n"
              "  pip install spacy\n"
              f"  {INSTALL_HINTS[args.model]}")
        sys.exit(2)

    try:
        t_load0 = time.perf_counter()
        nlp = load_model(args.model)
        load_s = time.perf_counter() - t_load0
    except OSError as exc:
        print(f"Failed to load model {args.model!r}: {exc}\n"
              f"Install it with:\n  {INSTALL_HINTS[args.model]}")
        sys.exit(2)

    with open(args.dataset, "r", encoding="utf-8") as f:
        samples = json.load(f)["samples"]

    uncertain = [s for s in samples if s["route"] == "UNCERTAIN"]

    print(f"=== spaCy gate benchmark v2: {args.model} Mode {args.mode} ===")
    print(f"full residual dataset: {len(samples)} samples, "
          f"UNCERTAIN subset: {len(uncertain)} samples")

    # Full real residual dataset (diagnostic).
    full_metrics, full_fn = compute_metrics(nlp, samples, args.mode)
    print_metrics("Full real residual dataset", full_metrics, None, load_s)

    # UNCERTAIN subset (main comparison).
    unc_metrics, unc_fn = compute_metrics(nlp, uncertain, args.mode)
    print_metrics("UNCERTAIN subset", unc_metrics, None, load_s)

    # Performance on the full dataset (same data for all models).
    texts = [s["residual"] for s in samples]
    perf = perf_benchmark(nlp, texts, batch_size=args.batch_size)
    print_metrics("Performance (full dataset)", full_metrics, perf, load_s)

    # End-to-end cascade metrics.
    cheap_preds = [predict(nlp, s["residual"], args.mode) for s in samples]
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