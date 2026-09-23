"""Thread budget setup for compute libraries.

Must be imported before torch/onnxruntime/numpy initialize their thread pools.
Limits each library to a single compute thread to match the 4 workers x 1
thread budget.
"""
from __future__ import annotations

import os


def limit_compute_threads(num_threads: int = 1) -> None:
    """Cap thread pools of compute libraries before they initialize."""
    os.environ.setdefault("OMP_NUM_THREADS", str(num_threads))
    os.environ.setdefault("MKL_NUM_THREADS", str(num_threads))
    os.environ.setdefault("OPENBLAS_NUM_THREADS", str(num_threads))
    os.environ.setdefault("VECLIB_MAXIMUM_THREADS", str(num_threads))
    os.environ.setdefault("NUMEXPR_NUM_THREADS", str(num_threads))
    os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")


# Apply defaults at import time so any later import of torch/onnxruntime/numpy
# sees the capped thread counts.
limit_compute_threads()