"""ONNX Runtime inference wrapper for a token-classification model."""
from __future__ import annotations

import threading
from typing import Dict, List, Optional, Tuple

# Must run before onnxruntime initializes its thread pools.
from . import threading_setup  # noqa: F401

import numpy as np
import onnxruntime as ort
from transformers import AutoTokenizer


class OnnxTokenClassifier:
    """Thread-safe wrapper around an ONNX token-classification model.

    The ORT session is shared across threads (it is thread-safe for run()).
    The tokenizer is NOT thread-safe (the Rust `tokenizers` backend mutates
    internal truncation/padding state), so each thread gets its own instance
    via a thread-local. This guarantees safe concurrent tokenization.
    """

    def __init__(
        self,
        onnx_path: str,
        tokenizer_dir: str,
        intra_op_threads: int = 1,
        inter_op_threads: int = 1,
    ) -> None:
        self._tokenizer_dir = tokenizer_dir
        self._local = threading.local()

        so = ort.SessionOptions()
        so.intra_op_num_threads = intra_op_threads
        so.inter_op_num_threads = inter_op_threads
        so.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
        so.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        # Disable spinning to reduce CPU contention across workers.
        so.add_session_config_entry("session.intra_op.allow_spinning", "0")
        so.add_session_config_entry("session.inter_op.allow_spinning", "0")

        self._session = ort.InferenceSession(
            onnx_path,
            sess_options=so,
            providers=["CPUExecutionProvider"],
        )
        self._input_names = [i.name for i in self._session.get_inputs()]

    @property
    def tokenizer(self):
        """Return the calling thread's tokenizer instance."""
        tok = getattr(self._local, "tokenizer", None)
        if tok is None:
            tok = AutoTokenizer.from_pretrained(self._tokenizer_dir)
            self._local.tokenizer = tok
        return tok

    def tokenize(self, text: str, **kwargs) -> Dict:
        """Tokenize using the calling thread's tokenizer instance."""
        return self.tokenizer(text, **kwargs)

    def run(self, enc: Dict) -> np.ndarray:
        """Run inference on a tokenized batch. Returns logits (B, S, C)."""
        feeds = {}
        for name in self._input_names:
            val = enc[name]
            if hasattr(val, "numpy"):
                val = val.numpy()
            feeds[name] = val
        return self._session.run(None, feeds)[0]

    def run_logits(self, text: str, **tokenize_kwargs) -> Tuple[np.ndarray, Dict]:
        """Tokenize and run inference in one call.

        Returns (logits, encoding). The encoding retains offset_mapping and
        overflow info for offset recovery.
        """
        enc = self.tokenize(text, return_offsets_mapping=True, **tokenize_kwargs)
        logits = self.run(enc)
        return logits, enc