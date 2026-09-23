"""Concurrency test: send multiple parallel DetectBatch requests."""
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
    t0 = time.time()
    resp = stub.DetectBatch(req, timeout=60)
    dt = time.time() - t0
    n = sum(len(r.entities) for r in resp.results)
    return idx, dt, n


def main() -> None:
    channel = grpc.insecure_channel("localhost:50051")
    stub = PIIDetectorStub(channel)

    # Warm up.
    send_one(stub, 0)

    t0 = time.time()
    with ThreadPoolExecutor(max_workers=4) as pool:
        futures = [pool.submit(send_one, stub, i) for i in range(1, 9)]
        results = [f.result() for f in futures]
    total = time.time() - t0

    for idx, dt, n in results:
        print(f"batch {idx}: {dt:.2f}s, entities={n}")
    print(f"Total for 8 parallel batches: {total:.2f}s")


if __name__ == "__main__":
    main()