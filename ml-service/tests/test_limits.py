"""Tests for cancellation, load limits, and config wiring."""
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

from ml_service import proto as pb
from ml_service.config import Config
from ml_service.core.cancellation import CancelledError, check_cancelled
from ml_service.core.detector import PIIDetector
from ml_service.server.service import PIIDetectorService


class _Ctx:
    def __init__(self, active=True, remaining=60.0):
        self.aborted = None
        self._active = active
        self._remaining = remaining

    def abort(self, code, details):
        self.aborted = (code, details)
        raise RuntimeError(f"abort {code} {details}")

    def is_active(self):
        return self._active

    def time_remaining(self):
        return self._remaining


def test_check_cancelled_none():
    # None context is a no-op.
    check_cancelled(None)


def test_check_cancelled_inactive():
    ctx = _Ctx(active=False)
    with pytest.raises(CancelledError):
        check_cancelled(ctx)


def test_check_cancelled_deadline_exceeded():
    ctx = _Ctx(remaining=0.0)
    with pytest.raises(CancelledError):
        check_cancelled(ctx)


def test_check_cancelled_active():
    ctx = _Ctx(active=True, remaining=30.0)
    check_cancelled(ctx)  # no exception


def test_too_large_chunk_rejected():
    det = PIIDetector(Config())
    svc = PIIDetectorService(det)
    req = pb.DetectBatchRequest(
        batch_id="b1", offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS
    )
    req.chunks.append(pb.Chunk(chunk_id="c1", text="a" * 100001))
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        svc.DetectBatch(req, ctx)
    assert ctx.aborted is not None
    assert ctx.aborted[0].name == "INVALID_ARGUMENT"


def test_too_many_chunks_rejected():
    det = PIIDetector(Config())
    svc = PIIDetectorService(det)
    req = pb.DetectBatchRequest(
        batch_id="b1", offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS
    )
    for i in range(Config().batch_max_items + 1):
        req.chunks.append(pb.Chunk(chunk_id=f"c{i}", text="x"))
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        svc.DetectBatch(req, ctx)
    assert ctx.aborted is not None


def test_batch_total_chars_rejected():
    det = PIIDetector(Config())
    svc = PIIDetectorService(det)
    cfg = Config()
    req = pb.DetectBatchRequest(
        batch_id="b1", offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS
    )
    # Two chunks each under max_chunk_chars but together over batch_max_chars.
    half = cfg.batch_max_chars // 2 + 1
    req.chunks.append(pb.Chunk(chunk_id="c1", text="a" * half))
    req.chunks.append(pb.Chunk(chunk_id="c2", text="b" * half))
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        svc.DetectBatch(req, ctx)
    assert ctx.aborted is not None


def test_num_workers_drives_grpc_max_workers(monkeypatch):
    monkeypatch.setenv("ML_NUM_WORKERS", "16")
    cfg = Config.from_env()
    assert cfg.num_workers == 16
    assert cfg.grpc_max_workers == 16


def test_cancelled_batch_raises():
    det = PIIDetector(Config())
    ctx = _Ctx(active=False)
    with pytest.raises(CancelledError):
        det.detect_batch("b1", "unicode_code_points", [("c1", "text")], context=ctx)


def test_overload_rejected():
    """When the worker semaphore is saturated, requests are rejected early."""
    det = PIIDetector(Config())
    svc = PIIDetectorService(det)
    # Acquire all semaphore slots to simulate saturation.
    for _ in range(det._config.grpc_max_workers):
        svc._semaphore.acquire()
    req = pb.DetectBatchRequest(
        batch_id="b1", offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS
    )
    req.chunks.append(pb.Chunk(chunk_id="c1", text="text"))
    ctx = _Ctx()
    with pytest.raises(RuntimeError):
        svc.DetectBatch(req, ctx)
    assert ctx.aborted is not None
    assert ctx.aborted[0].name == "RESOURCE_EXHAUSTED"