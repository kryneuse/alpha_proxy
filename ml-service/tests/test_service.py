"""Tests for the gRPC service mapping and validation."""
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

from ml_service import proto as pb
from ml_service.config import Config
from ml_service.core.detector import PIIDetector
from ml_service.server.service import PIIDetectorService


class _Ctx:
    def __init__(self):
        self.aborted = None

    def abort(self, code, details):
        self.aborted = (code, details)
        raise RuntimeError(f"abort {code} {details}")


@pytest.fixture(scope="module")
def service():
    return PIIDetectorService(PIIDetector(Config()))


def test_entity_type_mapping(service):
    req = pb.DetectBatchRequest(
        batch_id="b1",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    req.chunks.append(
        pb.Chunk(chunk_id="c1", text="Иванов Пётр, тел. +7 999 123-45-67, email ivanov@mail.ru")
    )
    resp = service.DetectBatch(req, _Ctx())
    assert resp.batch_id == "b1"
    assert resp.model_version
    assert resp.offset_unit == pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS

    types = {e.type for r in resp.results for e in r.entities}
    assert pb.EntityType.ENTITY_TYPE_FULL_NAME in types
    assert pb.EntityType.ENTITY_TYPE_PHONE in types
    assert pb.EntityType.ENTITY_TYPE_EMAIL in types


def test_empty_batch_id_rejected(service):
    req = pb.DetectBatchRequest(
        batch_id="",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    req.chunks.append(pb.Chunk(chunk_id="c1", text="text"))
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        service.DetectBatch(req, ctx)
    assert ctx.aborted is not None
    assert ctx.aborted[0] == grpc_status_code_invalid_argument()


def grpc_status_code_invalid_argument():
    import grpc

    return grpc.StatusCode.INVALID_ARGUMENT


def test_empty_chunks_rejected(service):
    req = pb.DetectBatchRequest(
        batch_id="b1",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        service.DetectBatch(req, ctx)
    assert ctx.aborted is not None


def test_duplicate_chunk_id_rejected(service):
    req = pb.DetectBatchRequest(
        batch_id="b1",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    req.chunks.append(pb.Chunk(chunk_id="c1", text="a"))
    req.chunks.append(pb.Chunk(chunk_id="c1", text="b"))
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        service.DetectBatch(req, ctx)
    assert ctx.aborted is not None


def test_error_code_none_for_success(service):
    req = pb.DetectBatchRequest(
        batch_id="b1",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    req.chunks.append(pb.Chunk(chunk_id="c1", text="обычный текст без пд"))
    resp = service.DetectBatch(req, _Ctx())
    assert resp.results[0].error_code == pb.ChunkErrorCode.CHUNK_ERROR_CODE_NONE