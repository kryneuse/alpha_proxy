"""gRPC service implementation for the PIIDetector V2 contract."""
from __future__ import annotations

import grpc

from ..config import Config
from ..core.detector import PIIDetector
from ..core.adaptive import Overloaded, RequestCancelled
from ..core.types import BatchResult, ChunkResult
from .. import proto as pb

# Map internal target entity types to proto EntityType enum values.
# For V2 we do NOT collapse both dates into DATE and do NOT emit
# COUNTRY/REGION/DISTRICT as ADDRESS on the transport level.
_TARGET_TO_PROTO = {
    "FULL_NAME": pb.EntityType.ENTITY_TYPE_FULL_NAME,
    "FIRST_NAME": pb.EntityType.ENTITY_TYPE_FIRST_NAME,
    "LAST_NAME": pb.EntityType.ENTITY_TYPE_LAST_NAME,
    "PATRONYMIC": pb.EntityType.ENTITY_TYPE_PATRONYMIC,
    "ADDRESS": pb.EntityType.ENTITY_TYPE_ADDRESS,
    "CITY": pb.EntityType.ENTITY_TYPE_CITY,
    "STREET": pb.EntityType.ENTITY_TYPE_STREET,
    "HOUSE": pb.EntityType.ENTITY_TYPE_HOUSE,
    "APARTMENT": pb.EntityType.ENTITY_TYPE_APARTMENT,
    "BIRTH_PLACE": pb.EntityType.ENTITY_TYPE_PLACE_OF_BIRTH,
    "CITIZENSHIP": pb.EntityType.ENTITY_TYPE_CITIZENSHIP,
    "PASSPORT_ISSUER": pb.EntityType.ENTITY_TYPE_PASSPORT_ISSUER,
    "CARDHOLDER_NAME": pb.EntityType.ENTITY_TYPE_CARDHOLDER_NAME,
    "EMAIL": pb.EntityType.ENTITY_TYPE_EMAIL,
    "PHONE": pb.EntityType.ENTITY_TYPE_PHONE,
    "INN": pb.EntityType.ENTITY_TYPE_INN,
    "CARD_NUMBER": pb.EntityType.ENTITY_TYPE_CARD,
    "PASSPORT": pb.EntityType.ENTITY_TYPE_PASSPORT,
    "DEPARTMENT_CODE": pb.EntityType.ENTITY_TYPE_DEPARTMENT_CODE,
    "DRIVER_LICENSE": pb.EntityType.ENTITY_TYPE_DRIVER_LICENSE,
    "CVV": pb.EntityType.ENTITY_TYPE_CVV,
    "PIN": pb.EntityType.ENTITY_TYPE_PIN,
    "POSTCODE": pb.EntityType.ENTITY_TYPE_POSTAL_CODE,
    "COUNTRY": pb.EntityType.ENTITY_TYPE_COUNTRY,
    "REGION": pb.EntityType.ENTITY_TYPE_REGION,
    "DISTRICT": pb.EntityType.ENTITY_TYPE_DISTRICT,
    "DATE_OF_BIRTH": pb.EntityType.ENTITY_TYPE_DATE_OF_BIRTH,
    "PASSPORT_ISSUE_DATE": pb.EntityType.ENTITY_TYPE_PASSPORT_ISSUE_DATE,
}

_ERROR_CODE_MAP = {
    "NONE": pb.ChunkErrorCode.CHUNK_ERROR_CODE_NONE,
    "INVALID_TEXT": pb.ChunkErrorCode.CHUNK_ERROR_CODE_INVALID_TEXT,
    "TOO_LARGE": pb.ChunkErrorCode.CHUNK_ERROR_CODE_TOO_LARGE,
    "MODEL_ERROR": pb.ChunkErrorCode.CHUNK_ERROR_CODE_MODEL_ERROR,
}


