"""gRPC service implementation for the PIIDetector contract."""
from __future__ import annotations

import grpc

from ..core.detector import PIIDetector
from ..core.types import BatchResult, ChunkResult
from .. import proto as pb

# Map internal target entity types to proto EntityType enum values.
# Types present in the target list but absent from the proto enum (e.g. COUNTRY)
# are not emitted.
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
    "DATE": pb.EntityType.ENTITY_TYPE_DATE,
    "DATE_OF_BIRTH": pb.EntityType.ENTITY_TYPE_DATE,
    "PASSPORT_ISSUE_DATE": pb.EntityType.ENTITY_TYPE_DATE,
    "DRIVER_LICENSE": pb.EntityType.ENTITY_TYPE_DRIVER_LICENSE,
    "CVV": pb.EntityType.ENTITY_TYPE_CVV,
    "PIN": pb.EntityType.ENTITY_TYPE_PIN,
    "POSTCODE": pb.EntityType.ENTITY_TYPE_POSTAL_CODE,
}

_ERROR_CODE_MAP = {
    "NONE": pb.ChunkErrorCode.CHUNK_ERROR_CODE_NONE,
    "INVALID_TEXT": pb.ChunkErrorCode.CHUNK_ERROR_CODE_INVALID_TEXT,
    "TOO_LARGE": pb.ChunkErrorCode.CHUNK_ERROR_CODE_TOO_LARGE,
    "MODEL_ERROR": pb.ChunkErrorCode.CHUNK_ERROR_CODE_MODEL_ERROR,
}


class PIIDetectorService(pb.PIIDetectorServicer):
    """gRPC servicer delegating to the core PIIDetector."""

    def __init__(self, detector: PIIDetector) -> None:
        self._detector = detector

    def _validate_request(self, request) -> None:
        """Validate the request; raises ValueError with a message on failure."""
        if not request.batch_id:
            raise ValueError("batch_id must be non-empty")
        if not request.chunks:
            raise ValueError("chunks must contain at least one item")
        seen = set()
        for chunk in request.chunks:
            if not chunk.chunk_id:
                raise ValueError("chunk_id must be non-empty")
            if chunk.chunk_id in seen:
                raise ValueError(f"duplicate chunk_id: {chunk.chunk_id}")
            seen.add(chunk.chunk_id)

    def DetectBatch(self, request, context):
        try:
            self._validate_request(request)
        except ValueError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
            return

        offset_unit = request.offset_unit
        if offset_unit == pb.OffsetUnit.OFFSET_UNIT_UNSPECIFIED:
            offset_unit = pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS

        chunks = [(c.chunk_id, c.text) for c in request.chunks]
        batch: BatchResult = self._detector.detect_batch(
            request.batch_id, "unicode_code_points", chunks
        )

        response = pb.DetectBatchResponse(
            batch_id=batch.batch_id,
            model_version=batch.model_version,
            offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
        )
        for res in batch.results:
            response.results.append(self._to_proto_result(res))
        return response

    def _to_proto_result(self, res: ChunkResult) -> pb.ChunkResult:
        proto_res = pb.ChunkResult(
            chunk_id=res.chunk_id,
            error_code=_ERROR_CODE_MAP.get(res.error_code, pb.ChunkErrorCode.CHUNK_ERROR_CODE_MODEL_ERROR),
        )
        for ent in res.entities:
            proto_type = _TARGET_TO_PROTO.get(ent.type)
            if proto_type is None:
                continue
            proto_res.entities.append(
                pb.Entity(
                    type=proto_type,
                    start=ent.start,
                    end=ent.end,
                    confidence=float(ent.score),
                )
            )
        return proto_res