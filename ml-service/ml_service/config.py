"""Configuration for the ML PII detection service."""
from __future__ import annotations

import os
from dataclasses import dataclass, field
from typing import List

# Default paths relative to the ml-service package root.
_PKG_ROOT = os.path.dirname(os.path.abspath(__file__))
_DEFAULT_MODELS_DIR = os.path.join(os.path.dirname(_PKG_ROOT), "models")


@dataclass(frozen=True)
class ModelConfig:
    """Paths and runtime settings for a single ONNX model."""

    onnx_path: str
    tokenizer_dir: str
    max_length: int
    stride: int
    # Window size in characters used by the author's chunking (LLAIM only).
    max_chars: int = 900


@dataclass(frozen=True)
class Config:
    """Top-level service configuration."""

    # Gate threshold. Heuristic routing score, not a calibrated probability.
    gate_threshold: float = 0.001

    # Parallelism budget: 4 workers x 1 compute thread, batch_size=1.
    num_workers: int = 4
    intra_op_threads: int = 1
    inter_op_threads: int = 1
    batch_size: int = 1

    # gRPC server settings.
    grpc_port: int = 50051
    grpc_max_workers: int = 4
    grpc_max_message_length: int = 64 * 1024 * 1024

    # Model version string reported in DetectBatchResponse.
    model_version: str = "1.0.0"

    # Target entity types applied to final entities. Does not change the fixed
    # gate type list.
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
        ]
    )

    # Maximum chunk text length in Unicode code points. Backend default is 350,
    # but we enforce a hard cap to protect the service.
    max_chunk_chars: int = 100_000

    llaim: ModelConfig = field(
        default_factory=lambda: ModelConfig(
            onnx_path=os.path.join(_DEFAULT_MODELS_DIR, "llaim-ru-legal-ner.onnx"),
            tokenizer_dir=os.path.join(_DEFAULT_MODELS_DIR, "llaim-ru-legal-ner"),
            max_length=384,
            stride=64,
            max_chars=900,
        )
    )

    redmadrobot: ModelConfig = field(
        default_factory=lambda: ModelConfig(
            onnx_path=os.path.join(
                _DEFAULT_MODELS_DIR, "redmadrobot-rubert-pii-ner-int8.onnx"
            ),
            tokenizer_dir=os.path.join(
                _DEFAULT_MODELS_DIR, "redmadrobot-rubert-pii-ner"
            ),
            max_length=512,
            stride=128,
        )
    )

    @classmethod
    def from_env(cls) -> "Config":
        """Build a Config from environment variables (optional overrides)."""
        models_dir = os.environ.get("ML_MODELS_DIR", _DEFAULT_MODELS_DIR)
        return cls(
            gate_threshold=float(os.environ.get("ML_GATE_THRESHOLD", "0.001")),
            num_workers=int(os.environ.get("ML_NUM_WORKERS", "4")),
            intra_op_threads=int(os.environ.get("ML_INTRA_OP_THREADS", "1")),
            inter_op_threads=int(os.environ.get("ML_INTER_OP_THREADS", "1")),
            grpc_port=int(os.environ.get("ML_GRPC_PORT", "50051")),
            model_version=os.environ.get("ML_MODEL_VERSION", "1.0.0"),
            llaim=ModelConfig(
                onnx_path=os.path.join(models_dir, "llaim-ru-legal-ner.onnx"),
                tokenizer_dir=os.path.join(models_dir, "llaim-ru-legal-ner"),
                max_length=384,
                stride=64,
                max_chars=900,
            ),
            redmadrobot=ModelConfig(
                onnx_path=os.path.join(
                    models_dir, "redmadrobot-rubert-pii-ner-int8.onnx"
                ),
                tokenizer_dir=os.path.join(models_dir, "redmadrobot-rubert-pii-ner"),
                max_length=512,
                stride=128,
            ),
        )