"""LLAIM gate: decides whether the main NER should run on a chunk.

Reproduces the author's BIO decoding and post-processing, and additionally
computes a softmax-based gate_score = max over allowed types/tokens/windows of
P(B-type) + P(I-type). The gate opens if at least one allowed entity was found
OR gate_score >= threshold.
"""
from __future__ import annotations

import re
import time
from typing import Dict, List, Optional, Tuple

import numpy as np

from .labels import GATE_ALLOWED_TYPES, GATE_IGNORED_TYPES
from .onnx_model import OnnxTokenClassifier
from .windowing import llaim_windows

# Author's post-processing constants.
_ID_TYPES = {"INN", "OGRN", "BANK_ACCOUNT", "EMAIL", "PHONE", "SNILS"}
_ID_SEPARATORS = ",;|/"
_LINE_BREAKS = "\n\r\u2028\u2029"


class GateResult:
    """Result of running the gate on one chunk."""

    __slots__ = ("open", "score", "entities", "matched_types", "ignored_types", "ms")

    def __init__(
        self,
        open_: bool,
        score: float,
        entities: List[Dict],
        matched_types: List[str],
        ignored_types: List[str],
        ms: float,
    ) -> None:
        self.open = open_
        self.score = score
        self.entities = entities
        self.matched_types = matched_types
        self.ignored_types = ignored_types
        self.ms = ms


