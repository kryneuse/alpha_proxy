"""Stress test: send many parallel requests to check overload behavior."""
from __future__ import annotations

import sys
import time
from concurrent.futures import ThreadPoolExecutor

import grpc

sys.path.insert(0, ".")
from ml_service import proto as pb
from ml_service.proto.pii_detector_pb2_grpc import PIIDetectorStub


def send_one(stub, idx: int) -> tuple:
    req = pb.DetectBatchRequest(
        batch_id=f"batch-{idx}",
        offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
    )
    req.chunks.append(
        pb.Chunk(
            chunk_id=f"c{idx}",
            text=f"Иванов Пётр {idx}, тел. +7 999 123-45-67, email ivanov{idx}@mail.ru",
        )
    )
    try:
        resp = stub.DetectBatch(req, timeout=30)
        n = sum(len(r.entities) for r in resp.results)
        return idx, "ok", n
    except grpc.RpcError as e:
        return idx, e.code().name, 0


def main() -> None:
    channel = grpc.insecure_channel("localhost:50051")
    stub = PIIDetectorStub(channel)

    # Warm up.
    send_one(stub, 0)

    # Fire 32 parallel requests (well above the 4-worker pool).
    with ThreadPoolExecutor(max_workers=32) as pool:
        futures = [pool.submit(send_one, stub, i) for i in range(1, 33)]
        results = [f.result() for f in futures]

    ok = sum(1 for _, s, _ in results if s == "ok")
    rejected = sum(1 for _, s, _ in results if s != "ok")
    print(f"ok={ok}, rejected={rejected}")
    for idx, status, n in results:
        if status != "ok":
            print(f"  batch {idx}: {status}")


if __name__ == "__main__":
    main()