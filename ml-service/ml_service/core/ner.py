"""RedMadRobot main NER: entity extraction, normalization and filtering."""
from __future__ import annotations

import time
from typing import Dict, List, Optional, Tuple

import numpy as np

from .labels import (
    RMR_NAME_COMPONENTS,
    RMR_NATIVE_TO_TARGET,
    RMR_NON_TARGET_NATIVE,
)
from .onnx_model import OnnxTokenClassifier
from .types import Entity
from .windowing import rmr_windows


class NerResult:
    """Result of running the main NER on one chunk."""

    __slots__ = ("entities", "ms")

    def __init__(self, entities: List[Entity], ms: float) -> None:
        self.entities = entities
        self.ms = ms


class RedMadRobotNer:
    """Main NER model wrapper (redmadrobot-rnd/rubert-base-pii-ner, INT8)."""

    def __init__(
        self,
        model: OnnxTokenClassifier,
        max_length: int = 512,
        stride: int = 128,
        min_confidence: float = 0.0,
    ) -> None:
        self._model = model
        self._max_length = max_length
        self._stride = stride
        self._min_confidence = min_confidence
        self._id2label = self._load_id2label()

    def _load_id2label(self) -> Dict[int, str]:
        return {
            0: "O",
            1: "B-PASSPORT",
            2: "I-PASSPORT",
            3: "B-CREDIT_CARD",
            4: "I-CREDIT_CARD",
            5: "B-DRIVER_LICENSE",
            6: "I-DRIVER_LICENSE",
            7: "B-INN",
            8: "I-INN",
            9: "B-SNILS",
            10: "I-SNILS",
            11: "B-MILITARY_ID",
            12: "I-MILITARY_ID",
            13: "B-BIRTH_CERTIFICATE",
            14: "I-BIRTH_CERTIFICATE",
            15: "B-OMS",
            16: "I-OMS",
            17: "B-CITY",
            18: "B-COUNTRY",
            19: "B-DISTRICT",
            20: "B-EMAIL",
            21: "B-FIRST_NAME",
            22: "B-HOUSE",
            23: "B-IP_ADDRESS",
            24: "B-LAST_NAME",
            25: "B-MIDDLE_NAME",
            26: "B-PHONE",
            27: "B-REGION",
            28: "B-STREET",
            29: "B-URL",
            30: "I-CITY",
            31: "I-COUNTRY",
            32: "I-DISTRICT",
            33: "I-EMAIL",
            34: "I-FIRST_NAME",
            35: "I-HOUSE",
            36: "I-IP_ADDRESS",
            37: "I-LAST_NAME",
            38: "I-MIDDLE_NAME",
            39: "I-PHONE",
            40: "I-REGION",
            41: "I-STREET",
            42: "I-URL",
        }

    def _label_type(self, label: str) -> Optional[str]:
        if label == "O":
            return None
        return label[2:]

    def _decode_window(
        self, logits: np.ndarray, offset_mapping
    ) -> List[Dict]:
        """BIO decode a single window into raw spans with confidence.

        A single I-label without a preceding B-label does not create an entity.
        offset_mapping is the raw tokenizer mapping (may include special/padding
        tokens); those are skipped in lockstep with the logits rows.
        """
        probs = np.exp(logits - logits.max(-1, keepdims=True))
        probs = probs / probs.sum(-1, keepdims=True)
        labels = [self._id2label[int(i)] for i in logits.argmax(-1)]

        spans: List[Dict] = []
        cur = None
        for tok_idx, ((s, e), lab) in enumerate(zip(offset_mapping, labels)):
            s = int(s)
            e = int(e)
            if s == e:
                continue
            if lab.startswith("B-"):
                if cur:
                    spans.append(cur)
                cur = {
                    "start": s,
                    "end": e,
                    "label": lab[2:],
                    "score": float(probs[tok_idx].max()),
                }
            elif lab.startswith("I-") and cur and lab[2:] == cur["label"]:
                cur["end"] = e
                cur["score"] = max(cur["score"], float(probs[tok_idx].max()))
            else:
                if cur:
                    spans.append(cur)
                cur = None
        if cur:
            spans.append(cur)
        return spans

    def _merge_overlapping(self, spans: List[Dict], text: str = "") -> List[Dict]:
        """Merge overlapping/adjacent spans of the same native type.

        Handles the case where the model re-emits B- mid-entity (e.g. an email
        split into fragments, or a passport number split by a space). Only spans
        of the same native type are merged. When `text` is provided, spans
        separated only by whitespace are also merged.
        """
        if not spans:
            return []
        spans = sorted(spans, key=lambda x: (x["start"], x["end"]))
        merged: List[Dict] = []
        for sp in spans:
            if not merged:
                merged.append(sp)
                continue
            last = merged[-1]
            gap = sp["start"] - last["end"]
            adjacent = sp["start"] <= last["end"]
            if text and gap > 0 and text[last["end"] : sp["start"]].isspace():
                adjacent = True
            if last["label"] == sp["label"] and adjacent:
                last["end"] = max(last["end"], sp["end"])
                last["score"] = max(last["score"], sp["score"])
            else:
                merged.append(sp)
        return merged

    def _normalize_names(self, spans: List[Dict], text: str = "") -> List[Dict]:
        """Merge adjacent name components (FIRST/LAST/MIDDLE) into FULL_NAME.

        Name components that are adjacent (no gap) or separated only by
        whitespace are merged into a single FULL_NAME entity spanning from the
        first start to the last end.
        """
        name_spans = [s for s in spans if s["label"] in RMR_NAME_COMPONENTS]
        other_spans = [s for s in spans if s["label"] not in RMR_NAME_COMPONENTS]

        if not name_spans:
            return other_spans

        name_spans = sorted(name_spans, key=lambda x: (x["start"], x["end"]))
        merged_names: List[Dict] = []
        for sp in name_spans:
            if not merged_names:
                merged_names.append(
                    {
                        "start": sp["start"],
                        "end": sp["end"],
                        "label": "FULL_NAME",
                        "score": sp["score"],
                    }
                )
                continue
            last = merged_names[-1]
            gap = sp["start"] - last["end"]
            adjacent = sp["start"] <= last["end"]
            if text and gap > 0 and text[last["end"] : sp["start"]].isspace():
                adjacent = True
            if adjacent:
                last["end"] = max(last["end"], sp["end"])
                last["score"] = max(last["score"], sp["score"])
            else:
                merged_names.append(
                    {
                        "start": sp["start"],
                        "end": sp["end"],
                        "label": "FULL_NAME",
                        "score": sp["score"],
                    }
                )

        return other_spans + merged_names

    def run(self, text: str) -> NerResult:
        """Run the main NER on a chunk of text."""
        t0 = time.perf_counter()
        all_spans: List[Dict] = []

        for win in rmr_windows(
            self._model.tokenizer,
            text,
            max_length=self._max_length,
            stride=self._stride,
        ):
            window_text = text[win.char_start : win.char_end]
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
            spans = self._decode_window(logits[0], offset_mapping)
            for sp in spans:
                sp["start"] += win.char_start
                sp["end"] += win.char_start
            all_spans.extend(spans)

        # Merge overlapping spans of the same native type across windows.
        merged = self._merge_overlapping(all_spans, text)
        # Normalize name components into FULL_NAME.
        normalized = self._normalize_names(merged, text)

        entities: List[Entity] = []
        for sp in normalized:
            label = sp["label"]
            # Normalized spans may already carry a target type (FULL_NAME).
            if label in RMR_NATIVE_TO_TARGET:
                target = RMR_NATIVE_TO_TARGET[label]
            elif label == "FULL_NAME":
                target = "FULL_NAME"
            else:
                # Native label with no target mapping (e.g. SNILS, URL).
                continue
            if sp["score"] < self._min_confidence:
                continue
            entities.append(
                Entity(type=target, start=sp["start"], end=sp["end"], score=sp["score"])
            )

        entities.sort(key=lambda e: (e.start, e.end))
        ms = (time.perf_counter() - t0) * 1000.0
        return NerResult(entities=entities, ms=ms)