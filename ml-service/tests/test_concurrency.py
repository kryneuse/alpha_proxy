"""Tests for thread-safe concurrent detection."""
import os
import sys
from concurrent.futures import ThreadPoolExecutor

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

from ml_service.config import Config
from ml_service.core.detector import PIIDetector


@pytest.fixture(scope="module")
def detector():
    return PIIDetector(Config())


def test_concurrent_detection(detector):
    """Multiple threads must safely share the detector (thread-local tokenizers)."""

    def run(i):
        text = f"Иванов Пётр {i}, тел. +7 999 123-45-67, email ivanov{i}@mail.ru"
        res = detector.detect_chunk(f"c{i}", text)
        return i, res.audit.gate_open, len(res.entities)

    # Warm up (loads thread-local tokenizers).
    run(0)

    with ThreadPoolExecutor(max_workers=4) as pool:
        futures = [pool.submit(run, i) for i in range(1, 9)]
        results = [f.result() for f in futures]

    for i, gate_open, n in results:
        assert gate_open is True, f"chunk {i}: gate should be open"
        assert n >= 1, f"chunk {i}: expected at least one entity"