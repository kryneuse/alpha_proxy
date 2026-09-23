"""Parity check: ml-service LeanGate/LeanNer vs reference pii-lab LeanModel.

Loads the same ONNX artifacts (gate1-v3-onnx, source99-expansion-v14a-epoch1-onnx)
through both runtimes and compares, on the same reference pairs:
  * gate score (float32 sigmoid, max over overflowing windows)
  * NER spans (type, start, end) before repair

Reference pairs come from pii-lab reports/rules_e2e_v1/chunks.json (real chunks).
A pair is (original_text, gate_text); for model parity we feed the same text to
both the gate and the NER, exactly as the reference LeanModel does.

Usage:
  python scripts/parity_check.py [--limit N] [--gate-dir DIR] [--ner-dir DIR]
                                 [--pairs PATH] [--pii-lab-src DIR]
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

# --- locate pii-lab reference runtime -------------------------------------
_PII_LAB_SRC = Path("/Users/aekuleshevskii/alpha_test_pd/pii-lab/src")
_PII_LAB_MODELS = Path("/Users/aekuleshevskii/alpha_test_pd/pii-lab/models")
_ML_SERVICE_ROOT = Path(__file__).resolve().parents[1]

# ml-service package must be importable.
sys.path.insert(0, str(_ML_SERVICE_ROOT))
# pii-lab reference modules (e2e_runtime, schema, postprocess) live in src/.
sys.path.insert(0, str(_PII_LAB_SRC))

from ml_service.core.onnx_model import LeanGate, LeanNer  # noqa: E402
from e2e_runtime import LeanModel  # noqa: E402


def spans(entities):
    return {(e["type"], e["start"], e["end"]) for e in entities}


def load_pairs(path: Path, limit: int):
    rows = json.loads(path.read_text(encoding="utf-8"))
    texts = [r["text"] for r in rows]
    if limit:
        texts = texts[:limit]
    return texts


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--limit", type=int, default=0, help="max pairs to check (0 = all)")
    parser.add_argument("--gate-dir", default=str(_PII_LAB_MODELS / "gate1-v3-onnx"))
    parser.add_argument("--ner-dir", default=str(_PII_LAB_MODELS / "source99-expansion-v14a-epoch1-onnx"))
    parser.add_argument("--pairs", default=str(_PII_LAB_MODELS.parent / "reports/rules_e2e_v1/chunks.json"))
    args = parser.parse_args()

    gate_dir = Path(args.gate_dir)
    ner_dir = Path(args.ner_dir)
    pairs_path = Path(args.pairs)
    for p in (gate_dir, ner_dir, pairs_path):
        if not p.exists():
            raise SystemExit(f"missing path: {p}")

    texts = load_pairs(pairs_path, args.limit)
    if not texts:
        raise SystemExit("no reference pairs loaded")

    # My runtime (ml-service).
    my_gate = LeanGate(str(gate_dir), intra_op_threads=1, inter_op_threads=1, tensor_batch_size=1)
    my_ner = LeanNer(str(ner_dir), intra_op_threads=1, inter_op_threads=1, tensor_batch_size=1)

    # Reference runtime (pii-lab), same artifacts.
    ref_gate = LeanModel(str(gate_dir), True, gate=True)
    ref_ner = LeanModel(str(ner_dir), True, gate=False)

    gate_deltas = []
    ner_disagreements = []
    for i, text in enumerate(texts):
        my_score = my_gate.score(text)
        ref_score = ref_gate.run(text)
        delta = abs(my_score - ref_score)
        gate_deltas.append(delta)
        if delta != 0:
            print(f"  gate delta {delta:.3e} at pair {i}: {text[:60]!r}")

        my_es = my_ner.run(text)
        ref_es = ref_ner.run(text)
        if spans(my_es) != spans(ref_es):
            ner_disagreements.append(i)
            print(f"  NER span mismatch at pair {i}: {text[:60]!r}")
            print(f"    mine: {sorted(spans(my_es))}")
            print(f"    ref : {sorted(spans(ref_es))}")

    max_gate_delta = max(gate_deltas) if gate_deltas else 0.0
    report = {
        "pairs_checked": len(texts),
        "gate_max_score_difference": max_gate_delta,
        "gate_exact_match": max_gate_delta == 0,
        "ner_span_disagreements": ner_disagreements,
        "ner_exact_match": not ner_disagreements,
        "gate_dir": str(gate_dir),
        "ner_dir": str(ner_dir),
        "pairs": str(pairs_path),
    }
    print(json.dumps(report, ensure_ascii=False, indent=2))

    if max_gate_delta != 0 or ner_disagreements:
        raise SystemExit("PARITY FAILED")
    print("PARITY OK")


if __name__ == "__main__":
    main()