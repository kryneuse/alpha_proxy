"""Runtime settings for quality, spaCy and adaptive PII inference."""
from __future__ import annotations

import os
import math
from dataclasses import dataclass, field
from typing import List

# Default paths relative to the ml-service package root.
_PKG_ROOT = os.path.dirname(os.path.abspath(__file__))
_DEFAULT_MODELS_DIR = os.path.join(os.path.dirname(_PKG_ROOT), "models")

# Selected cascade threshold (separate trained gate1-v3). Do not take the
# threshold from model.json: the gate artifact stores a different value, and the
# internal gate output of the NER graph is not our cascade gate.
DEFAULT_GATE_THRESHOLD = 1.823714370630114e-7


@dataclass(frozen=True)
class ModelConfig:
    """Paths and runtime settings for a single ONNX artifact.

    Each artifact directory contains model.int8.onnx, model.json, tokenizer.json,
    tokenizer_config.json, special_tokens_map.json, vocab.txt and export.json.
    """

    dir: str
    # Token window length and stride (both artifacts use 256 / 64).
    max_length: int = 256
    stride: int = 64


@dataclass(frozen=True)
class Config:
    """Top-level service configuration."""

    # Adaptive admission is based on bounded chunk work, not request RPS.
    backend: str = "adaptive"  # adaptive | quality | spacy_sm
    quality_workers: int = 2
    spacy_workers: int = 2
    quality_queue: int = 2
    spacy_queue: int = 64
    routing_budget_ms: float = 100.0
    quality_initial_ms: float = 40.0
    quality_min_ms: float = 10.0
    spacy_initial_ms: float = 3.0
    recovery_hold_ms: float = 250.0
    quality_recovery_pending: int = 0
    recovery_ratio: float = 0.7
    grpc_max_concurrent_rpcs: int = 32
    max_batch_chunks: int = 32
    metrics_host: str = "127.0.0.1"
    metrics_port: int = 9091  # 0 disables the metrics HTTP listener
    spacy_dir: str = os.path.join(_DEFAULT_MODELS_DIR, "ru_core_news_sm_pii_v1")

    # Separate trained gate threshold. Comparison is score >= threshold.
    gate_threshold: float = DEFAULT_GATE_THRESHOLD

    # Parallelism budget: 4 workers x 1 compute thread, tensor batch size 1.
    num_workers: int = 4
    intra_op_threads: int = 1
    inter_op_threads: int = 1
    # Tensor batch size for both graphs. Start at 1 (matches the reference path).
    tensor_batch_size: int = 1

    # gRPC server settings.
    grpc_host: str = "127.0.0.1"
    grpc_port: int = 50051
    grpc_max_workers: int = 16
    grpc_max_message_length: int = 64 * 1024 * 1024

    # Model version string reported in DetectBatchResponse. It must account for
    # both model hashes, tokenizers, threshold, decoder/repair and contract.
    model_version: str = "v14a-gate1-v3-spacy-sm-v1-adaptive-v1"

    # Target entity types applied to final entities (all 25 NER types).
    enabled_types: List[str] = field(
        default_factory=lambda: [
            "FULL_NAME",
            "DATE_OF_BIRTH",
            "BIRTH_PLACE",
            "PASSPORT",
            "CITIZENSHIP",
            "PASSPORT_ISSUER",
            "DEPARTMENT_CODE",
            "PASSPORT_ISSUE_DATE",
            "DRIVER_LICENSE",
            "ADDRESS",
            "COUNTRY",
            "POSTCODE",
            "CITY",
            "STREET",
            "HOUSE",
            "APARTMENT",
            "EMAIL",
            "PHONE",
            "INN",
            "CARD_NUMBER",
            "CVV",
            "PIN",
            "CARDHOLDER_NAME",
            "REGION",
            "DISTRICT",
        ]
    )

    # Maximum chunk original_text length in Unicode code points. Backend default
    # is 350; we enforce a hard cap to protect the service.
    max_chunk_codepoints: int = 350
    # Maximum original_text byte length: 350 code points can be up to 4 bytes
    # each (emoji), so up to 1400 bytes. gate_text has the same byte length.
    max_original_bytes: int = 1400

    ner: ModelConfig = field(
        default_factory=lambda: ModelConfig(
            dir=os.path.join(_DEFAULT_MODELS_DIR, "source99-expansion-v14a-epoch1-onnx"),
            max_length=256,
            stride=64,
        )
    )

    gate: ModelConfig = field(
        default_factory=lambda: ModelConfig(
            dir=os.path.join(_DEFAULT_MODELS_DIR, "gate1-v3-onnx"),
            max_length=256,
            stride=64,
        )
    )

    def __post_init__(self):
        if self.backend not in {"adaptive", "quality", "spacy_sm"}:
            raise ValueError("ML_BACKEND must be adaptive, quality or spacy_sm")
        for name in ("quality_workers", "spacy_workers", "grpc_max_workers", "grpc_max_concurrent_rpcs", "max_batch_chunks", "max_chunk_codepoints", "max_original_bytes", "intra_op_threads", "inter_op_threads", "tensor_batch_size"):
            if getattr(self, name) <= 0:
                raise ValueError(f"{name} must be positive")
        for name in ("quality_queue", "spacy_queue", "quality_recovery_pending"):
            if getattr(self, name) < 0:
                raise ValueError(f"{name} must be non-negative")
        for name in ("routing_budget_ms", "quality_initial_ms", "quality_min_ms", "spacy_initial_ms", "recovery_hold_ms"):
            value = getattr(self, name)
            if not math.isfinite(value) or value <= 0:
                raise ValueError(f"{name} must be finite and positive")
        if not 0 < self.recovery_ratio < 1:
            raise ValueError("recovery_ratio must be between zero and one")
        if self.quality_recovery_pending >= self.quality_workers + self.quality_queue:
            raise ValueError("recovery watermark must be below quality capacity")
        if not 0 <= self.metrics_port <= 65535:
            raise ValueError("metrics_port must be 0..65535")
        if self.backend == "adaptive" and self.grpc_max_workers <= self.quality_workers:
            raise ValueError("adaptive mode needs spare gRPC workers for the fast path")
        if not math.isfinite(self.gate_threshold) or not 0 <= self.gate_threshold <= 1:
            raise ValueError("gate_threshold must be in [0, 1]")

    @classmethod
    def from_env(cls) -> "Config":
        """Build a Config from environment variables (optional overrides)."""
        models_dir = os.environ.get("ML_MODELS_DIR", _DEFAULT_MODELS_DIR)
        ner_dir = os.environ.get("ML_NER_DIR", os.path.join(models_dir, "source99-expansion-v14a-epoch1-onnx"))
        gate_dir = os.environ.get("ML_GATE_DIR", os.path.join(models_dir, "gate1-v3-onnx"))
        backend = os.environ.get("ML_BACKEND", "adaptive")
        return cls(
            backend=backend,
            quality_workers=int(os.environ.get("ML_QUALITY_WORKERS", "2")),
            spacy_workers=int(os.environ.get("ML_SPACY_WORKERS", "2")),
            quality_queue=int(os.environ.get("ML_QUALITY_QUEUE", "2")),
            spacy_queue=int(os.environ.get("ML_SPACY_QUEUE", "64")),
            routing_budget_ms=float(os.environ.get("ML_ROUTING_BUDGET_MS", "100")),
            quality_initial_ms=float(os.environ.get("ML_QUALITY_INITIAL_MS", "40")),
            quality_min_ms=float(os.environ.get("ML_QUALITY_MIN_MS", "10")),
            spacy_initial_ms=float(os.environ.get("ML_SPACY_INITIAL_MS", "3")),
            recovery_hold_ms=float(os.environ.get("ML_RECOVERY_HOLD_MS", "250")),
            quality_recovery_pending=int(os.environ.get("ML_QUALITY_RECOVERY_PENDING", "0")),
            recovery_ratio=float(os.environ.get("ML_RECOVERY_RATIO", "0.7")),
            grpc_max_concurrent_rpcs=int(os.environ.get("ML_GRPC_MAX_CONCURRENT_RPCS", "32")),
            max_batch_chunks=int(os.environ.get("ML_MAX_BATCH_CHUNKS", "32")),
            metrics_host=os.environ.get("ML_METRICS_HOST", "127.0.0.1"),
            metrics_port=int(os.environ.get("ML_METRICS_PORT", "9091")),
            spacy_dir=os.environ.get("ML_SPACY_DIR", os.path.join(models_dir, "ru_core_news_sm_pii_v1")),
            gate_threshold=float(os.environ.get("ML_GATE_THRESHOLD", str(DEFAULT_GATE_THRESHOLD))),
            num_workers=int(os.environ.get("ML_NUM_WORKERS", "4")),
            intra_op_threads=int(os.environ.get("ML_INTRA_OP_THREADS", "1")),
            inter_op_threads=int(os.environ.get("ML_INTER_OP_THREADS", "1")),
            tensor_batch_size=int(os.environ.get("ML_TENSOR_BATCH_SIZE", "1")),
            grpc_host=os.environ.get("ML_GRPC_HOST", "127.0.0.1"),
            grpc_port=int(os.environ.get("ML_GRPC_PORT", "50051")),
            grpc_max_workers=int(os.environ.get("ML_GRPC_MAX_WORKERS", "16")),
            model_version=os.environ.get("ML_MODEL_VERSION", f"v14a-gate1-v3-spacy-sm-v1-{backend}-v1"),
            max_chunk_codepoints=int(os.environ.get("ML_MAX_CHUNK_CODEPOINTS", "350")),
            max_original_bytes=int(os.environ.get("ML_MAX_ORIGINAL_BYTES", "1400")),
            ner=ModelConfig(dir=ner_dir, max_length=256, stride=64),
            gate=ModelConfig(dir=gate_dir, max_length=256, stride=64),
        )