class LlaimGate:
    """Gate model wrapper (LLAIMlegal/ru-legal-ner, ONNX FP32)."""

    def __init__(
        self,
        model: OnnxTokenClassifier,
        max_chars: int = 900,
        max_length: int = 384,
        stride: int = 64,
        threshold: float = 0.001,
    ) -> None:
        self._model = model
        self._max_chars = max_chars
        self._max_length = max_length
        self._stride = stride
        self._threshold = threshold
        self._id2label = self._load_id2label()

    def _load_id2label(self) -> Dict[int, str]:
        # id2label is fixed by the model config; hardcode from config.json.
        return {
            0: "O",
            1: "B-PER",
            2: "I-PER",
            3: "B-ORG",
            4: "I-ORG",
            5: "B-ADDRESS",
            6: "I-ADDRESS",
            7: "B-INN",
            8: "I-INN",
            9: "B-OGRN",
            10: "I-OGRN",
            11: "B-SNILS",
            12: "I-SNILS",
            13: "B-PASSPORT",
            14: "I-PASSPORT",
            15: "B-PHONE",
            16: "I-PHONE",
            17: "B-EMAIL",
            18: "I-EMAIL",
            19: "B-CASE_NUMBER",
            20: "I-CASE_NUMBER",
            21: "B-BANK_ACCOUNT",
            22: "I-BANK_ACCOUNT",
            23: "B-DATE",
            24: "I-DATE",
            25: "B-POSITION",
            26: "I-POSITION",
        }

    def _label_type(self, label: str) -> str:
        """Return the entity type of a BIO label, or None for O."""
        if label == "O":
            return None
        return label[2:]

    def _greedy_spans(
        self, text: str, logits: np.ndarray, offset_mapping
    ) -> List[Dict]:
        """Author's greedy BIO decoding for a single window.

        A single I-label without a preceding B-label does not create an entity.
        offset_mapping is the raw tokenizer mapping; special/padding tokens are
        skipped in lockstep with the logits rows.
        """
        labels = [self._id2label[int(i)] for i in logits.argmax(-1)]
        spans: List[Dict] = []
        cur = None
        for (s, e), lab in zip(offset_mapping, labels):
            s = int(s)
            e = int(e)
            if s == e:
                continue
            if lab.startswith("B-"):
                if cur:
                    spans.append(cur)
                cur = {"start": s, "end": e, "label": lab[2:]}
            elif lab.startswith("I-") and cur and lab[2:] == cur["label"]:
                cur["end"] = e
            else:
                if cur:
                    spans.append(cur)
                cur = None
        if cur:
            spans.append(cur)
        return spans

    @staticmethod
    def _keep_period(text: str, s: int, e: int, label: str) -> bool:
        if label in _ID_TYPES or label in ("CASE_NUMBER", "PASSPORT"):
            return False
        m = re.search(r"([^\W\d_]+)$", text[s : e - 1])
        word = m.group(1) if m else ""
        if not word:
            return False
        if label == "PER":
            return len(word) == 1 and word.isupper()
        if label == "DATE":
            return word.lower() in ("г", "гг")
        return len(word) <= 3

    @staticmethod
    def _trim(text: str, s: int, e: int, label: str) -> Tuple[int, int]:
        if label != "ADDRESS":
            for k in range(s, e):
                if text[k] in _LINE_BREAKS:
                    e = k
                    break
        while e > s:
            c = text[e - 1]
            if c.isspace() or c in ",;:":
                e -= 1
            elif c == "." and not LlaimGate._keep_period(text, s, e, label):
                e -= 1
            elif c == ")" and "(" not in text[s:e]:
                e -= 1
            else:
                break
        while s < e and text[s].isspace():
            s += 1
        return s, e

    def _postprocess(self, text: str, spans: List[Dict]) -> List[Dict]:
        """Author's boundary post-processing."""
        spans = sorted(spans, key=lambda x: x["start"])
        out: List[Dict] = []
        for k, sp in enumerate(spans):
            s, e, label = sp["start"], sp["end"], sp["label"]
            if e < len(text) and not text[e - 1].isspace() and not text[e].isspace():
                j = e
                while j < len(text) and not text[j].isspace():
                    j += 1
                if label in _ID_TYPES:
                    for m in range(e, j):
                        if text[m] in _ID_SEPARATORS:
                            j = m
                            break
                if k + 1 < len(spans):
                    j = min(j, spans[k + 1]["start"])
                e = max(e, j)
            s, e = self._trim(text, s, e, label)
            if e - s >= 2:
                out.append({"start": s, "end": e, "label": label, "text": text[s:e]})
        return out

    def _gate_score(
        self, logits: np.ndarray, offset_mapping
    ) -> float:
        """Compute max over allowed types/tokens of P(B-type)+P(I-type).

        Only real tokens (non-padding, non-special) and only allowed types are
        considered. Ignored types never enter the maximum.
        """
        probs = np.exp(logits - logits.max(-1, keepdims=True))
        probs = probs / probs.sum(-1, keepdims=True)

        best = 0.0
        for tok_idx, (s, e) in enumerate(offset_mapping):
            s = int(s)
            e = int(e)
            if s == e:
                continue
            for native_type in GATE_ALLOWED_TYPES:
                b_id = self._label_id("B-" + native_type)
                i_id = self._label_id("I-" + native_type)
                p = 0.0
                if b_id is not None:
                    p += float(probs[tok_idx, b_id])
                if i_id is not None:
                    p += float(probs[tok_idx, i_id])
                if p > best:
                    best = p
        return best

    def _label_id(self, label: str) -> Optional[int]:
        for i, lab in self._id2label.items():
            if lab == label:
                return i
        return None

    def run(self, text: str) -> GateResult:
        """Run the gate on a chunk of text."""
        t0 = time.perf_counter()
        all_spans: List[Dict] = []
        matched_types: set = set()
        ignored_types: set = set()
        gate_score = 0.0

        for chunk_offset, win in llaim_windows(
            self._model.tokenizer,
            text,
            max_chars=self._max_chars,
            max_length=self._max_length,
            stride=self._stride,
        ):
            # win offsets are relative to the chunk; shift to the full text.
            win_start = chunk_offset + win.char_start
            win_end = chunk_offset + win.char_end
            window_text = text[win_start:win_end]
            enc = self._model.tokenize(
                window_text,
                return_offsets_mapping=True,
                truncation=True,
                max_length=self._max_length,
                return_tensors="np",
            )
            logits = self._model.run(enc)

            # offset_mapping is relative to window_text; shift to the chunk.
            offset_mapping = enc["offset_mapping"][0]
            spans = self._greedy_spans(window_text, logits[0], offset_mapping)
            for sp in spans:
                sp["start"] += win_start
                sp["end"] += win_start
            all_spans.extend(spans)

            for sp in spans:
                native = sp["label"]
                if native in GATE_ALLOWED_TYPES:
                    matched_types.add(native)
                elif native in GATE_IGNORED_TYPES:
                    ignored_types.add(native)

            gate_score = max(gate_score, self._gate_score(logits[0], offset_mapping))

        # Post-process spans over the whole chunk text.
        entities = self._postprocess(text, all_spans)
        for ent in entities:
            native = ent["label"]
            if native in GATE_ALLOWED_TYPES:
                matched_types.add(native)
            elif native in GATE_IGNORED_TYPES:
                ignored_types.add(native)

        gate_open = bool(matched_types) or gate_score >= self._threshold
        ms = (time.perf_counter() - t0) * 1000.0
        return GateResult(
            open_=gate_open,
            score=gate_score,
            entities=entities,
            matched_types=sorted(matched_types),
            ignored_types=sorted(ignored_types),
            ms=ms,
        )