class PIIDetectorService(pb.PIIDetectorServicer):
    """gRPC servicer delegating to the core PIIDetector (V2 only)."""

    def __init__(self, detector: PIIDetector, config: Config) -> None:
        self._detector = detector
        self._config = config

    def DetectBatch(self, request, context):
        # The new configuration serves V2 only. Reject the old RPC explicitly.
        context.abort(
            grpc.StatusCode.FAILED_PRECONDITION,
            "DetectBatch is not supported; use DetectBatchV2",
        )
        return

    def DetectBatchV2(self, request, context):
        try:
            self._validate_request(request)
        except ValueError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
            return

        offset_unit = request.offset_unit
        if offset_unit != pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS:
            context.abort(
                grpc.StatusCode.INVALID_ARGUMENT,
                "unsupported offset_unit; only OFFSET_UNIT_UNICODE_CODE_POINTS is supported",
            )
            return

        chunks = [
            (c.chunk_id, c.original_text, c.gate_text)
            for c in request.chunks
        ]
        try:
            batch: BatchResult = self._detector.detect_batch_v2(
                request.batch_id, "unicode_code_points", chunks,
                active=context.is_active, timeout=context.time_remaining(),
            )
        except Overloaded:
            context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "ML inference capacity exhausted")
        except RequestCancelled:
            remaining = context.time_remaining()
            code = grpc.StatusCode.DEADLINE_EXCEEDED if remaining is not None and remaining <= 0 else grpc.StatusCode.CANCELLED
            context.abort(code, "ML request cancelled or deadline exceeded")
        counts = {"quality": 0, "spacy_sm": 0}
        for result in batch.results:
            if result.audit and result.audit.backend in counts:
                counts[result.audit.backend] += 1
        context.set_trailing_metadata((
            ("x-ml-quality-chunks", str(counts["quality"])),
            ("x-ml-spacy-chunks", str(counts["spacy_sm"])),
        ))

        response = pb.DetectBatchResponse(
            batch_id=batch.batch_id,
            model_version=batch.model_version,
            offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
        )
        for res in batch.results:
            response.results.append(self._to_proto_result(res))
        return response

    def _validate_request(self, request) -> None:
        """Validate the request; raises ValueError with a message on failure."""
        if not request.batch_id:
            raise ValueError("batch_id must be non-empty")
        if not request.chunks:
            raise ValueError("chunks must contain at least one item")
        if len(request.chunks) > self._config.max_batch_chunks:
            raise ValueError("too many chunks in batch")
        seen = set()
        for chunk in request.chunks:
            if not chunk.chunk_id:
                raise ValueError("chunk_id must be non-empty")
            if chunk.chunk_id in seen:
                raise ValueError("duplicate chunk_id")
            seen.add(chunk.chunk_id)

            if not chunk.HasField("gate_text"):
                raise ValueError("gate_text is required")
            if not chunk.original_text:
                raise ValueError("original_text must not be empty")

            orig_bytes = len(chunk.original_text.encode("utf-8"))
            gate_bytes = len(chunk.gate_text.encode("utf-8"))
            if orig_bytes != gate_bytes:
                raise ValueError(
                    "original_text and gate_text byte lengths differ"
                )
            if orig_bytes > self._config.max_original_bytes:
                raise ValueError(
                    "original_text exceeds max byte length"
                )
            if len(chunk.original_text) > self._config.max_chunk_codepoints:
                raise ValueError(
                    "original_text exceeds max code points"
                )

    def _to_proto_result(self, res: ChunkResult) -> pb.ChunkResult:
        proto_res = pb.ChunkResult(
            chunk_id=res.chunk_id,
            error_code=_ERROR_CODE_MAP.get(
                res.error_code, pb.ChunkErrorCode.CHUNK_ERROR_CODE_MODEL_ERROR
            ),
        )
        for ent in res.entities:
            proto_type = _TARGET_TO_PROTO.get(ent.type)
            if proto_type is None:
                # Unknown types are never silently lost at the transport boundary.
                return pb.ChunkResult(chunk_id=res.chunk_id,
                    error_code=pb.ChunkErrorCode.CHUNK_ERROR_CODE_MODEL_ERROR)
            proto_res.entities.append(
                pb.Entity(
                    type=proto_type,
                    start=ent.start,
                    end=ent.end,
                    confidence=float(ent.score),
                )
            )
        return proto_res