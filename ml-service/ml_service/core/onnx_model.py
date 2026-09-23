"""ONNX Runtime inference for the gate1-v3 gate and the v14a NER.

Both graphs use the `tokenizers` library (not transformers AutoTokenizer) and
their own tokenizer.json. Inputs are int64 [batch, sequence]: input_ids,
attention_mask, token_type_ids. The gate returns gate_logits float [batch]; the
NER returns ner_logits float [batch, sequence, 25, 3] (last axis O/B/I).

This mirrors the reference LeanModel / LeanOnnxGate runtime: dynamic length
without padding up to the window length, all overflowing windows, and float32
sigmoid for the gate.
"""
from __future__ import annotations

import json
import threading
from pathlib import Path
from typing import Dict, List

# Must run before onnxruntime initializes its thread pools.
from . import threading_setup  # noqa: F401

import numpy as np
import onnxruntime as ort
from tokenizers import Tokenizer

from .v14_types import TYPE_ID, TYPES


def _dedup(entities: List[Dict]) -> List[Dict]:
    return list({(e["type"], e["start"], e["end"]): e for e in entities}.values())


class _BaseModel:
    """Shared ONNX session + tokenizer setup for a single artifact."""

    def __init__(
        self,
        artifact_dir: str,
        intra_op_threads: int = 1,
        inter_op_threads: int = 1,
        tensor_batch_size: int = 1,
    ) -> None:
        path = Path(artifact_dir)
        self.meta = json.loads((path / "model.json").read_text())
        self.tensor_batch_size = tensor_batch_size

        length = self.meta.get("args", {}).get("length", 256)
        self.max_length = length
        self.stride = min(64, length // 4)

        self.tokenizer = Tokenizer.from_file(str(path / "tokenizer.json"))
        self.tokenizer.no_padding()
        self.tokenizer.enable_truncation(max_length=length, stride=self.stride)

        options = ort.SessionOptions()
        options.intra_op_num_threads = intra_op_threads
        options.inter_op_num_threads = inter_op_threads
        options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
        options.add_session_config_entry("session.intra_op.allow_spinning", "0")
        options.add_session_config_entry("session.inter_op.allow_spinning", "0")
        self.session = ort.InferenceSession(
            str(path / "model.int8.onnx"),
            sess_options=options,
            providers=["CPUExecutionProvider"],
        )
        self.input_names = {x.name for x in self.session.get_inputs()}

    def _feeds(self, parts: List) -> Dict[str, np.ndarray]:
        """Build int64 feeds for a mini-batch of tokenizer encodings."""
        length = max(len(e.ids) for e in parts)
        arrays = {
            "input_ids": np.zeros((len(parts), length), dtype=np.int64),
            "attention_mask": np.zeros((len(parts), length), dtype=np.int64),
            "token_type_ids": np.zeros((len(parts), length), dtype=np.int64),
        }
        for j, e in enumerate(parts):
            n = len(e.ids)
            arrays["input_ids"][j, :n] = e.ids
            arrays["attention_mask"][j, :n] = e.attention_mask
            arrays["token_type_ids"][j, :n] = e.type_ids
        return {k: v for k, v in arrays.items() if k in self.input_names}


class LeanGate(_BaseModel):
    """Separate trained gate1-v3. Returns a score per text (max over windows)."""

    def __init__(self, artifact_dir: str, **kwargs) -> None:
        super().__init__(artifact_dir, **kwargs)
        outputs = {x.name for x in self.session.get_outputs()}
        if "gate_logits" not in outputs:
            raise ValueError("ONNX gate model has no gate_logits output")

    def scores(self, texts: List[str]) -> List[float]:
        if not texts:
            return []
        if not all(isinstance(t, str) for t in texts):
            raise ValueError("texts must contain only strings")
        scores = np.zeros(len(texts), dtype=np.float32)
        items: List = []
        for index, encoding in enumerate(self.tokenizer.encode_batch(texts)):
            items.append((index, encoding))
            items.extend((index, part) for part in encoding.overflowing)
        for start in range(0, len(items), self.tensor_batch_size):
            batch = items[start : start + self.tensor_batch_size]
            feeds = self._feeds([e for _, e in batch])
            logits = self.session.run(["gate_logits"], feeds)[0]
            if logits.shape != (len(batch),) or not np.isfinite(logits).all():
                raise ValueError("invalid gate logits; text must not be treated as clean")
            values = 1 / (1 + np.exp(-np.clip(logits, -80, 80)))
            for (index, _e), score in zip(batch, values):
                scores[index] = max(scores[index], score)
        return scores.tolist()

    def score(self, text: str) -> float:
        return self.scores([text])[0]


class LeanNer(_BaseModel):
    """v14a NER. Returns entities with char offsets in the input text."""

    def __init__(self, artifact_dir: str, **kwargs) -> None:
        super().__init__(artifact_dir, **kwargs)
        outputs = {x.name for x in self.session.get_outputs()}
        if "ner_logits" not in outputs:
            raise ValueError("ONNX NER model has no ner_logits output")
        # Validate the head shape on startup: [batch, seq, 25, 3].
        shape = self.session.get_outputs()[0].shape
        if len(shape) != 4 or shape[2] != len(TYPES) or shape[3] != 3:
            raise ValueError(f"unexpected ner_logits shape {shape}; expected [batch,seq,{len(TYPES)},3]")

    def run(self, text: str) -> List[Dict]:
        encoding = self.tokenizer.encode(text)
        es: List[Dict] = []
        for part in [encoding] + encoding.overflowing:
            feeds = self._feeds([part])
            logits = self.session.run(["ner_logits"], feeds)[0]
            if not np.isfinite(logits).all():
                raise ValueError("non-finite NER output")
            tags = logits[0].argmax(-1)  # [seq, 25]
            for typ in TYPES:
                current = None
                c = TYPE_ID[typ]
                for j, (a, b) in enumerate(part.offsets):
                    tag = tags[j, c]
                    if b <= a or tag == 0:
                        current = None
                        continue
                    # I without an open span starts a new entity (reference rule).
                    if tag == 1 or current is None:
                        current = dict(type=typ, start=a, end=b)
                        es.append(current)
                    else:
                        current["end"] = b
        result: List[Dict] = []
        for typ in TYPES:
            for e in sorted(
                (e for e in _dedup(es) if e["type"] == typ),
                key=lambda e: (e["start"], e["end"]),
            ):
                if result and result[-1]["type"] == typ and e["start"] < result[-1]["end"]:
                    result[-1]["end"] = max(result[-1]["end"], e["end"])
                else:
                    result.append(e.copy())
        for e in result:
            e["text"] = text[e["start"] : e["end"]]
        return result