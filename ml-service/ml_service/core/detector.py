"""PII detector: gate + main NER + normalization + target-type filter."""
from __future__ import annotations

from typing import List, Optional, Set

from ..config import Config
from .gate import LlaimGate
from .ner import RedMadRobotNer
from .onnx_model import OnnxTokenClassifier
from .types import BatchResult, ChunkAudit, ChunkResult, Entity


class PIIDetector:
    """High-level detector combining the LLAIM gate and RedMadRobot NER.

    Models are loaded once and shared across parallel calls. The tokenizers are
    thread-safe via internal locks.
    """

    def __init__(self, config: Config) -> None:
        self._config = config
        self._enabled_types: Set[str] = set(config.enabled_types)

        self._gate_model = OnnxTokenClassifier(
            config.llaim.onnx_path,
            config.llaim.tokenizer_dir,
            intra_op_threads=config.intra_op_threads,
            inter_op_threads=config.inter_op_threads,
        )
        self._ner_model = OnnxTokenClassifier(
            config.redmadrobot.onnx_path,
            config.redmadrobot.tokenizer_dir,
            intra_op_threads=config.intra_op_threads,
            inter_op_threads=config.inter_op_threads,
        )

        self._gate = LlaimGate(
            self._gate_model,
            max_chars=config.llaim.max_chars,
            max_length=config.llaim.max_length,
            stride=config.llaim.stride,
            threshold=config.gate_threshold,
        )
        self._ner = RedMadRobotNer(
            self._ner_model,
            max_length=config.redmadrobot.max_length,
            stride=config.redmadrobot.stride,
        )

    @property
    def model_version(self) -> str:
        return self._config.model_version

    def _filter_entities(self, entities: List[Entity]) -> List[Entity]:
        """Apply the enabled_types filter to final entities."""
        return [e for e in entities if e.type in self._enabled_types]

    def detect_chunk(self, chunk_id: str, text: str) -> ChunkResult:
        """Process a single chunk through gate -> NER -> filter."""
        audit = ChunkAudit(chunk_id=chunk_id)

        gate = self._gate.run(text)
        audit.gate_open = gate.open
        audit.gate_score = gate.score
        audit.gate_ms = gate.ms
        audit.matched_types = gate.matched_types
        audit.ignored_types = gate.ignored_types

        if not gate.open:
            audit.ner_ms = 0.0
            return ChunkResult(chunk_id=chunk_id, entities=[], error_code="NONE", audit=audit)

        ner = self._ner.run(text)
        audit.ner_ms = ner.ms

        entities = self._filter_entities(ner.entities)
        return ChunkResult(chunk_id=chunk_id, entities=entities, error_code="NONE", audit=audit)

    def detect_batch(
        self,
        batch_id: str,
        offset_unit: str,
        chunks: List[tuple],
    ) -> BatchResult:
        """Process a batch of (chunk_id, text) pairs sequentially.

        Each chunk is independent; a failure in one chunk is reported via its
        error_code and does not affect the others.
        """
        results: List[ChunkResult] = []
        for chunk_id, text in chunks:
            try:
                results.append(self.detect_chunk(chunk_id, text))
            except Exception as exc:  # noqa: BLE001
                results.append(
                    ChunkResult(
                        chunk_id=chunk_id,
                        entities=[],
                        error_code="MODEL_ERROR",
                        audit=ChunkAudit(chunk_id=chunk_id, error=str(exc)),
                    )
                )
        return BatchResult(
            batch_id=batch_id,
            model_version=self.model_version,
            offset_unit=offset_unit,
            results=results,
        )