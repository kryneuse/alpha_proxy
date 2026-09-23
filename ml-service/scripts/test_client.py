"""Simple gRPC client to exercise the PIIDetector service."""
from __future__ import annotations

import sys

import grpc

sys.path.insert(0, ".")
from ml_service import proto as pb
from ml_service.proto.pii_detector_pb2_grpc import PIIDetectorStub


def main() -> None:
    channel = grpc.insecure_channel("localhost:50051")
    stub = PIIDetectorStub(channel)

    req = pb.DetectBatchRequest(
        batch_id="test-batch",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    req.chunks.append(
        pb.Chunk(
            chunk_id="c1",
            text="Иванов Пётр Сергеевич, паспорт 45 11 123456, тел. +7 999 123-45-67, email ivanov@mail.ru",
        )
    )
    req.chunks.append(
        pb.Chunk(chunk_id="c2", text="Сегодня хорошая погода в парке.")
    )

    resp = stub.DetectBatch(req, timeout=60)
    print(f"batch_id: {resp.batch_id}")
    print(f"model_version: {resp.model_version}")
    print(f"offset_unit: {resp.offset_unit}")
    for r in resp.results:
        print(f"chunk {r.chunk_id}: error={r.error_code}, entities={len(r.entities)}")
        for e in r.entities:
            print(f"  type={e.type} [{e.start}:{e.end}] conf={e.confidence:.3f}")


if __name__ == "__main__":
    main()