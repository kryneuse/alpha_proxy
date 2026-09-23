"""Quality gate+NER and ungated spaCy with bounded adaptive chunk routing."""
from __future__ import annotations

import math
import threading
import time
from concurrent.futures import TimeoutError as FutureTimeout

from ..config import Config
from .adaptive import AdaptiveScheduler, Overloaded, RequestCancelled
from .types import BatchResult, ChunkAudit, ChunkResult, Entity
from .v14_repair import repair_structured_v3
from .v14_types import TYPES


class PIIDetector:
    def __init__(self, config: Config, *, gate=None, ner=None, spacy=None, warmup=True):
        self._config = config
        self._enabled_types = set(config.enabled_types)
        if not self._enabled_types <= set(TYPES):
            raise ValueError("Unknown enabled entity type")
        self._gate, self._ner, self._spacy = gate, ner, spacy
        if config.backend in {"adaptive", "quality"}:
            if gate is None or ner is None:
                from .onnx_model import LeanGate, LeanNer
                options = dict(intra_op_threads=config.intra_op_threads,
                               inter_op_threads=config.inter_op_threads,
                               tensor_batch_size=config.tensor_batch_size)
                self._gate = gate if gate is not None else LeanGate(config.gate.dir, **options)
                self._ner = ner if ner is not None else LeanNer(config.ner.dir, **options)
            if warmup:
                self._gate.score("Тестовый клиент: Иван Тестов.")
                self._ner.run("Тестовый клиент: Иван Тестов.")
        if config.backend in {"adaptive", "spacy_sm"} and spacy is None:
            from .spacy_ner import SpacyNerPool
            self._spacy = SpacyNerPool(config.spacy_dir, config.spacy_workers)
        self.scheduler = AdaptiveScheduler(config)

    @property
    def model_version(self):
        # Include mode even for a programmatically constructed Config.
        return f"{self._config.model_version};mode={self._config.backend}"

    def close(self):
        self.scheduler.close()

    def _detect(self, route, reason, queue_ms, chunk_id, original, gate_text, active):
        audit = ChunkAudit(chunk_id=chunk_id, backend=route,
                           route_reason=reason, queue_ms=queue_ms)
        if route == "quality":
            if not gate_text.strip():
                return ChunkResult(chunk_id=chunk_id, audit=audit)
            audit.gate_ran = True
            started = time.perf_counter()
            try:
                score = float(self._gate.score(gate_text))
                if not math.isfinite(score) or not 0 <= score <= 1:
                    raise ValueError("invalid gate score")
                audit.gate_score = score
                audit.gate_open = score >= self._config.gate_threshold
            except Exception as exc:
                # Error is not a clean decision. Keep the existing NER fallback.
                audit.gate_open = True
                audit.gate_error = type(exc).__name__
                audit.gate_error_fallback = True
            finally:
                audit.gate_ms = (time.perf_counter() - started) * 1000
            if not active():
                raise RequestCancelled()
            if not audit.gate_open:
                return ChunkResult(chunk_id=chunk_id, audit=audit)
            model = self._ner
        else:
            # The fast route never gates on residual, including blank residual.
            model = self._spacy
        if not active():
            raise RequestCancelled()
        started = time.perf_counter()
        try:
            raw = model.run(original)
            self._validate_entities(original, raw)
            fixed = repair_structured_v3(original, raw)
            self._validate_entities(original, fixed)
            entities = [Entity(type=e["type"], start=e["start"], end=e["end"], score=1.0)
                        for e in fixed if e["type"] in self._enabled_types]
            entities.sort(key=lambda e: (e.start, e.end, e.type))
            audit.matched_types = sorted({e.type for e in entities})
            return ChunkResult(chunk_id=chunk_id, entities=entities, audit=audit)
        except Exception as exc:
            # Do not silently downgrade failed quality inference to a clean result.
            audit.error = type(exc).__name__
            return ChunkResult(chunk_id=chunk_id, error_code="MODEL_ERROR", audit=audit)
        finally:
            audit.ner_ms = (time.perf_counter() - started) * 1000

    @staticmethod
    def _validate_entities(text, entities):
        for entity in entities:
            a, b = entity["start"], entity["end"]
            if (entity["type"] not in TYPES or type(a) is not int or type(b) is not int
                    or not 0 <= a < b <= len(text) or entity["text"] != text[a:b]):
                raise ValueError("invalid model entity")

    def detect_chunk_v2(self, chunk_id, original_text, gate_text):
        return self.detect_batch_v2("single", "unicode_code_points",
                                    [(chunk_id, original_text, gate_text)]).results[0]

    def detect_batch_v2(self, batch_id, offset_unit, chunks, *, active=None, timeout=None):
        if len(chunks) > self._config.max_batch_chunks:
            raise ValueError("batch exceeds max_batch_chunks")
        external_active = active or (lambda: True)
        cancelled = threading.Event()
        deadline = None if timeout is None else time.monotonic() + max(0.0, timeout)

        def is_active():
            return (not cancelled.is_set() and external_active()
                    and (deadline is None or time.monotonic() < deadline))

        pending = []
        try:
            for chunk_id, original, gate_text in chunks:
                # Bind values per task: concurrent requests never share chunk state.
                def run(route, reason, queue_ms, cid=chunk_id, text=original, gate=gate_text):
                    return self._detect(route, reason, queue_ms, cid, text, gate, is_active)
                pending.append(self.scheduler.submit(run, len(original), active=is_active, deadline=deadline))
            results = []
            for future in pending:
                while True:
                    if not is_active():
                        raise RequestCancelled()
                    try:
                        results.append(future.result(timeout=0.02))
                        break
                    except FutureTimeout:
                        continue
            if not is_active():
                raise RequestCancelled()
            return BatchResult(batch_id=batch_id, model_version=self.model_version,
                               offset_unit=offset_unit, results=results)
        except BaseException:
            # Do not future.cancel(): keep reservations until tasks are dequeued,
            # so repeated cancelled RPCs cannot grow the underlying work queues.
            cancelled.set()
            raise
