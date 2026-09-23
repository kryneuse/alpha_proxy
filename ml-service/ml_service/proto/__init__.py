"""Generated gRPC/protobuf modules for the PII detector contract (V2).

Generated from the single source api/ml/v1/pii.proto via scripts/gen_proto.sh.
The generated code uses top-level imports (``import pii_pb2``), so we expose the
proto directory on ``sys.path`` and re-export the modules here.
"""
from __future__ import annotations

import os
import sys

_PROTO_DIR = os.path.dirname(os.path.abspath(__file__))
if _PROTO_DIR not in sys.path:
    sys.path.insert(0, _PROTO_DIR)

from pii_pb2 import (  # noqa: E402,F401
    Chunk,
    ChunkErrorCode,
    ChunkResult,
    ChunkV2,
    DetectBatchRequest,
    DetectBatchResponse,
    DetectBatchV2Request,
    Entity,
    EntityType,
    OffsetUnit,
)
from pii_pb2_grpc import (  # noqa: E402,F401
    PIIDetectorServicer,
    PIIDetectorStub,
    add_PIIDetectorServicer_to_server,
)

__all__ = [
    "Chunk",
    "ChunkErrorCode",
    "ChunkResult",
    "ChunkV2",
    "DetectBatchRequest",
    "DetectBatchResponse",
    "DetectBatchV2Request",
    "Entity",
    "EntityType",
    "OffsetUnit",
    "PIIDetectorServicer",
    "PIIDetectorStub",
    "add_PIIDetectorServicer_to_server",
]